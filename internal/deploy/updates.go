package deploy

// Update notices: whether an environment runs the newest commit of its Git
// ref and the newest images behind its tags. Checks run in the background
// every few hours and on demand, and their results are cached in the
// database. An update is an ordinary deployment of the current spec, so it
// gets the deploy log, rollback and setup commands like any other.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/distribution/reference"
	"github.com/docker/docker/client"
)

const (
	// How often an environment is checked when the last check went well
	DefaultUpdateInterval = 6 * time.Hour
	// How often the background loop looks for environments that are due
	updateTick = 10 * time.Minute
	// After a failed check, the next waits this long, doubling each time
	// it fails again, up to updateRetryMax
	updateRetryBase = 30 * time.Minute
	updateRetryMax  = 24 * time.Hour
	// The least time between two requests to one registry or forge, so a
	// round of checks doesn't arrive as a burst
	hostSpacing = 2 * time.Second
	// A big stack has dozens of services; past this many images, the rest
	// wait for the next round rather than all asked at once
	maxImagesPerCheck = 25
	// "Check now" right after a check answers with what it found
	recheckAfter = 15 * time.Second
	checkTimeout = 2 * time.Minute
)

// UpdateIntervalFromEnv reads DOCKTRINE_UPDATE_INTERVAL, e.g. "12h". "0" or
// "off" turns the background checks off; checking on demand still works.
func UpdateIntervalFromEnv() time.Duration {
	value := strings.TrimSpace(os.Getenv("DOCKTRINE_UPDATE_INTERVAL"))
	switch value {
	case "":
		return DefaultUpdateInterval
	case "0", "off", "false":
		return 0
	}
	interval, err := time.ParseDuration(value)
	if err != nil || interval < 0 {
		logger.Warn(fmt.Sprintf("DOCKTRINE_UPDATE_INTERVAL=%q is not a duration; checking every %s", value, DefaultUpdateInterval))
		return DefaultUpdateInterval
	}
	// More often than this mostly asks registries the same question again
	return max(interval, 15*time.Minute)
}

// updateState is what checks share: when each host may be asked next, the
// hosts that asked to be left alone for a while, and checks in progress.
type updateState struct {
	mu       sync.Mutex
	interval time.Duration
	// Checks run on their own, in the background and after deploys
	background bool
	nextSlot   map[string]time.Time
	cooldown   map[string]time.Time
	inflight   map[int64]chan struct{}
}

func newUpdateState() *updateState {
	return &updateState{
		interval: DefaultUpdateInterval,
		nextSlot: map[string]time.Time{},
		cooldown: map[string]time.Time{},
		inflight: map[int64]chan struct{}{},
	}
}

// gate waits for host's turn, or refuses while it is rate limiting us.
func (u *updateState) gate(ctx context.Context, host string) error {
	u.mu.Lock()
	now := time.Now()
	if until, ok := u.cooldown[host]; ok && now.Before(until) {
		u.mu.Unlock()
		return &errRateLimited{host: host, until: until}
	}
	slot := u.nextSlot[host]
	if slot.Before(now) {
		slot = now
	}
	u.nextSlot[host] = slot.Add(hostSpacing)
	u.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Until(slot)):
		return nil
	}
}

// noteRateLimit stops asking host until it said to come back, when err
// says it limited us.
func (u *updateState) noteRateLimit(host string, err error) {
	var limited *errRateLimited
	if !errors.As(err, &limited) {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if limited.until.After(u.cooldown[host]) {
		u.cooldown[host] = limited.until
	}
}

// UpdatesCheckable says whether update checks apply to an environment
// running spec. Previews are left out: the CLI rebuilds them from their
// branch, and they don't live long.
func UpdatesCheckable(env *database.Environment, spec *manifest.Spec) bool {
	return env != nil && spec != nil && !env.IsPreview &&
		(spec.Source != nil || spec.Image != "" || spec.Compose != "")
}

// UpdateInfo is a check as the UI shows it.
type UpdateInfo struct {
	*database.UpdateCheck
	// The setup command updating runs again, which a rollback can't undo
	Setup string `json:"setup,omitempty"`
}

// UpdateNotice returns what to show about updates for an environment, or
// nil when its last check is about a deployment that no longer runs.
func UpdateNotice(env *database.Environment, check *database.UpdateCheck, spec *manifest.Spec) *UpdateInfo {
	if check == nil || env.CurrentDeploymentID == nil || check.DeploymentID != *env.CurrentDeploymentID {
		return nil
	}
	return &UpdateInfo{UpdateCheck: check, Setup: updateRunsSetup(spec, check)}
}

// updateRunsSetup returns the setup command an update would run: when the
// commit changes, or when it runs on every deploy.
func updateRunsSetup(spec *manifest.Spec, check *database.UpdateCheck) string {
	if spec == nil || spec.Setup == nil || check.Status != database.UpdateAvailable {
		return ""
	}
	if spec.Setup.Always || (check.Git != nil && check.Git.Update) {
		return spec.Setup.Command
	}
	return ""
}

// CheckUpdates looks for newer versions of what env runs, saves what it
// found and returns it. It returns nil when there is nothing to check.
// Callers that arrive while env is being checked get that check's result.
func (d *Deployer) CheckUpdates(ctx context.Context, env *database.Environment) (*database.UpdateCheck, error) {
	u := d.updates
	u.mu.Lock()
	if wait, busy := u.inflight[env.ID]; busy {
		u.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return d.db.GetUpdateCheck(env.ID)
	}
	done := make(chan struct{})
	u.inflight[env.ID] = done
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		delete(u.inflight, env.ID)
		u.mu.Unlock()
		close(done)
	}()

	if env.CurrentDeploymentID == nil {
		return nil, nil
	}
	dep, err := d.db.GetDeployment(*env.CurrentDeploymentID)
	if err != nil || dep == nil || !UpdatesCheckable(env, dep.Spec) {
		return nil, err
	}
	prev, err := d.db.GetUpdateCheck(env.ID)
	if err != nil {
		return nil, err
	}
	if prev != nil && prev.DeploymentID == dep.ID && time.Since(prev.CheckedAt) < recheckAfter {
		return prev, nil
	}

	check := &database.UpdateCheck{EnvironmentID: env.ID, DeploymentID: dep.ID, CheckedAt: time.Now().UTC()}
	if prev != nil && prev.DeploymentID != dep.ID {
		prev = nil
	}

	problems := []string{}
	if src := dep.Spec.Source; src != nil {
		var prevGit *database.GitUpdate
		if prev != nil {
			prevGit = prev.Git
		}
		git, err := d.checkGit(ctx, src, prevGit)
		if err != nil {
			problems = append(problems, err.Error())
		}
		check.Git = git
	}
	images, err := d.checkImages(ctx, env, dep)
	if err != nil {
		problems = append(problems, err.Error())
	}
	check.Images = images

	check.Status, check.Error = summarizeCheck(check.Git, check.Images, problems)
	if check.Status == database.UpdateError {
		if prev != nil {
			check.Failures = prev.Failures
		}
		check.Failures++
		check.NextCheckAt = check.CheckedAt.Add(retryDelay(check.Failures))
	} else {
		check.NextCheckAt = check.CheckedAt.Add(u.interval)
	}
	if err := d.db.SaveUpdateCheck(check); err != nil {
		return nil, err
	}
	return check, nil
}

// retryDelay spaces out checks that keep failing.
func retryDelay(failures int) time.Duration {
	delay := updateRetryBase
	for i := 1; i < failures && delay < updateRetryMax; i++ {
		delay *= 2
	}
	return min(delay, updateRetryMax)
}

// summarizeCheck turns what a check found into its status, and the problems
// it ran into into one message.
func summarizeCheck(git *database.GitUpdate, images []database.ImageUpdate, problems []string) (string, string) {
	compared, update := false, false
	if git != nil && git.Latest != "" {
		compared = compared || git.RefKind != "commit"
		update = update || git.Update
	}
	for _, img := range images {
		if img.Error != "" {
			problems = append(problems, img.Image+": "+img.Error)
			continue
		}
		compared = true
		update = update || img.Update
	}
	message := strings.Join(problems, "; ")
	switch {
	case update:
		return database.UpdateAvailable, message
	case len(problems) > 0 && !compared:
		return database.UpdateError, message
	case !compared:
		return database.UpdatePinned, message
	}
	return database.UpdateCurrent, message
}

// Update redeploys what env runs at its newest version: the head of its
// Git ref, and its images pulled again by tag. It is an ordinary
// deployment of the current spec.
func (d *Deployer) Update(env *database.Environment, createdBy string) (*database.Deployment, error) {
	if env.CurrentDeploymentID == nil {
		return nil, fmt.Errorf("nothing runs in %s/%s to update", env.App, env.Name)
	}
	current, err := d.db.GetDeployment(*env.CurrentDeploymentID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Spec == nil {
		return nil, fmt.Errorf("the running deployment of %s/%s has no spec to update", env.App, env.Name)
	}
	spec, err := updatedSpec(current.Spec)
	if err != nil {
		return nil, err
	}
	return d.Start(spec, createdBy+" (update)")
}

// updatedSpec copies spec for an update. Without the recorded commit, the
// deploy fetches the latest of the ref and records the new commit; that new
// commit is what reruns the setup command. Image deploys always pull, so an
// image by tag needs no change.
func updatedSpec(spec *manifest.Spec) (*manifest.Spec, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	var copied manifest.Spec
	if err := json.Unmarshal(data, &copied); err != nil {
		return nil, err
	}
	if src := copied.Source; src != nil && !looksLikeCommit(src.Ref) {
		src.Commit = ""
	}
	if copied.Setup != nil {
		copied.Setup.Force = false
	}
	return &copied, nil
}

// StartUpdateChecks checks environments for updates in the background as
// they fall due. With interval 0, only checks asked for run.
func (d *Deployer) StartUpdateChecks(ctx context.Context, interval time.Duration) {
	d.updates.mu.Lock()
	if interval > 0 {
		d.updates.interval = interval
		d.updates.background = true
	}
	d.updates.mu.Unlock()
	if interval <= 0 {
		return
	}
	go func() {
		// Not right away: a server restarting in a loop shouldn't ask
		// every registry each time
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			d.checkDueUpdates(ctx)
			timer.Reset(updateTick)
		}
	}()
}

// checkDueUpdates checks, one at a time, the environments whose last check
// is old, failed a while ago, or was about an earlier deployment.
func (d *Deployer) checkDueUpdates(ctx context.Context) {
	envs, err := d.db.ListEnvironments()
	if err != nil {
		logger.Error(err, "Failed to list environments for update checks")
		return
	}
	checks, err := d.db.ListUpdateChecks()
	if err != nil {
		logger.Error(err, "Failed to read update checks")
		return
	}
	now := time.Now()
	for i := range envs {
		env := &envs[i]
		if env.CurrentDeploymentID == nil || env.IsPreview {
			continue
		}
		if c := checks[env.ID]; c != nil && c.DeploymentID == *env.CurrentDeploymentID && now.Before(c.NextCheckAt) {
			continue
		}
		// It is checked once the deployment in progress settles
		if active, err := d.db.HasActiveDeployment(env.ID); err != nil || active {
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		if _, err := d.CheckUpdates(checkCtx, env); err != nil {
			logger.Warn(fmt.Sprintf("Update check of %s/%s failed: %v", env.App, env.Name, err))
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
	}
}

// checkAfterDeploy checks an environment that just deployed, so a notice
// about the version it replaced goes away.
func (d *Deployer) checkAfterDeploy(app, name string) {
	d.updates.mu.Lock()
	background := d.updates.background
	d.updates.mu.Unlock()
	if !background {
		return
	}
	env, err := d.db.GetEnvironment(app, name)
	if err != nil || env == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	if _, err := d.CheckUpdates(ctx, env); err != nil {
		logger.Warn(fmt.Sprintf("Update check of %s/%s failed: %v", env.App, env.Name, err))
	}
}

// Git

var commitPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// looksLikeCommit says whether a ref names a commit rather than a branch or
// tag. A full hash always does; a short one, when no branch or tag has
// that name.
func looksLikeCommit(ref string) bool {
	return commitPattern.MatchString(ref)
}

// sameCommit compares commits that may be abbreviated.
func sameCommit(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	if len(a) < 7 || len(b) < 7 {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

// lsRemotePatterns are the refs to ask a remote for to resolve ref. A tag's
// own hash is the tag object's when it is annotated; ^{} asks for the
// commit it points to, which is what a deploy records.
func lsRemotePatterns(ref string) []string {
	switch {
	case ref == "":
		return []string{"HEAD"}
	case strings.HasPrefix(ref, "refs/"):
		return []string{ref, ref + "^{}"}
	}
	return []string{"refs/heads/" + ref, "refs/tags/" + ref, "refs/tags/" + ref + "^{}"}
}

// parseLsRemote reads `git ls-remote` output: a hash and a ref per line.
func parseLsRemote(out string) map[string]string {
	refs := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		hash, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || !commitPattern.MatchString(hash) || name == "" {
			continue
		}
		refs[strings.TrimSpace(name)] = hash
	}
	return refs
}

// resolveRemoteRef finds the commit ref points to among a remote's refs,
// and says whether it is a "branch", "tag" or the "default" branch. A
// branch wins over a tag of the same name, as it does for git fetch.
func resolveRemoteRef(refs map[string]string, ref string) (commit, kind string, ok bool) {
	pick := func(name string) (string, bool) {
		if peeled, ok := refs[name+"^{}"]; ok {
			return peeled, true
		}
		hash, ok := refs[name]
		return hash, ok
	}
	if ref == "" {
		commit, ok = refs["HEAD"]
		return commit, "default", ok
	}
	candidates := []struct{ name, kind string }{
		{"refs/heads/" + ref, "branch"},
		{"refs/tags/" + ref, "tag"},
	}
	if strings.HasPrefix(ref, "refs/") {
		kind := "branch"
		if strings.HasPrefix(ref, "refs/tags/") {
			kind = "tag"
		}
		candidates = []struct{ name, kind string }{{ref, kind}}
	}
	for _, c := range candidates {
		if commit, ok := pick(c.name); ok {
			return commit, c.kind, true
		}
	}
	return "", "", false
}

// lsRemote asks a remote for the refs that may resolve ref.
func lsRemote(ctx context.Context, repo, ref string) (map[string]string, error) {
	args := append([]string{"ls-remote", repo}, lsRemotePatterns(ref)...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitEnviron()
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			stderr = exit.Stderr
		}
		return nil, gitError("ls-remote", err, stderr)
	}
	return parseLsRemote(string(out)), nil
}

func (d *Deployer) checkGit(ctx context.Context, src *manifest.GitSource, prev *database.GitUpdate) (*database.GitUpdate, error) {
	git := &database.GitUpdate{Repo: src.Repo, Ref: src.Ref, Current: src.Commit}

	// A commit never moves, so there is nothing to ask
	if len(src.Ref) == 40 && looksLikeCommit(src.Ref) {
		git.RefKind, git.Latest = "commit", src.Ref
		return git, nil
	}

	host := "git"
	if u, err := url.Parse(src.Repo); err == nil {
		host = u.Host
	}
	if err := d.updates.gate(ctx, host); err != nil {
		return git, err
	}
	refs, err := lsRemote(ctx, src.Repo, src.Ref)
	if err != nil {
		return git, err
	}
	latest, kind, ok := resolveRemoteRef(refs, src.Ref)
	if !ok {
		if looksLikeCommit(src.Ref) {
			git.RefKind, git.Latest = "commit", src.Commit
			return git, nil
		}
		return git, fmt.Errorf("%s has no branch or tag named %q any more", src.Repo, src.Ref)
	}
	git.RefKind, git.Latest = kind, latest
	if git.Current == "" || sameCommit(git.Current, latest) {
		return git, nil
	}

	git.Update = true
	git.CompareURL = compareURL(src.Repo, git.Current, latest)
	// Counting the commits costs a request to the forge's API, so it is
	// asked once per new head
	if prev != nil && prev.Current == git.Current && prev.Latest == latest {
		git.Behind = prev.Behind
	} else {
		git.Behind = d.commitsBehind(ctx, src.Repo, git.Current, latest)
	}
	return git, nil
}

// forgePath splits a repository URL on a forge whose compare views and APIs
// are known into the forge's host and the repository's path.
func forgePath(repo string) (host, path string, ok bool) {
	u, err := url.Parse(repo)
	if err != nil || u.Scheme != "https" {
		return "", "", false
	}
	host = strings.ToLower(u.Host)
	switch host {
	case "github.com", "gitlab.com", "codeberg.org":
	default:
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if strings.Count(path, "/") < 1 || strings.ContainsAny(path, "?#") {
		return "", "", false
	}
	return host, path, true
}

// compareURL links to the forge's view of the commits between two, for
// GitHub, GitLab and Codeberg.
func compareURL(repo, from, to string) string {
	host, path, ok := forgePath(repo)
	if !ok {
		return ""
	}
	if host == "gitlab.com" {
		return fmt.Sprintf("https://gitlab.com/%s/-/compare/%s...%s", path, from, to)
	}
	return fmt.Sprintf("https://%s/%s/compare/%s...%s", host, path, from, to)
}

// commitsBehind asks GitHub or Codeberg how many commits to is ahead of
// from. It returns 0 when it can't tell, which reads as "a newer commit".
func (d *Deployer) commitsBehind(ctx context.Context, repo, from, to string) int {
	host, path, ok := forgePath(repo)
	if !ok || strings.Count(path, "/") != 1 {
		return 0
	}
	var api, field string
	switch host {
	case "github.com":
		host, field = "api.github.com", "ahead_by"
		api = fmt.Sprintf("https://api.github.com/repos/%s/compare/%s...%s?per_page=1", path, from, to)
	case "codeberg.org":
		field = "total_commits"
		api = fmt.Sprintf("https://codeberg.org/api/v1/repos/%s/compare/%s...%s?limit=1", path, from, to)
	default:
		return 0
	}
	if err := d.updates.gate(ctx, host); err != nil {
		return 0
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Accept", "application/json")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return 0
	}
	defer res.Body.Close()
	// GitHub allows 60 requests an hour without a token, and says when
	// they run out with a 403
	if res.StatusCode == http.StatusTooManyRequests ||
		(res.StatusCode == http.StatusForbidden && res.Header.Get("X-RateLimit-Remaining") == "0") {
		limited := rateLimited(host, res.Header)
		if reset, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			limited.until = time.Unix(reset, 0)
		}
		d.updates.noteRateLimit(host, limited)
		return 0
	}
	if res.StatusCode != http.StatusOK {
		return 0
	}
	// GitHub's status is "diverged" when the ref was rewritten; a count
	// of commits ahead then says little
	if host == "api.github.com" {
		status, n := jsonFields(io.LimitReader(res.Body, 4<<20), "status", field)
		if status != "ahead" {
			return 0
		}
		count, _ := strconv.Atoi(n)
		return count
	}
	_, n := jsonFields(io.LimitReader(res.Body, 4<<20), "", field)
	count, _ := strconv.Atoi(n)
	return count
}

// jsonFields reads two top-level fields of a JSON object as text, stopping
// once it has them: compare responses go on to list every changed file.
func jsonFields(r io.Reader, first, second string) (string, string) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return "", ""
	}
	values := map[string]string{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		key, _ := tok.(string)
		var value any
		if err := dec.Decode(&value); err != nil {
			break
		}
		if key == first || key == second {
			values[key] = fmt.Sprint(value)
		}
		if (first == "" || values[first] != "") && values[second] != "" {
			break
		}
	}
	return values[first], values[second]
}

// Images

// runningImage is an image an environment's containers run, by the
// reference they were started from.
type runningImage struct {
	ref      string
	id       string
	services []string
}

// checkImages compares the images env's containers run with what their tags
// point to now. Images by digest can't move and are left out, and so are
// images built on the node, which no registry has.
func (d *Deployer) checkImages(ctx context.Context, env *database.Environment, dep *database.Deployment) ([]database.ImageUpdate, error) {
	server, err := d.db.ResolveServer(env.Server)
	if err != nil {
		return nil, err
	}
	cli, err := connect(server)
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	running, err := runningImages(ctx, cli, env, dep)
	if err != nil {
		return nil, err
	}

	updates := []database.ImageUpdate{}
	for _, img := range running {
		if len(updates) >= maxImagesPerCheck {
			break
		}
		named, err := reference.ParseNormalizedNamed(img.ref)
		if err != nil {
			continue
		}
		if _, pinned := named.(reference.Digested); pinned {
			continue
		}
		digests, err := repoDigests(ctx, cli, img.id, named)
		if err != nil {
			return updates, err
		}
		if len(digests) == 0 {
			continue
		}
		update := database.ImageUpdate{
			Service: strings.Join(img.services, ", "),
			Image:   reference.FamiliarString(reference.TagNameOnly(named)),
			Current: digests[0],
		}
		latest, err := d.latestDigest(ctx, img.ref)
		if err != nil {
			update.Error = err.Error()
		} else {
			update.Latest = latest
			update.Update = !contains(digests, latest)
			if !update.Update {
				update.Current = latest
			}
		}
		updates = append(updates, update)
	}
	return updates, nil
}

// runningImages lists the images env runs: for an image deploy, the one its
// current container runs; for compose, every service's.
func runningImages(ctx context.Context, cli *client.Client, env *database.Environment, dep *database.Deployment) ([]runningImage, error) {
	containers, err := listWorkload(ctx, cli, env.App, env.Name)
	if err != nil {
		return nil, err
	}
	imageDeploy := dep.Spec.Image != ""
	current := strconv.FormatInt(dep.ID, 10)

	byKey := map[string]*runningImage{}
	order := []string{}
	for _, c := range containers {
		// Containers of an older deployment are on their way out
		if imageDeploy && c.Labels[LabelDeployment] != current {
			continue
		}
		inspect, err := cli.ContainerInspect(ctx, c.ID)
		if err != nil || inspect.Config == nil {
			continue
		}
		ref := inspect.Config.Image
		if imageDeploy {
			ref = dep.Spec.Image
		}
		key := ref + " " + inspect.Image
		img, ok := byKey[key]
		if !ok {
			img = &runningImage{ref: ref, id: inspect.Image}
			byKey[key] = img
			order = append(order, key)
		}
		if service := c.Labels["com.docker.compose.service"]; service != "" && !contains(img.services, service) {
			img.services = append(img.services, service)
		}
	}

	images := make([]runningImage, 0, len(order))
	for _, key := range order {
		img := byKey[key]
		sort.Strings(img.services)
		images = append(images, *img)
	}
	return images, nil
}

// repoDigests returns the registry digests an image is known by in named's
// repository; none for images that were built rather than pulled.
func repoDigests(ctx context.Context, cli *client.Client, id string, named reference.Named) ([]string, error) {
	inspect, _, err := cli.ImageInspectWithRaw(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("reading image %s on the node: %w", short(strings.TrimPrefix(id, "sha256:")), err)
	}
	return matchingDigests(inspect.RepoDigests, named), nil
}

// matchingDigests picks the digests of repoDigests ("nginx@sha256:...") that
// are of named's repository.
func matchingDigests(repoDigests []string, named reference.Named) []string {
	digests := []string{}
	for _, rd := range repoDigests {
		parsed, err := reference.ParseNormalizedNamed(rd)
		if err != nil {
			continue
		}
		digested, ok := parsed.(reference.Digested)
		if ok && parsed.Name() == named.Name() {
			digests = append(digests, digested.Digest().String())
		}
	}
	return digests
}

// latestDigest asks the registry which digest ref's tag points to now.
func (d *Deployer) latestDigest(ctx context.Context, ref string) (string, error) {
	r, named, version, err := d.registryFor(ref)
	if err != nil {
		return "", err
	}
	host := registryHost(named)
	if err := d.updates.gate(ctx, host); err != nil {
		return "", err
	}
	digest, err := r.digest(ctx, version)
	d.updates.noteRateLimit(host, err)
	return digest, err
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
