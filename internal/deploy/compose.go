package deploy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"gopkg.in/yaml.v3"
)

// deployCompose runs `docker compose up` against the node with an override
// file that adds Docktrine labels, ingress routing and env/secrets.
//
// Env and secrets are written to .env, so the compose file can reference
// them as ${VAR}, and are also injected into the exposed service.
func (d *Deployer) deployCompose(ctx context.Context, t target, log *deployLog) error {
	work, err := os.MkdirTemp("", "docktrine-compose-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	// The project directory: the checkout of a stack from Git, or a
	// directory holding just the compose file sent by the CLI
	root := filepath.Join(work, "project")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	composePath := filepath.Join(root, "docker-compose.yml")
	compose := t.spec.Compose

	if src := t.spec.Source; src != nil {
		log.Printf("Fetching %s%s", src.Repo, refSuffix(src))
		commit, err := fetchRepo(ctx, src, root)
		if err != nil {
			return err
		}
		file, err := findComposeFile(root, src.Path)
		if err != nil {
			return err
		}
		log.Printf("Using %s at %s", file, commit[:min(12, len(commit))])
		// Redeploying or rolling back to this deployment runs this commit
		if src.Commit != commit {
			src.Commit = commit
			if err := d.db.UpdateDeploymentSpec(t.deploymentID, src.Label(), t.spec); err != nil {
				return err
			}
		}
		composePath = filepath.Join(root, filepath.FromSlash(file))
		data, err := os.ReadFile(composePath)
		if err != nil {
			return err
		}
		compose = string(data)
	} else if err := os.WriteFile(composePath, []byte(compose), 0600); err != nil {
		return err
	}
	projectDir := filepath.Dir(composePath)

	services, err := composeServices(compose)
	if err != nil {
		return err
	}
	exposed := ""
	if t.spec.Expose != nil {
		exposed = t.spec.Expose.Service
		if _, ok := services[exposed]; !ok && len(t.spec.Hostnames) > 0 {
			return fmt.Errorf("expose.service %q is not a service in the compose file", exposed)
		}
	}

	override, err := t.composeOverride(services, exposed)
	if err != nil {
		return err
	}

	// The repository's own .env stays as written, since its values may
	// refer to each other; values set in Docktrine follow it, and the last
	// assignment of a key wins
	env, err := os.ReadFile(filepath.Join(projectDir, ".env"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(env) > 0 {
		env = append(env, []byte("\n# Set in Docktrine\n")...)
	}
	env = append(env, dotenv(t.env)...)

	files := map[string][]byte{
		filepath.Join(projectDir, ".env"):             env,
		filepath.Join(work, "docktrine.override.yml"): override,
		filepath.Join(work, "config", "config.json"):  d.dockerConfigJSON(),
	}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}

	if err := ensureNetworkOn(ctx, t); err != nil {
		return err
	}

	base := []string{"compose",
		"--project-name", t.project(),
		"--project-directory", projectDir,
		"-f", composePath,
		"-f", filepath.Join(work, "docktrine.override.yml"),
	}
	environ := append(os.Environ(),
		"DOCKER_HOST="+t.server.Host,
		"DOCKER_CONFIG="+filepath.Join(work, "config"),
	)

	waitTimeout := "120"
	if t.spec.Source != nil {
		nodeDir := StacksDir() + "/" + t.project()
		resolved, err := resolveStack(ctx, environ, base, root, nodeDir)
		if err != nil {
			return err
		}
		resolvedPath := filepath.Join(work, "resolved.json")
		if err := os.WriteFile(resolvedPath, resolved, 0600); err != nil {
			return err
		}

		log.Printf("Copying the stack's files to %s on %s", nodeDir, t.server.Name)
		cli, err := connect(t.server)
		if err != nil {
			return err
		}
		envFile := filepath.ToSlash(mustRel(root, filepath.Join(projectDir, ".env")))
		err = syncStack(ctx, cli, root, nodeDir, func(rel string) bool { return rel == envFile })
		cli.Close()
		if err != nil {
			return err
		}

		base = []string{"compose",
			"--project-name", t.project(),
			"--project-directory", projectDir,
			"-f", resolvedPath,
		}
		// Stacks that aren't ours tend to be bigger and slower to start
		waitTimeout = "300"
	}

	log.Printf("Pulling images")
	if err := runLogged(ctx, log, environ, append(base, "pull", "--ignore-buildable")...); err != nil {
		return fmt.Errorf("docker compose pull: %w", err)
	}

	log.Printf("Starting services")
	up := append(base, "up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", waitTimeout)
	if err := runLogged(ctx, log, environ, up...); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}

	if exposed == "" || len(t.spec.Hostnames) == 0 {
		return nil
	}
	return waitComposeRouted(ctx, t, exposed, log)
}

func refSuffix(src *manifest.GitSource) string {
	switch {
	case src.Commit != "":
		return " at " + src.Commit[:min(12, len(src.Commit))]
	case src.Ref != "":
		return " (" + src.Ref + ")"
	}
	return ""
}

func mustRel(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return rel
}

// waitComposeRouted waits until the ingress routes to the exposed service's
// containers, so a successful deploy means its hostnames answer.
func waitComposeRouted(ctx context.Context, t target, service string, log *deployLog) error {
	cli, err := connect(t.server)
	if err != nil {
		return err
	}
	defer cli.Close()

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+t.project()),
			filters.Arg("label", "com.docker.compose.service="+service),
		),
	})
	if err != nil {
		return err
	}
	ids := []string{}
	for _, c := range containers {
		ids = append(ids, c.ID)
	}

	err = waitRouted(ctx, cli, t, ids, log)
	if errors.Is(err, errNoIngressAPI) {
		return nil
	}
	if err != nil {
		// Compose has already replaced the containers; there is nothing
		// older to fall back to
		return fmt.Errorf("%w; the new containers are running but not reachable through the hostnames", err)
	}
	return nil
}

func composeServices(compose string) (map[string]any, error) {
	var parsed struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &parsed); err != nil {
		return nil, fmt.Errorf("invalid compose file: %w", err)
	}
	if len(parsed.Services) == 0 {
		return nil, fmt.Errorf("compose file has no services")
	}
	return parsed.Services, nil
}

func (t target) composeOverride(services map[string]any, exposed string) ([]byte, error) {
	out := map[string]any{}
	for name := range services {
		labels := t.labels()
		// A label that changes every deployment would make compose recreate
		// every service, even unchanged ones; compose stops before it starts,
		// so that is downtime. The deployment is tracked in the database.
		delete(labels, LabelDeployment)
		service := map[string]any{}
		if name == exposed {
			for k, v := range t.routingLabels() {
				labels[k] = v
			}
			// Listing default keeps the service on the project network
			// alongside its siblings, as without an override.
			service["networks"] = []string{"default", Network}
			if len(t.env) > 0 {
				// Compose interpolates the override too; $$ keeps a $ in a
				// value literal
				env := map[string]string{}
				for k, v := range t.env {
					env[k] = strings.ReplaceAll(v, "$", "$$")
				}
				service["environment"] = env
			}
		}
		service["labels"] = labels
		out[name] = service
	}

	doc := map[string]any{"services": out}
	if exposed != "" {
		doc["networks"] = map[string]any{
			Network: map[string]any{"external": true},
		}
	}
	return yaml.Marshal(doc)
}

func dotenv(values map[string]string) []byte {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		// Single quotes keep compose from interpolating the value itself.
		fmt.Fprintf(&b, "%s='%s'\n", k, strings.ReplaceAll(values[k], "'", `'\''`))
	}
	return []byte(b.String())
}

func ensureNetworkOn(ctx context.Context, t target) error {
	cli, err := connect(t.server)
	if err != nil {
		return err
	}
	defer cli.Close()
	return ensureNetwork(ctx, cli)
}

// runLogged runs the docker CLI and copies its output into the deployment log.
func runLogged(ctx context.Context, log *deployLog, environ []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = environ

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			if line := strings.TrimSpace(scanner.Text()); line != "" {
				log.Printf("  | %s", line)
			}
		}
	}()

	err := cmd.Run()
	pw.Close()
	wg.Wait()
	if err != nil && strings.Contains(err.Error(), "executable file not found") {
		return fmt.Errorf("the docker CLI (with the compose plugin) must be installed on the Docktrine server")
	}
	return err
}
