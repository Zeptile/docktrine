// Package manifest defines the docktrine.yaml format and the resolved
// deployment Spec that the CLI sends to the API.
package manifest

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const FileName = "docktrine.yaml"

// PreviewEnvironment is the environment key that opts an app into
// per-branch preview deployments.
const PreviewEnvironment = "preview"

type Manifest struct {
	App          string                 `yaml:"app"`
	Build        *Build                 `yaml:"build,omitempty"`
	Image        string                 `yaml:"image,omitempty"`
	Compose      string                 `yaml:"compose,omitempty"`
	Expose       *Expose                `yaml:"expose,omitempty"`
	Environments map[string]Environment `yaml:"environments"`
}

type Build struct {
	Dockerfile string            `yaml:"dockerfile,omitempty"`
	Context    string            `yaml:"context,omitempty"`
	Platform   string            `yaml:"platform,omitempty"`
	Args       map[string]string `yaml:"args,omitempty"`
}

// Expose names the compose service (and its port) that hostnames route to.
type Expose struct {
	Service string `yaml:"service" json:"service"`
	Port    int    `yaml:"port" json:"port"`
}

type Environment struct {
	Node        string            `yaml:"node"`
	Hostnames   []string          `yaml:"hostnames,omitempty"`
	Port        int               `yaml:"port,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Secrets     []string          `yaml:"secrets,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	Healthcheck string            `yaml:"healthcheck,omitempty"`
	TTL         string            `yaml:"ttl,omitempty"`
}

// Spec is one environment of a manifest, fully resolved and ready to deploy.
type Spec struct {
	App         string            `json:"app"`
	Environment string            `json:"environment"`
	Node        string            `json:"node"`
	Image       string            `json:"image,omitempty"`
	Compose     string            `json:"compose,omitempty"`
	Expose      *Expose           `json:"expose,omitempty"`
	Hostnames   []string          `json:"hostnames,omitempty"`
	Port        int               `json:"port,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Secrets     []string          `json:"secrets,omitempty"`
	Volumes     []string          `json:"volumes,omitempty"`
	Command     []string          `json:"command,omitempty"`
	Healthcheck string            `json:"healthcheck,omitempty"`
	Preview     bool              `json:"preview,omitempty"`
	Branch      string            `json:"branch,omitempty"`
	TTL         string            `json:"ttl,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", FileName, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) Validate() error {
	if !namePattern.MatchString(m.App) {
		return fmt.Errorf("app: %q must be lowercase letters, digits and dashes", m.App)
	}

	sources := 0
	for _, set := range []bool{m.Build != nil, m.Image != "", m.Compose != ""} {
		if set {
			sources++
		}
	}
	if sources != 1 {
		return fmt.Errorf("exactly one of build, image or compose must be set")
	}
	if m.Compose != "" && m.Expose == nil {
		for _, env := range m.Environments {
			if len(env.Hostnames) > 0 {
				return fmt.Errorf("compose apps with hostnames need expose.service and expose.port")
			}
		}
	}

	if len(m.Environments) == 0 {
		return fmt.Errorf("at least one environment is required")
	}
	for name, env := range m.Environments {
		if !namePattern.MatchString(name) {
			return fmt.Errorf("environments.%s: name must be lowercase letters, digits and dashes", name)
		}
		if env.Node == "" {
			return fmt.Errorf("environments.%s.node is required", name)
		}
		if len(env.Hostnames) > 0 && env.Port == 0 && m.Compose == "" {
			return fmt.Errorf("environments.%s.port is required when hostnames are set", name)
		}
		if env.TTL != "" {
			if name != PreviewEnvironment {
				return fmt.Errorf("environments.%s.ttl is only valid for the preview environment", name)
			}
			if _, err := time.ParseDuration(env.TTL); err != nil {
				return fmt.Errorf("environments.%s.ttl: %w", name, err)
			}
		}
		for _, host := range env.Hostnames {
			if name != PreviewEnvironment && strings.Contains(host, "{branch}") {
				return fmt.Errorf("environments.%s: {branch} is only valid in the preview environment", name)
			}
		}
	}
	return nil
}

// EnvironmentNames returns the regular (non-preview) environments, sorted.
func (m *Manifest) EnvironmentNames() []string {
	names := []string{}
	for name := range m.Environments {
		if name != PreviewEnvironment {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Resolve turns one environment into a Spec. For previews, branch selects
// the preview instance and fills the {branch} placeholder in hostnames.
// image is the full image reference to run (ignored for compose apps).
func (m *Manifest) Resolve(envName, branch, image string) (*Spec, error) {
	env, ok := m.Environments[envName]
	if !ok {
		return nil, fmt.Errorf("environment %q is not defined in %s", envName, FileName)
	}

	spec := &Spec{
		App:         m.App,
		Environment: envName,
		Node:        env.Node,
		Expose:      m.Expose,
		Hostnames:   env.Hostnames,
		Port:        env.Port,
		Env:         env.Env,
		Secrets:     env.Secrets,
		Volumes:     env.Volumes,
		Command:     env.Command,
		Healthcheck: env.Healthcheck,
	}

	if m.Compose != "" {
		data, err := os.ReadFile(m.Compose)
		if err != nil {
			return nil, fmt.Errorf("reading compose file: %w", err)
		}
		spec.Compose = string(data)
		if m.Expose != nil {
			spec.Port = m.Expose.Port
		}
	} else {
		spec.Image = image
	}

	if envName == PreviewEnvironment {
		slug := Slugify(branch)
		if slug == "" {
			return nil, fmt.Errorf("a branch name is required for preview deployments")
		}
		spec.Environment = "preview-" + slug
		spec.Preview = true
		spec.Branch = branch
		spec.TTL = env.TTL
		spec.Hostnames = nil
		for _, host := range env.Hostnames {
			spec.Hostnames = append(spec.Hostnames, strings.ReplaceAll(host, "{branch}", slug))
		}
	}

	return spec, nil
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify makes a branch name safe for container names and DNS labels.
func Slugify(s string) string {
	slug := strings.Trim(slugPattern.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	return slug
}

// Validate checks a Spec received over the API.
func (s *Spec) Validate() error {
	if !namePattern.MatchString(s.App) {
		return fmt.Errorf("invalid app name %q", s.App)
	}
	if !namePattern.MatchString(s.Environment) {
		return fmt.Errorf("invalid environment name %q", s.Environment)
	}
	if s.Image == "" && s.Compose == "" {
		return fmt.Errorf("spec needs an image or a compose file")
	}
	if len(s.Hostnames) > 0 && s.Port == 0 {
		return fmt.Errorf("a port is required when hostnames are set")
	}
	if s.TTL != "" {
		if _, err := time.ParseDuration(s.TTL); err != nil {
			return fmt.Errorf("invalid ttl: %w", err)
		}
	}
	return nil
}

// Template is the starting docktrine.yaml written by `docktrine init`.
func Template(app string, compose string) string {
	source := `build:
  dockerfile: Dockerfile
  context: .
  platform: linux/amd64`
	port := "    port: 8080\n"
	if compose != "" {
		source = fmt.Sprintf(`compose: %s
expose:
  service: web     # the service your hostnames route to
  port: 8080`, compose)
		port = ""
	}

	return fmt.Sprintf(`app: %s
%s

environments:
  dev:
    node: onsite
    hostnames: [%s.dev.zeptile.software]
%s    env:
      LOG_LEVEL: debug

  prod:
    node: prd1-tor
    hostnames: [%s.zeptile.software]
%s    secrets: []          # set values with: docktrine secrets set prod KEY=value
    # healthcheck: /healthz

  # Opt-in per-branch previews: docktrine deploy preview
  # preview:
  #   node: onsite
  #   hostnames: ["{branch}.%s.dev.zeptile.software"]
%s  #   ttl: 72h
`, app, source, app, port, app, port, app, strings.ReplaceAll(port, "    ", "  #   "))
}
