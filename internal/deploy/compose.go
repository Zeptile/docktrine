package deploy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// deployCompose runs `docker compose up` against the node with an override
// file that adds Docktrine labels, ingress routing and env/secrets.
//
// Env and secrets are written to .env, so the compose file can reference
// them as ${VAR}, and are also injected into the exposed service.
func (d *Deployer) deployCompose(ctx context.Context, t target, log *deployLog) error {
	services, err := composeServices(t.spec.Compose)
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

	dir, err := os.MkdirTemp("", "docktrine-compose-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	override, err := t.composeOverride(services, exposed)
	if err != nil {
		return err
	}

	files := map[string][]byte{
		"docker-compose.yml":     []byte(t.spec.Compose),
		"docktrine.override.yml": override,
		".env":                   dotenv(t.env),
		"config/config.json":     d.dockerConfigJSON(),
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
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
		"--project-directory", dir,
		"-f", filepath.Join(dir, "docker-compose.yml"),
		"-f", filepath.Join(dir, "docktrine.override.yml"),
	}
	environ := append(os.Environ(),
		"DOCKER_HOST="+t.server.Host,
		"DOCKER_CONFIG="+filepath.Join(dir, "config"),
	)

	log.Printf("Pulling images")
	if err := runLogged(ctx, log, environ, append(base, "pull", "--ignore-buildable")...); err != nil {
		return fmt.Errorf("docker compose pull: %w", err)
	}

	log.Printf("Starting services")
	up := append(base, "up", "--detach", "--remove-orphans", "--wait", "--wait-timeout", "120")
	if err := runLogged(ctx, log, environ, up...); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
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
		service := map[string]any{}
		if name == exposed {
			for k, v := range t.routingLabels() {
				labels[k] = v
			}
			// Listing default keeps the service on the project network
			// alongside its siblings, as without an override.
			service["networks"] = []string{"default", Network}
			if len(t.env) > 0 {
				service["environment"] = t.env
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
