package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

// Backups copy an environment's named volumes, and the stack's directory
// for stacks from Git, into gzipped tarballs on the node itself, through
// helper containers, so they work over any Docker transport. Deploys take
// one before a setup command runs, since its migrations can't be rolled
// back, and optionally before any deploy that changes the version.

// BackupsDir is where nodes keep backups: one directory per environment,
// and one inside it per backup.
func BackupsDir() string {
	if dir := os.Getenv("DOCKTRINE_BACKUPS_DIR"); dir != "" {
		return strings.TrimRight(dir, "/")
	}
	return "/var/lib/docktrine/backups"
}

// DefaultBackupsKept is how many backups an environment keeps when its
// spec doesn't say.
func DefaultBackupsKept() int {
	if n, err := strconv.Atoi(os.Getenv("DOCKTRINE_BACKUPS_KEEP")); err == nil && n > 0 {
		return n
	}
	return 5
}

func backupsKept(spec *manifest.Spec) int {
	if spec != nil && spec.Backup != nil && spec.Backup.Keep > 0 {
		return spec.Backup.Keep
	}
	return DefaultBackupsKept()
}

// A backup and restore can take long for big volumes.
const backupTimeout = 2 * time.Hour

const stampLayout = "20060102-150405"

var (
	stampPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}(-[0-9]+)?$`)
	// What Docker allows in volume names
	volumeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// printer is where backups report progress: a deployment's log, or the
// server log for backups taken on demand.
type printer interface {
	Printf(format string, args ...any)
}

type serverLog struct{ prefix string }

func (l serverLog) Printf(format string, args ...any) {
	logger.Info(l.prefix + fmt.Sprintf(format, args...))
}

// backupReason says why a deploy backs up first, or "" when it doesn't.
// setup is why the setup command runs ("" when it doesn't run), and
// upgrade how the version changes ("" when it doesn't). Each reason reads
// after "since".
func backupReason(policy, setup, upgrade string) string {
	switch {
	case policy == manifest.BackupNever:
		return ""
	case setup != "":
		return "the setup command runs"
	case policy == manifest.BackupBeforeUpgrade && upgrade != "":
		return upgrade
	}
	return ""
}

// versionChange describes how next changes the version of what runs from
// prev: another image, another commit of a stack from Git, or other images
// in a compose file. It is "" when nothing ran before, or when only
// settings change.
func versionChange(prev, next *manifest.Spec) string {
	if prev == nil || next == nil {
		return ""
	}
	switch {
	case prev.Source != nil && next.Source != nil:
		if prev.Source.Repo != next.Source.Repo {
			return "the stack's repository changes"
		}
		if prev.Source.Commit != "" && next.Source.Commit != "" && prev.Source.Commit != next.Source.Commit {
			return fmt.Sprintf("the stack changes from %s to %s", short(prev.Source.Commit), short(next.Source.Commit))
		}
	case prev.Image != "" && next.Image != "":
		if prev.Image != next.Image {
			return fmt.Sprintf("the image changes from %s to %s", prev.Image, next.Image)
		}
	case prev.Compose != "" && next.Compose != "":
		return composeImageChange(prev.Compose, next.Compose)
	default:
		return "what runs changes from " + describeKind(prev) + " to " + describeKind(next)
	}
	return ""
}

func describeKind(s *manifest.Spec) string {
	switch {
	case s.Source != nil:
		return "a stack from Git"
	case s.Compose != "":
		return "a compose file"
	}
	return "an image"
}

// composeImageChange names the services whose image changes between two
// compose files.
func composeImageChange(prev, next string) string {
	before, _, err1 := parseComposeServices(prev)
	after, order, err2 := parseComposeServices(next)
	if err1 != nil || err2 != nil {
		return ""
	}
	changes := []string{}
	for _, name := range order {
		was, _ := before[name]["image"].(string)
		now, _ := after[name]["image"].(string)
		if was != "" && now != "" && was != now {
			changes = append(changes, fmt.Sprintf("%s changes from %s to %s", name, was, now))
		}
	}
	switch len(changes) {
	case 0:
		return ""
	case 1:
		return changes[0]
	}
	return fmt.Sprintf("%s, and %d more %s", changes[0], len(changes)-1, plural(len(changes)-1, "service", "services"))
}

// newerImages returns the images of running containers whose tag now
// points at a different image than the one the container runs, as after
// pulling a newer `latest`.
func newerImages(ctx context.Context, cli *client.Client, containers []types.Container) []string {
	seen := map[string]bool{}
	newer := []string{}
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		// The list shows an image ID once its tag has moved on; the
		// container's config keeps the reference it was created from
		inspect, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil || inspect.Config == nil {
			continue
		}
		ref := inspect.Config.Image
		if seen[ref] || strings.HasPrefix(ref, "sha256:") {
			continue
		}
		seen[ref] = true
		img, _, err := cli.ImageInspectWithRaw(ctx, ref)
		if err == nil && img.ID != inspect.Image {
			newer = append(newer, ref)
		}
	}
	sort.Strings(newer)
	return newer
}

func describeNewer(images []string) string {
	if len(images) == 1 {
		return images[0] + " has a newer image"
	}
	return fmt.Sprintf("%s and %d more have newer images", images[0], len(images)-1)
}

// environmentVolumes picks an environment's volumes: those image deploys
// create, labelled with the app and environment, and those compose creates
// for the project.
func environmentVolumes(all []*volume.Volume, app, env string) []*volume.Volume {
	project := app + "-" + env
	picked := []*volume.Volume{}
	for _, v := range all {
		if v == nil {
			continue
		}
		ours := v.Labels[LabelApp] == app && v.Labels[LabelEnv] == env
		if ours || v.Labels["com.docker.compose.project"] == project {
			picked = append(picked, v)
		}
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].Name < picked[j].Name })
	return picked
}

// mountInfo is one mount of one of the environment's containers.
type mountInfo struct {
	Container string
	Type      string
	Name      string
	Source    string
	Target    string
}

// skippedMounts lists what the containers mount that a backup leaves out:
// directories on the node, other than the stack's own when it is backed up,
// and volumes the environment didn't create.
func skippedMounts(mounts []mountInfo, backedUp map[string]bool, stackDir string) []database.SkippedMount {
	skipped := []database.SkippedMount{}
	// One entry per mount, naming every container that has it
	seen := map[string]int{}
	for _, m := range mounts {
		var reason string
		switch m.Type {
		case "bind":
			if stackDir != "" && (m.Source == stackDir || strings.HasPrefix(m.Source, stackDir+"/")) {
				continue
			}
			reason = "a directory on the node, which backups leave alone"
		case "volume":
			if backedUp[m.Name] {
				continue
			}
			if anonymousVolume.MatchString(m.Name) {
				reason = "an anonymous volume, with no name to restore it to"
			} else {
				reason = "a volume this environment didn't create"
			}
		default:
			continue
		}
		key := m.Type + "\x00" + m.Source + "\x00" + m.Name + "\x00" + m.Target
		if i, ok := seen[key]; ok {
			if !strings.Contains(", "+skipped[i].Container+", ", ", "+m.Container+", ") {
				skipped[i].Container += ", " + m.Container
			}
			continue
		}
		seen[key] = len(skipped)
		source := m.Source
		if m.Type == "volume" {
			source = m.Name
		}
		skipped = append(skipped, database.SkippedMount{
			Type: m.Type, Source: source, Target: m.Target, Container: m.Container, Reason: reason,
		})
	}
	return skipped
}

var anonymousVolume = regexp.MustCompile(`^[0-9a-f]{64}$`)

// expiredBackups picks the backups retention deletes, from an
// environment's backups, newest first: successful ones beyond the newest
// keep, and failed ones older than a successful one. The newest successful
// backup always stays, and so do running ones.
func expiredBackups(backups []database.Backup, keep int) []database.Backup {
	keep = max(keep, 1)
	expired := []database.Backup{}
	succeeded := 0
	for _, b := range backups {
		switch b.Status {
		case database.BackupSucceeded:
			succeeded++
			if succeeded > keep {
				expired = append(expired, b)
			}
		case database.BackupFailed:
			if succeeded > 0 {
				expired = append(expired, b)
			}
		}
	}
	return expired
}

// validBackupPath checks that a backup's directory is one Docktrine made,
// <root>/<app>-<env>/<timestamp>, so deleting it can't reach anything else.
func validBackupPath(p string) bool {
	if !path.IsAbs(p) || path.Clean(p) != p {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == "." {
			return false
		}
	}
	project, stamp := path.Base(path.Dir(p)), path.Base(p)
	root := path.Dir(path.Dir(p))
	return root != "/" && root != "." && projectPattern.MatchString(project) && stampPattern.MatchString(stamp)
}

var projectPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// validArchiveName checks an archive's file name in a backup's directory.
func validArchiveName(name string) bool {
	if !strings.HasSuffix(name, ".tar.gz") || strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	base := strings.TrimSuffix(name, ".tar.gz")
	return base != "" && !strings.Contains(base, "..") && volumeNamePattern.MatchString(base)
}

// archiveName is the file a volume, or the stack's directory, goes into.
// The prefix keeps a volume named "stack" apart from the directory.
func archiveName(item database.BackupItem) string {
	if item.Kind == database.BackupItemStack {
		return "stack.tar.gz"
	}
	return "volume-" + item.Name + ".tar.gz"
}

// backupDirName is a new backup's directory name, unique among existing.
func backupDirName(now time.Time, existing map[string]bool) string {
	name := now.UTC().Format(stampLayout)
	for i := 2; existing[name]; i++ {
		name = fmt.Sprintf("%s-%d", now.UTC().Format(stampLayout), i)
	}
	return name
}

// spaceNeeded is the free space a backup of total bytes asks for. The
// archives are compressed, so they take less than total, but the disk
// shouldn't be left full either.
func spaceNeeded(total int64) int64 {
	return total + total/20 + 100<<20
}

// parseDu reads `du -sk` lines into bytes per path.
func parseDu(out string) map[string]int64 {
	sizes := map[string]int64{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if kb, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			sizes[fields[1]] = kb * 1024
		}
	}
	return sizes
}

// parseDfAvailable reads the bytes available from `df -Pk` output.
func parseDfAvailable(out string) (int64, bool) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 1; i-- {
		fields := strings.Fields(lines[i])
		if len(fields) >= 6 {
			if kb, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
				return kb * 1024, true
			}
		}
	}
	return 0, false
}

func formatBytes(n int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func since(start time.Time) string {
	d := time.Since(start)
	if d < 10*time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// runHelper runs cmd in a helper container on the node, with binds, and
// returns what it printed.
func runHelper(ctx context.Context, cli *client.Client, binds []string, cmd ...string) (string, error) {
	if err := ensureImage(ctx, cli, helperImage); err != nil {
		return "", err
	}
	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:  helperImage,
			Cmd:    cmd,
			Labels: map[string]string{LabelRole: "helper"},
		},
		&container.HostConfig{Binds: binds, NetworkMode: "none"},
		nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("creating the helper container: %w", err)
	}
	defer removeContainer(cli, created.ID)

	wait, errs := cli.ContainerWait(ctx, created.ID, container.WaitConditionNextExit)
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("starting the helper container: %w", err)
	}
	var status int64
	select {
	case res := <-wait:
		if res.Error != nil {
			return "", fmt.Errorf("the helper container: %s", res.Error.Message)
		}
		status = res.StatusCode
	case err := <-errs:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}

	var out bytes.Buffer
	if logs, err := cli.ContainerLogs(ctx, created.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true}); err == nil {
		demux(&out, logs)
		logs.Close()
	}
	text := strings.TrimSpace(out.String())
	if status != 0 {
		if text == "" {
			text = fmt.Sprintf("exited with %d", status)
		}
		return text, errors.New(lastLines(text, 5))
	}
	return text, nil
}

func lastLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "; ")
}

// environmentContainers lists every container of an environment: those
// Docktrine labels, and those compose runs for its project, which includes
// any a setup command started itself.
func environmentContainers(ctx context.Context, cli *client.Client, app, env string) ([]types.Container, error) {
	labelled, err := listWorkload(ctx, cli, app, env)
	if err != nil {
		return nil, err
	}
	project, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+app+"-"+env)),
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	all := []types.Container{}
	for _, c := range append(labelled, project...) {
		if !seen[c.ID] && c.Labels[LabelRole] == "" {
			seen[c.ID] = true
			all = append(all, c)
		}
	}
	return all, nil
}

func containerName(c types.Container) string {
	if name := c.Labels["com.docker.compose.service"]; name != "" {
		return name
	}
	return strings.TrimPrefix(firstName(c.Names), "/")
}

// stopRunning stops the running containers among cs, all at once, and
// returns the ones it stopped.
func stopRunning(ctx context.Context, cli *client.Client, cs []types.Container) ([]types.Container, error) {
	running := []types.Container{}
	for _, c := range cs {
		if c.State == "running" || c.State == "restarting" {
			running = append(running, c)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, len(running))
	timeout := 30
	for i, c := range running {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			errs[i] = cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
		}(i, c.ID)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return running, fmt.Errorf("stopping %s: %w", containerName(running[i]), err)
		}
	}
	return running, nil
}

func startAll(cli *client.Client, cs []types.Container) error {
	// Starting again must work even after the deploy's own time ran out
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	failed := []string{}
	for _, c := range cs {
		if err := cli.ContainerStart(ctx, c.ID, container.StartOptions{}); err != nil && !errdefs.IsNotFound(err) {
			failed = append(failed, containerName(c))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("couldn't start %s again", strings.Join(failed, ", "))
	}
	return nil
}

func containerNames(cs []types.Container) string {
	names := []string{}
	for _, c := range cs {
		names = append(names, containerName(c))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// backupPlan is what a backup of an environment covers.
type backupPlan struct {
	server     *database.Server
	app, env   string
	items      []database.BackupItem
	skipped    []database.SkippedMount
	containers []types.Container
}

func (p *backupPlan) project() string { return p.app + "-" + p.env }

// planBackup finds what an environment keeps data in. withStack adds the
// stack's directory on the node, which stacks from Git have.
func planBackup(ctx context.Context, cli *client.Client, server *database.Server, app, env string, withStack bool) (*backupPlan, error) {
	plan := &backupPlan{server: server, app: app, env: env}

	list, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing volumes: %w", err)
	}
	backedUp := map[string]bool{}
	for _, v := range environmentVolumes(list.Volumes, app, env) {
		if !volumeNamePattern.MatchString(v.Name) {
			continue
		}
		backedUp[v.Name] = true
		plan.items = append(plan.items, database.BackupItem{Kind: database.BackupItemVolume, Name: v.Name, Labels: v.Labels})
	}
	stackDir := ""
	if withStack {
		stackDir = StacksDir() + "/" + plan.project()
		plan.items = append(plan.items, database.BackupItem{Kind: database.BackupItemStack, Name: stackDir})
	}
	for i := range plan.items {
		plan.items[i].File = archiveName(plan.items[i])
	}

	if plan.containers, err = environmentContainers(ctx, cli, app, env); err != nil {
		return nil, err
	}
	mounts := []mountInfo{}
	for _, c := range plan.containers {
		for _, m := range c.Mounts {
			mounts = append(mounts, mountInfo{
				Container: containerName(c), Type: string(m.Type), Name: m.Name, Source: m.Source, Target: m.Destination,
			})
		}
	}
	plan.skipped = skippedMounts(mounts, backedUp, stackDir)
	return plan, nil
}

// backupMeta is what a backup records about why and when it was taken.
type backupMeta struct {
	reason      string
	createdBy   string
	triggeredBy *int64
	// What ran when it was taken
	current *database.Deployment
}

// recordBackup creates the backup's row, with a directory of its own.
func (d *Deployer) recordBackup(plan *backupPlan, meta backupMeta) (*database.Backup, error) {
	existing, err := d.db.ListBackups(plan.app, plan.env)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, b := range existing {
		used[path.Base(b.Path)] = true
	}
	dir := BackupsDir() + "/" + plan.project() + "/" + backupDirName(time.Now(), used)
	if !validBackupPath(dir) {
		return nil, fmt.Errorf("the backups directory %s must be an absolute path", BackupsDir())
	}
	b := &database.Backup{
		App: plan.app, Environment: plan.env, Server: plan.server.Name, Path: dir,
		Reason: meta.reason, CreatedBy: meta.createdBy, TriggeredBy: meta.triggeredBy,
	}
	if meta.current != nil {
		b.DeploymentID, b.Spec = &meta.current.ID, meta.current.Spec
	}
	return d.db.CreateBackup(b)
}

// runBackup archives each item of the plan into the backup's directory,
// with the environment's containers stopped so databases are copied in a
// consistent state, and starts them again after.
func (d *Deployer) runBackup(ctx context.Context, cli *client.Client, plan *backupPlan, b *database.Backup, log printer) (err error) {
	started := time.Now()
	items := append([]database.BackupItem(nil), plan.items...)
	defer func() {
		status, message := database.BackupSucceeded, ""
		if err != nil {
			status, message = database.BackupFailed, err.Error()
			items = nil
			// A half-written backup is no use, and takes space
			if rmErr := deleteBackupFiles(context.Background(), cli, b.Path); rmErr != nil {
				log.Printf("Couldn't delete the incomplete backup in %s: %v", b.Path, rmErr)
			}
		}
		if dbErr := d.db.FinishBackup(b.ID, status, message, items, plan.skipped); dbErr != nil {
			logger.Error(dbErr, "Failed to record the backup")
		}
	}()

	// Every item read-only under /v, to measure; the backups directory, to
	// check its free space
	binds := []string{BackupsDir() + ":/backups"}
	paths := []string{}
	for i, item := range items {
		binds = append(binds, fmt.Sprintf("%s:/v/%d:ro", item.Name, i))
		paths = append(paths, fmt.Sprintf("/v/%d", i))
	}
	out, err := runHelper(ctx, cli, binds, append([]string{"sh", "-c", `du -sk "$@"; df -Pk /backups`, "sh"}, paths...)...)
	if err != nil {
		return fmt.Errorf("measuring the volumes: %w", err)
	}
	sizes := parseDu(out)
	free, ok := parseDfAvailable(out)
	if !ok {
		return fmt.Errorf("couldn't read the free space in %s on %s", BackupsDir(), plan.server.Name)
	}
	var total int64
	described := []string{}
	for i := range items {
		items[i].Size = sizes[paths[i]]
		total += items[i].Size
		described = append(described, fmt.Sprintf("%s (%s)", describeItem(items[i]), formatBytes(items[i].Size)))
	}
	if len(items) == 1 {
		log.Printf("Backing up %s", described[0])
	} else {
		log.Printf("Backing up %s: %s in all", strings.Join(described, ", "), formatBytes(total))
	}
	for _, s := range plan.skipped {
		log.Printf("Not backed up: %s at %s in %s, %s", s.Source, s.Target, s.Container, s.Reason)
	}
	if need := spaceNeeded(total); free < need {
		return fmt.Errorf("not enough space for the backup on %s: it needs up to %s in %s, which has %s free. Free some space, delete old backups, or turn off backups for this environment",
			plan.server.Name, formatBytes(need), BackupsDir(), formatBytes(free))
	}

	if len(plan.containers) > 0 {
		stopping := time.Now()
		stopped, stopErr := stopRunning(ctx, cli, plan.containers)
		if len(stopped) > 0 {
			log.Printf("Stopped %s, so the copy is consistent (%s)", containerNames(stopped), since(stopping))
			defer func() {
				// The backup is good either way; a deploy that follows
				// starts the containers itself
				if startErr := startAll(cli, stopped); startErr != nil {
					log.Printf("WARNING: %v", startErr)
					return
				}
				log.Printf("Started %s again, after %s stopped", containerNames(stopped), since(stopping))
			}()
		}
		if stopErr != nil {
			return stopErr
		}
	}

	for i := range items {
		item := &items[i]
		at := time.Now()
		out, err := runHelper(ctx, cli,
			[]string{item.Name + ":/data:ro", b.Path + ":/backup"},
			// Only root on the node may read them: they hold the databases
			"sh", "-c", `umask 077 && chmod 700 /backup && tar -czf "/backup/$1" -C /data . && stat -c %s "/backup/$1"`, "sh", item.File)
		if err != nil {
			return fmt.Errorf("backing up %s: %w", describeItem(*item), err)
		}
		item.ArchiveSize, _ = strconv.ParseInt(strings.TrimSpace(lastLine(out)), 10, 64)
		log.Printf("Backed up %s: %s, compressed to %s, in %s", describeItem(*item), formatBytes(item.Size), formatBytes(item.ArchiveSize), since(at))
	}

	var size int64
	for _, item := range items {
		size += item.ArchiveSize
	}
	log.Printf("Backup #%d finished in %s: %s in %s on %s", b.ID, since(started), formatBytes(size), b.Path, plan.server.Name)
	return nil
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return lines[len(lines)-1]
}

func describeItem(item database.BackupItem) string {
	if item.Kind == database.BackupItemStack {
		return "the stack's directory"
	}
	return item.Name
}

// deleteBackupFiles removes a backup's directory on the node, and the
// environment's directory once it holds no backups.
func deleteBackupFiles(ctx context.Context, cli *client.Client, dir string) error {
	if !validBackupPath(dir) {
		return fmt.Errorf("refusing to delete %q, which isn't a backup's directory", dir)
	}
	root, project, stamp := path.Dir(path.Dir(dir)), path.Base(path.Dir(dir)), path.Base(dir)
	_, err := runHelper(ctx, cli, []string{root + ":/backups"},
		"sh", "-c", `rm -rf "/backups/$1/$2" && (rmdir "/backups/$1" 2>/dev/null || true)`, "sh", project, stamp)
	return err
}

// pruneBackups deletes the environment's backups beyond what it keeps.
func (d *Deployer) pruneBackups(ctx context.Context, cli *client.Client, server *database.Server, app, env string, keep int, log printer) {
	backups, err := d.db.ListBackups(app, env)
	if err != nil {
		log.Printf("WARNING: couldn't list backups to delete old ones: %v", err)
		return
	}
	for _, b := range expiredBackups(backups, keep) {
		// Only backups on this node can be deleted from here
		if b.Server != server.Name {
			continue
		}
		if err := deleteBackupFiles(ctx, cli, b.Path); err != nil {
			log.Printf("WARNING: couldn't delete backup #%d: %v", b.ID, err)
			continue
		}
		if err := d.db.DeleteBackup(b.ID); err != nil {
			log.Printf("WARNING: couldn't delete backup #%d: %v", b.ID, err)
			continue
		}
		if b.Status == database.BackupSucceeded {
			log.Printf("Deleted backup #%d from %s, keeping the last %d", b.ID, b.CreatedAt.Local().Format("Jan 2 15:04"), keep)
		} else {
			log.Printf("Deleted failed backup #%d", b.ID)
		}
	}
}

// runningDeployment is the deployment an environment runs now, if any.
func (d *Deployer) runningDeployment(app, env string) (*database.Deployment, error) {
	e, err := d.db.GetEnvironment(app, env)
	if err != nil || e == nil || e.CurrentDeploymentID == nil {
		return nil, err
	}
	return d.db.GetDeployment(*e.CurrentDeploymentID)
}

// hasStackDir says whether the environment keeps a stack directory on the
// node: any earlier deploy of a stack from Git made one, even a failed one,
// which may have run its setup command. except is the deploy in progress,
// which hasn't copied anything yet.
func (d *Deployer) hasStackDir(app, env string, except int64) (bool, error) {
	e, err := d.db.GetEnvironment(app, env)
	if err != nil || e == nil {
		return false, err
	}
	history, err := d.db.ListDeployments(e.ID, 1000)
	if err != nil {
		return false, err
	}
	for _, dep := range history {
		if dep.ID != except && dep.Spec != nil && dep.Spec.Source != nil {
			return true, nil
		}
	}
	return false, nil
}

// backupBeforeDeploy backs the environment up when reason says to, before
// a deploy changes anything. Running out of space, or any other failure,
// fails the deploy: better than migrating without a way back.
func (d *Deployer) backupBeforeDeploy(ctx context.Context, cli *client.Client, t target, reason string, log *deployLog) error {
	current, err := d.runningDeployment(t.spec.App, t.spec.Environment)
	if err != nil {
		return err
	}
	stack, err := d.hasStackDir(t.spec.App, t.spec.Environment, t.deploymentID)
	if err != nil {
		return err
	}
	plan, err := planBackup(ctx, cli, t.server, t.spec.App, t.spec.Environment, stack)
	if err != nil {
		return fmt.Errorf("planning the backup: %w", err)
	}
	if len(plan.items) == 0 {
		log.Printf("Nothing to back up yet: %s/%s has no volumes", t.spec.App, t.spec.Environment)
		return nil
	}
	log.Printf("Backing up first, since %s", reason)
	id := t.deploymentID
	b, err := d.recordBackup(plan, backupMeta{reason: reason, createdBy: "deploy #" + strconv.FormatInt(id, 10), triggeredBy: &id, current: current})
	if err != nil {
		return err
	}
	if err := d.runBackup(ctx, cli, plan, b, log); err != nil {
		return fmt.Errorf("the backup before deploying failed, so nothing was changed: %w", err)
	}
	d.pruneBackups(ctx, cli, t.server, t.spec.App, t.spec.Environment, backupsKept(t.spec), log)
	return nil
}

// Busy says what keeps an environment from deploying, such as a backup in
// progress, or "".
func (d *Deployer) Busy(app, env string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ops[app+"-"+env]
}

// claim marks an environment busy with op, unless it already is.
func (d *Deployer) claim(app, env, op string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if busy := d.ops[app+"-"+env]; busy != "" {
		return fmt.Errorf("%s/%s is busy: %s", app, env, busy)
	}
	e, err := d.db.GetEnvironment(app, env)
	if err != nil {
		return err
	}
	if e != nil {
		active, err := d.db.HasActiveDeployment(e.ID)
		if err != nil {
			return err
		}
		if active {
			return fmt.Errorf("a deployment of %s/%s is in progress", app, env)
		}
	}
	d.ops[app+"-"+env] = op
	return nil
}

func (d *Deployer) release(app, env string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.ops, app+"-"+env)
}

// BackupNow starts a backup of an environment and returns it while it
// runs. Its containers stop for as long as it takes.
func (d *Deployer) BackupNow(env *database.Environment, createdBy string) (*database.Backup, error) {
	server, err := d.db.ResolveServer(env.Server)
	if err != nil {
		return nil, err
	}
	if err := d.claim(env.App, env.Name, "a backup is in progress"); err != nil {
		return nil, err
	}
	cli, err := connect(server)
	if err != nil {
		d.release(env.App, env.Name)
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), backupTimeout)
	cleanup := func() {
		cancel()
		cli.Close()
		d.release(env.App, env.Name)
	}

	current, err := d.runningDeployment(env.App, env.Name)
	if err != nil {
		cleanup()
		return nil, err
	}
	stack, err := d.hasStackDir(env.App, env.Name, 0)
	if err != nil {
		cleanup()
		return nil, err
	}
	plan, err := planBackup(ctx, cli, server, env.App, env.Name, stack)
	if err != nil {
		cleanup()
		return nil, err
	}
	if len(plan.items) == 0 {
		cleanup()
		return nil, fmt.Errorf("%s/%s has no volumes to back up", env.App, env.Name)
	}
	b, err := d.recordBackup(plan, backupMeta{reason: "it was asked for", createdBy: createdBy, current: current})
	if err != nil {
		cleanup()
		return nil, err
	}

	go func() {
		defer cleanup()
		log := serverLog{prefix: fmt.Sprintf("backup #%d of %s/%s: ", b.ID, env.App, env.Name)}
		if err := d.runBackup(ctx, cli, plan, b, log); err != nil {
			log.Printf("FAILED: %v", err)
			return
		}
		var spec *manifest.Spec
		if current != nil {
			spec = current.Spec
		}
		d.pruneBackups(ctx, cli, server, env.App, env.Name, backupsKept(spec), log)
	}()
	return b, nil
}

// RestoreResult says what a restore did.
type RestoreResult struct {
	Backup *database.Backup `json:"backup"`
	// The redeploy of the deployment the backup was taken with, if asked for
	Deployment *database.Deployment `json:"deployment,omitempty"`
	Message    string               `json:"message"`
}

// Restore replaces an environment's volumes, and its stack directory, with
// a backup's. Its containers are stopped first. With redeploy, the
// deployment that ran when the backup was taken is deployed again, so the
// code matches the data; otherwise the stopped containers start again.
func (d *Deployer) Restore(b *database.Backup, redeploy bool, createdBy string) (*RestoreResult, error) {
	if b.Status != database.BackupSucceeded {
		return nil, fmt.Errorf("backup #%d didn't finish, so it can't be restored", b.ID)
	}
	if redeploy && b.Spec == nil {
		return nil, fmt.Errorf("backup #%d doesn't record what ran when it was taken, so it can only be restored without redeploying", b.ID)
	}
	if !validBackupPath(b.Path) {
		return nil, fmt.Errorf("backup #%d has an invalid directory %q", b.ID, b.Path)
	}
	for _, item := range b.Items {
		if !validArchiveName(item.File) {
			return nil, fmt.Errorf("backup #%d has an invalid archive %q", b.ID, item.File)
		}
		if item.Kind == database.BackupItemStack && !validStackDir(item.Name, b.App+"-"+b.Environment) {
			return nil, fmt.Errorf("backup #%d has an invalid stack directory %q", b.ID, item.Name)
		}
		if item.Kind == database.BackupItemVolume && !volumeNamePattern.MatchString(item.Name) {
			return nil, fmt.Errorf("backup #%d has an invalid volume name %q", b.ID, item.Name)
		}
	}
	server, err := d.db.ResolveServer(b.Server)
	if err != nil {
		return nil, err
	}
	if err := d.claim(b.App, b.Environment, "a restore is in progress"); err != nil {
		return nil, err
	}
	released := false
	release := func() {
		if !released {
			released = true
			d.release(b.App, b.Environment)
		}
	}
	defer release()

	cli, err := connect(server)
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), backupTimeout)
	defer cancel()

	log := serverLog{prefix: fmt.Sprintf("restore of backup #%d to %s/%s: ", b.ID, b.App, b.Environment)}
	started := time.Now()

	containers, err := environmentContainers(ctx, cli, b.App, b.Environment)
	if err != nil {
		return nil, err
	}
	stopped, err := stopRunning(ctx, cli, containers)
	if len(stopped) > 0 {
		log.Printf("Stopped %s", containerNames(stopped))
	}
	// Without a redeploy, or when restoring fails, what ran starts again
	restart := !redeploy
	defer func() {
		if restart && len(stopped) > 0 {
			if err := startAll(cli, stopped); err != nil {
				log.Printf("WARNING: %v", err)
			}
		}
	}()
	if err != nil {
		restart = true
		return nil, err
	}

	for _, item := range b.Items {
		at := time.Now()
		if item.Kind == database.BackupItemVolume {
			if _, err := cli.VolumeInspect(ctx, item.Name); errdefs.IsNotFound(err) {
				// Destroyed with its volumes; create it as it was
				if _, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: item.Name, Labels: item.Labels}); err != nil {
					restart = true
					return nil, fmt.Errorf("creating volume %s: %w", item.Name, err)
				}
			} else if err != nil {
				restart = true
				return nil, err
			}
		}
		// The archive is checked whole before anything is deleted
		_, err := runHelper(ctx, cli,
			[]string{item.Name + ":/data", b.Path + ":/backup:ro"},
			"sh", "-c", `gzip -t "/backup/$1" && find /data -mindepth 1 -maxdepth 1 -exec rm -rf {} + && tar -xzf "/backup/$1" -C /data`, "sh", item.File)
		if err != nil {
			restart = true
			return nil, fmt.Errorf("restoring %s: %w", describeItem(item), err)
		}
		log.Printf("Restored %s (%s) in %s", describeItem(item), formatBytes(item.Size), since(at))
	}
	if err := d.db.SetBackupRestored(b.ID); err != nil {
		return nil, err
	}

	res := &RestoreResult{Backup: b}
	taken := b.CreatedAt.Local().Format("Jan 2 15:04")
	res.Message = fmt.Sprintf("Restored backup #%d from %s in %s.", b.ID, taken, since(started))
	if !redeploy {
		if len(stopped) > 0 {
			res.Message += " Its containers are starting again."
		}
		return res, nil
	}

	release()
	what := "the deployment"
	if b.DeploymentID != nil {
		what = fmt.Sprintf("#%d", *b.DeploymentID)
	}
	dep, err := d.start(b.Spec, fmt.Sprintf("%s (restoring backup #%d)", createdBy, b.ID), startOptions{skipBackup: true})
	if err != nil {
		// The data is back; what ran is stopped and shouldn't start on it
		res.Message += fmt.Sprintf(" Redeploying %s didn't start: %v. Its containers are stopped; deploy it to start them.", what, err)
		return res, nil
	}
	res.Deployment = dep
	res.Message += fmt.Sprintf(" Redeploying %s as #%d.", what, dep.ID)
	return res, nil
}

// validStackDir checks a stack directory recorded in a backup: a clean
// absolute path, ending in the environment's project.
func validStackDir(dir, project string) bool {
	return path.IsAbs(dir) && path.Clean(dir) == dir && path.Base(dir) == project && path.Dir(dir) != "/" &&
		!strings.Contains(dir, "/../")
}

// DeleteBackup removes a backup's files from its node and its record.
func (d *Deployer) DeleteBackup(b *database.Backup) error {
	if b.Status == database.BackupRunning {
		return fmt.Errorf("backup #%d is still running", b.ID)
	}
	server, err := d.db.ResolveServer(b.Server)
	if err != nil {
		return fmt.Errorf("%w; the backup's files are on %s", err, b.Server)
	}
	cli, err := connect(server)
	if err != nil {
		return err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := deleteBackupFiles(ctx, cli, b.Path); err != nil {
		return err
	}
	return d.db.DeleteBackup(b.ID)
}
