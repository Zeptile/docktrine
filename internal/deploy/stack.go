package deploy

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"gopkg.in/yaml.v3"
)

// Stacks from a Git repository usually mount files that sit next to their
// compose file (./nginx.conf:/etc/nginx/nginx.conf). Compose runs here, but
// the daemon that mounts those paths is on the node, so the repository is
// copied to the node, and bind sources inside it are pointed at that copy.

// helperImage runs on nodes to receive stack files and to delete them.
const helperImage = "busybox:1.37"

// composeCandidates are the names `docker compose` looks for, in its order.
var composeCandidates = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// StacksDir is where nodes keep the files of stacks from Git, one directory
// per environment. Files written there by the stack itself, e.g. uploads
// under ./data, stay across deploys.
func StacksDir() string {
	if dir := os.Getenv("DOCKTRINE_STACKS_DIR"); dir != "" {
		return strings.TrimRight(dir, "/")
	}
	return "/var/lib/docktrine/stacks"
}

// fetchRepo fetches one commit of src into dir, which must be empty, and
// returns the commit hash.
func fetchRepo(ctx context.Context, src *manifest.GitSource, dir string) (string, error) {
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			// Never wait for a password: public repositories only
			"GIT_TERMINAL_PROMPT=0",
			"GIT_ASKPASS=/bin/false",
			// Submodules can't reach file:// or ext:: transports either
			"GIT_ALLOW_PROTOCOL=https",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_LFS_SKIP_SMUDGE=1",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(err.Error(), "executable file not found") {
				return "", fmt.Errorf("git must be installed on the Docktrine server")
			}
			msg := strings.TrimSpace(string(out))
			if strings.Contains(msg, "terminal prompts disabled") || strings.Contains(msg, "could not read Username") {
				msg = "the repository is private or doesn't exist; only public repositories are supported"
			}
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return strings.TrimSpace(string(out)), nil
	}

	want := src.Commit
	if want == "" {
		want = src.Ref
	}
	if want == "" {
		want = "HEAD"
	}

	steps := [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", src.Repo},
		{"fetch", "--quiet", "--depth", "1", "origin", want},
		{"checkout", "--quiet", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if _, err := git(args...); err != nil {
			if args[0] == "fetch" && src.Ref != "" && src.Commit == "" {
				return "", fmt.Errorf("couldn't fetch %q from %s: %w", src.Ref, src.Repo, err)
			}
			return "", err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err == nil {
		if _, err := git("submodule", "update", "--quiet", "--init", "--recursive", "--depth", "1"); err != nil {
			return "", err
		}
	}
	return git("rev-parse", "HEAD")
}

// findComposeFile returns the compose file of a checked out repository,
// relative to its root.
func findComposeFile(root, want string) (string, error) {
	if want != "" {
		clean := filepath.Clean(want)
		info, err := os.Stat(filepath.Join(root, clean))
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("%s is not a file in the repository", want)
		}
		return filepath.ToSlash(clean), nil
	}
	for _, name := range composeCandidates {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("no compose file at the root of the repository (looked for %s); name the file to use",
		strings.Join(composeCandidates, ", "))
}

// listComposeFiles finds likely compose files near the root of a checkout,
// to offer when the root has none.
func listComposeFiles(root string) []string {
	found := []string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || strings.Count(rel, string(filepath.Separator)) >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if (strings.HasPrefix(name, "docker-compose") || strings.HasPrefix(name, "compose")) &&
			(strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")) {
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(found)
	return found
}

// StackInfo describes a compose stack, for filling in the launch form.
type StackInfo struct {
	ComposeFile  string          `json:"compose_file,omitempty"`
	ComposeFiles []string        `json:"compose_files,omitempty"`
	Commit       string          `json:"commit,omitempty"`
	Services     []StackService  `json:"services"`
	Variables    []StackVariable `json:"variables"`
	// The service and port hostnames most likely route to
	Service string `json:"service,omitempty"`
	Port    int    `json:"port,omitempty"`
	// A setup command the stack likely needs, from its install script
	Setup string `json:"setup,omitempty"`
	// What it needs of the node, for the preflight checks
	Requirements *Requirements `json:"requirements,omitempty"`
}

type StackService struct {
	Name  string      `json:"name"`
	Image string      `json:"image,omitempty"`
	Build bool        `json:"build,omitempty"`
	Ports []StackPort `json:"ports"`
}

type StackPort struct {
	Target int `json:"target"`
	// The port it is published on, on the node, if it is
	Published string `json:"published,omitempty"`
}

// StackVariable is a ${VAR} the compose file reads, or a key of its .env.
type StackVariable struct {
	Name string `json:"name"`
	// What applies when it isn't set: the repository's .env, or the
	// compose file's ${VAR:-default}
	Default    string `json:"default,omitempty"`
	HasDefault bool   `json:"has_default,omitempty"`
	// A starting value from .env.example, which applies only if set
	Suggested string `json:"suggested,omitempty"`
	// ${VAR:?message}: compose refuses to start without it
	Required    bool   `json:"required,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Description string `json:"description,omitempty"`
}

// InspectStack reads a compose stack from Git, or from compose text when
// src is nil.
func InspectStack(ctx context.Context, src *manifest.GitSource, compose string) (*StackInfo, error) {
	info := &StackInfo{}
	var dotEnv, example []envLine

	if src != nil {
		if err := src.Validate(); err != nil {
			return nil, err
		}
		dir, err := os.MkdirTemp("", "docktrine-inspect-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)

		if info.Commit, err = fetchRepo(ctx, src, dir); err != nil {
			return nil, err
		}
		file, err := findComposeFile(dir, src.Path)
		if err != nil {
			info.ComposeFiles = listComposeFiles(dir)
			if len(info.ComposeFiles) > 0 && src.Path == "" {
				return info, nil
			}
			return nil, err
		}
		info.ComposeFile = file
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return nil, err
		}
		compose = string(data)

		projectDir := filepath.Dir(filepath.Join(dir, file))
		info.Setup = suggestSetup(projectDir)
		if req, ok := scriptRequirements(projectDir); ok {
			info.Requirements = &req
		}
		dotEnv = readEnvFile(filepath.Join(projectDir, ".env"))
		for _, name := range []string{".env.example", ".env.sample", ".env.dist", "example.env", ".env.template"} {
			if lines := readEnvFile(filepath.Join(projectDir, name)); len(lines) > 0 {
				example = lines
				break
			}
		}
	}

	services, order, err := parseComposeServices(compose)
	if err != nil {
		return nil, err
	}
	for _, name := range order {
		info.Services = append(info.Services, describeService(name, services[name]))
	}
	info.Service, info.Port = suggestExposed(info.Services)
	if mem, cpus := composeReservations(services); mem > 0 || cpus > 0 {
		if info.Requirements == nil {
			info.Requirements = &Requirements{}
		}
		info.Requirements.ReservedMemoryMB, info.Requirements.ReservedCPUs = mem, cpus
	}
	info.Variables = stackVariables(compose, dotEnv, example)
	return info, nil
}

// parseComposeServices returns the services in file order.
func parseComposeServices(compose string) (map[string]map[string]any, []string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
		return nil, nil, fmt.Errorf("invalid compose file: %w", err)
	}
	var parsed struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	if err := doc.Decode(&parsed); err != nil {
		return nil, nil, fmt.Errorf("invalid compose file: %w", err)
	}
	if len(parsed.Services) == 0 {
		return nil, nil, fmt.Errorf("the compose file has no services")
	}

	order := []string{}
	if len(doc.Content) > 0 {
		root := doc.Content[0]
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value != "services" {
				continue
			}
			list := root.Content[i+1]
			for j := 0; j+1 < len(list.Content); j += 2 {
				order = append(order, list.Content[j].Value)
			}
		}
	}
	if len(order) != len(parsed.Services) {
		order = order[:0]
		for name := range parsed.Services {
			order = append(order, name)
		}
		sort.Strings(order)
	}
	return parsed.Services, order, nil
}

func describeService(name string, svc map[string]any) StackService {
	s := StackService{Name: name, Ports: []StackPort{}}
	s.Image, _ = svc["image"].(string)
	_, s.Build = svc["build"]

	seen := map[int]bool{}
	add := func(p StackPort) {
		if p.Target > 0 && !seen[p.Target] {
			seen[p.Target] = true
			s.Ports = append(s.Ports, p)
		}
	}
	if ports, ok := svc["ports"].([]any); ok {
		for _, p := range ports {
			add(parsePort(p))
		}
	}
	if expose, ok := svc["expose"].([]any); ok {
		for _, p := range expose {
			port := parsePort(p)
			port.Published = ""
			add(port)
		}
	}
	return s
}

// parsePort reads short ("127.0.0.1:8080:80/tcp", "80") and long port syntax.
func parsePort(p any) StackPort {
	switch v := p.(type) {
	case int:
		return StackPort{Target: v}
	case string:
		spec, _, _ := strings.Cut(v, "/")
		// The target is after the last colon outside ${...}
		depth, cut := 0, -1
		for i, r := range spec {
			switch r {
			case '{':
				depth++
			case '}':
				depth--
			case ':':
				if depth == 0 {
					cut = i
				}
			}
		}
		target, published := spec, ""
		if cut >= 0 {
			target, published = spec[cut+1:], spec[:cut]
			if i := strings.LastIndex(published, ":"); i >= 0 && !strings.Contains(published[i:], "}") {
				published = published[i+1:]
			}
		}
		n, _ := strconv.Atoi(strings.Split(target, "-")[0])
		return StackPort{Target: n, Published: published}
	case map[string]any:
		port := StackPort{}
		switch t := v["target"].(type) {
		case int:
			port.Target = t
		case string:
			port.Target, _ = strconv.Atoi(t)
		}
		if pub, ok := v["published"]; ok {
			port.Published = fmt.Sprint(pub)
		}
		return port
	}
	return StackPort{}
}

// suggestExposed guesses which service the public hostnames are for: the
// front door if one is named like it, else the first that has a port.
func suggestExposed(services []StackService) (string, int) {
	front := []string{"nginx", "caddy", "proxy", "web", "frontend", "app", "server", "ui", "gateway"}
	for _, want := range front {
		for _, s := range services {
			if len(s.Ports) > 0 && (s.Name == want || strings.HasSuffix(s.Name, "-"+want)) {
				return s.Name, pickPort(s.Ports)
			}
		}
	}
	for _, s := range services {
		if len(s.Ports) > 0 {
			return s.Name, pickPort(s.Ports)
		}
	}
	return "", 0
}

func pickPort(ports []StackPort) int {
	for _, want := range []int{80, 8080, 3000, 8000, 5000, 443} {
		for _, p := range ports {
			if p.Target == want {
				return want
			}
		}
	}
	return ports[0].Target
}

var (
	braced = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?+])([^}]*))?\}`)
	bare   = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	secret = regexp.MustCompile(`(?i)(PASSWORD|PASSWD|PASS$|SECRET|TOKEN|PRIVATE|(^|_)KEY$|API_?KEY|CREDENTIAL|DSN$|SALT)`)
)

func stackVariables(compose string, dotEnv, example []envLine) []StackVariable {
	vars := []StackVariable{}
	index := map[string]int{}
	get := func(name string) *StackVariable {
		if i, ok := index[name]; ok {
			return &vars[i]
		}
		index[name] = len(vars)
		vars = append(vars, StackVariable{Name: name, Secret: secret.MatchString(name)})
		return &vars[len(vars)-1]
	}

	// $$ is a literal dollar sign, not a variable
	text := strings.ReplaceAll(compose, "$$", "")
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, m := range braced.FindAllStringSubmatch(line, -1) {
			v := get(m[1])
			switch m[2] {
			case ":-", "-":
				if !v.HasDefault {
					v.Default, v.HasDefault = m[3], true
				}
			case ":?", "?":
				v.Required = true
				if v.Description == "" {
					v.Description = m[3]
				}
			}
		}
		for _, m := range bare.FindAllStringSubmatch(braced.ReplaceAllString(line, ""), -1) {
			get(m[1])
		}
	}

	for _, l := range dotEnv {
		v := get(l.key)
		v.Default, v.HasDefault, v.Required = l.value, true, false
		if l.comment != "" {
			v.Description = l.comment
		}
	}
	for _, l := range example {
		v := get(l.key)
		if !v.HasDefault || v.Default == "" {
			v.Suggested = l.value
		}
		if l.comment != "" && v.Description == "" {
			v.Description = l.comment
		}
	}
	return vars
}

type envLine struct {
	key, value, comment string
}

// readEnvFile parses a dotenv file, keeping the comment just above each key.
func readEnvFile(path string) []envLine {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	lines := []envLine{}
	comment := []string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			comment = comment[:0]
			continue
		}
		if strings.HasPrefix(line, "#") {
			text := strings.TrimSpace(strings.TrimLeft(line, "#"))
			// A commented-out assignment isn't a description, and the
			// comment above it was about it
			if commentedOut.MatchString(text) {
				comment = comment[:0]
			} else if text != "" {
				comment = append(comment, text)
			}
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		key = strings.TrimSpace(key)
		if !ok || !secretKeyPattern.MatchString(key) {
			comment = comment[:0]
			continue
		}
		lines = append(lines, envLine{key: key, value: unquote(strings.TrimSpace(value)), comment: strings.Join(comment, " ")})
		comment = comment[:0]
	}
	return lines
}

var (
	secretKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	commentedOut     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			return v[1 : end+1]
		}
	}
	// An unquoted value ends at a comment
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

// resolveStack runs `docker compose config` over the stack and its override
// and points bind mounts inside the checkout at nodeDir, the checkout's
// copy on the node. The result is a single, self-contained compose file.
func resolveStack(ctx context.Context, environ []string, args []string, root, nodeDir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", append(args, "config", "--format", "json")...)
	cmd.Env = environ
	var stderr strings.Builder
	cmd.Stderr = &stderr
	// Its output holds every secret; it never goes to the log
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose config: %s", strings.TrimSpace(stderr.String()))
	}

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("reading the resolved compose file: %w", err)
	}

	moved := func(source string) (string, bool) {
		rel, err := filepath.Rel(root, source)
		if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return source, false
		}
		return filepath.ToSlash(filepath.Join(nodeDir, rel)), true
	}

	services, _ := doc["services"].(map[string]any)
	for _, s := range services {
		svc, _ := s.(map[string]any)
		volumes, _ := svc["volumes"].([]any)
		for _, v := range volumes {
			vol, _ := v.(map[string]any)
			if vol["type"] != "bind" {
				continue
			}
			if source, ok := vol["source"].(string); ok {
				vol["source"], _ = moved(source)
			}
		}
	}
	// File-based configs and secrets are bind mounts too
	for _, section := range []string{"configs", "secrets"} {
		entries, _ := doc[section].(map[string]any)
		for _, e := range entries {
			entry, _ := e.(map[string]any)
			if file, ok := entry["file"].(string); ok {
				entry["file"], _ = moved(file)
			}
		}
	}
	return json.Marshal(doc)
}

// syncStack copies the checkout at root to dir on the node, through a
// helper container, so it works over any Docker transport. Files are added
// and replaced but never deleted: the stack may keep its data there.
func syncStack(ctx context.Context, cli *client.Client, root, dir string) error {
	if err := ensureImage(ctx, cli, helperImage); err != nil {
		return err
	}
	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:  helperImage,
			Cmd:    []string{"sleep", "600"},
			Labels: map[string]string{LabelRole: "helper"},
		},
		&container.HostConfig{Binds: []string{dir + ":/stack"}, NetworkMode: "none"},
		nil, nil, "")
	if err != nil {
		return fmt.Errorf("creating the helper container: %w", err)
	}
	defer removeContainer(cli, created.ID)
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting the helper container: %w", err)
	}

	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(writeTar(pw, root)) }()
	if err := cli.CopyToContainer(ctx, created.ID, "/stack", pr, container.CopyToContainerOptions{}); err != nil {
		pr.CloseWithError(err)
		return fmt.Errorf("copying the stack to the node: %w", err)
	}
	return nil
}

// removeStackFiles deletes an environment's stack directory on the node.
func removeStackFiles(ctx context.Context, cli *client.Client, project string) error {
	if err := ensureImage(ctx, cli, helperImage); err != nil {
		return err
	}
	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:  helperImage,
			Cmd:    []string{"rm", "-rf", "/stacks/" + project},
			Labels: map[string]string{LabelRole: "helper"},
		},
		&container.HostConfig{Binds: []string{StacksDir() + ":/stacks"}, NetworkMode: "none"},
		nil, nil, "")
	if err != nil {
		return err
	}
	defer removeContainer(cli, created.ID)

	wait, errs := cli.ContainerWait(ctx, created.ID, container.WaitConditionNextExit)
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return err
	}
	select {
	case res := <-wait:
		if res.StatusCode != 0 {
			return fmt.Errorf("removing %s/%s on the node exited with %d", StacksDir(), project, res.StatusCode)
		}
		return nil
	case err := <-errs:
		return err
	}
}

func ensureImage(ctx context.Context, cli *client.Client, ref string) error {
	if _, _, err := cli.ImageInspectWithRaw(ctx, ref); err == nil {
		return nil
	} else if !errdefs.IsNotFound(err) {
		return err
	}
	if err := pullImage(ctx, cli, ref, ""); err != nil {
		return fmt.Errorf("pulling %s: %w", ref, err)
	}
	return nil
}

func writeTar(w io.Writer, root string) error {
	tw := tar.NewWriter(w)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(p); err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = rel
		if d.IsDir() {
			header.Name += "/"
		}
		header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	return tw.Close()
}
