// Package catalog holds presets for the launch form: popular self-hosted
// apps with the source, variables and settings they need, pinned to versions
// that were launched and checked.
//
// The presets live in presets.yaml, embedded in the binary, so adding one is
// an edit to that file: no code, and the CLI or the API can use them as well
// as the web UI. The tests keep every entry well-formed.
package catalog

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/manifest"
	"gopkg.in/yaml.v3"
)

//go:embed presets.yaml
var presetsYAML []byte

// Kinds match the launch form's choices.
const (
	KindImage   = "image"
	KindGit     = "git"
	KindCompose = "compose"
)

// Preset fills the launch form for one app. Everything it sets stays
// editable there.
type Preset struct {
	// Lowercase letters, digits and dashes; also the suggested app name
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Homepage    string `yaml:"homepage" json:"homepage"`
	Kind        string `yaml:"kind" json:"kind"`
	// The version it is pinned to, as people know it, e.g. "3.2.2"
	Version string `yaml:"version" json:"version"`

	// One of these, by kind
	Image   string  `yaml:"image,omitempty" json:"image,omitempty"`
	Source  *Source `yaml:"source,omitempty" json:"source,omitempty"`
	Compose string  `yaml:"compose,omitempty" json:"compose,omitempty"`

	// Stacks: the service and port hostnames route to
	Expose *manifest.Expose `yaml:"expose,omitempty" json:"expose,omitempty"`
	// Images: the container port hostnames route to
	Port int `yaml:"port,omitempty" json:"port,omitempty"`
	// Images: name:/path volumes, kept across deploys
	Volumes     []string `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Healthcheck string   `yaml:"healthcheck,omitempty" json:"healthcheck,omitempty"`

	// Stacks from Git: run on the server before the stack starts
	Setup string `yaml:"setup,omitempty" json:"setup,omitempty"`

	Variables []Variable `yaml:"variables,omitempty" json:"variables,omitempty"`

	// What it needs of the server. The preflight checks use these when the
	// stack doesn't say itself, as Sentry's install script does.
	Requirements *deploy.Requirements `yaml:"requirements,omitempty" json:"requirements,omitempty"`

	// Short things to know before launching, one sentence or two each
	Notes []string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Source is a public Git repository, pinned to a tag or release branch.
type Source struct {
	Repo string `yaml:"repo" json:"repo"`
	Ref  string `yaml:"ref" json:"ref"`
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
}

// Variable is one the app reads, with what the form should start from.
type Variable struct {
	Name string `yaml:"name" json:"name"`
	// A starting value; empty means leave it to the person, or the default
	Value       string `yaml:"value,omitempty" json:"value,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// The app doesn't work without it
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`
	// Stored encrypted rather than in the deployment's spec
	Secret bool `yaml:"secret,omitempty" json:"secret,omitempty"`
	// A random value would do, in this format; the form can fill it in
	Generate *Generate `yaml:"generate,omitempty" json:"generate,omitempty"`
	// Filled from the first hostname, e.g. "https://{hostname}"
	FromHostname string `yaml:"from_hostname,omitempty" json:"from_hostname,omitempty"`
}

// Generate describes a random value. It matches the web UI's secret
// generator: alnum and hex count characters, base64 counts random bytes.
type Generate struct {
	Kind   string `yaml:"kind" json:"kind"`
	Length int    `yaml:"length" json:"length"`
}

var (
	loadOnce sync.Once
	presets  []Preset
	loadErr  error
)

// All returns the presets in the order the catalog lists them.
func All() ([]Preset, error) {
	loadOnce.Do(func() {
		presets, loadErr = parse(presetsYAML)
	})
	return presets, loadErr
}

// Get returns one preset by its ID.
func Get(id string) (*Preset, bool) {
	all, err := All()
	if err != nil {
		return nil, false
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], true
		}
	}
	return nil, false
}

func parse(data []byte) ([]Preset, error) {
	var doc struct {
		Presets []Preset `yaml:"presets"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// A misspelt field would otherwise be dropped without a word
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("reading the catalog: %w", err)
	}
	seen := map[string]bool{}
	for i := range doc.Presets {
		p := &doc.Presets[i]
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("catalog entry %q: %w", p.ID, err)
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("catalog entry %q is listed twice", p.ID)
		}
		seen[p.ID] = true
		// The preflight checks say where a minimum comes from
		if p.Requirements != nil && p.Requirements.Source == "" {
			p.Requirements.Source = "its catalog entry"
		}
	}
	return doc.Presets, nil
}

var (
	idPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	envPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	// Names that hold credentials, as the launch form and stack inspection
	// recognize them
	secretName = regexp.MustCompile(`(?i)(PASSWORD|PASSWD|PASS$|SECRET|TOKEN|PRIVATE|(^|_)KEY$|API_?KEY|CREDENTIAL|DSN$|SALT)`)
	// A tag or ref that moves with every release
	movingRef = regexp.MustCompile(`^(latest|main|master|stable|nightly|beta|next|develop|HEAD)$`)
)

// Validate checks that a preset can fill the form and launch: the fields its
// kind needs, a pinned version, and credentials stored as secrets.
func (p *Preset) Validate() error {
	if !idPattern.MatchString(p.ID) || len(p.ID) > 40 {
		return fmt.Errorf("id must be lowercase letters, digits and dashes")
	}
	if p.Name == "" || p.Description == "" || p.Version == "" {
		return fmt.Errorf("name, description and version are required")
	}
	if !strings.HasPrefix(p.Homepage, "https://") {
		return fmt.Errorf("homepage must be an https:// address")
	}

	switch p.Kind {
	case KindImage:
		if p.Image == "" || p.Source != nil || p.Compose != "" {
			return fmt.Errorf("an image preset sets image, and not source or compose")
		}
		tag := ""
		if i := strings.LastIndex(p.Image, ":"); i > strings.LastIndex(p.Image, "/") {
			tag = p.Image[i+1:]
		}
		if tag == "" || movingRef.MatchString(tag) {
			return fmt.Errorf("image %s must be pinned to a version tag", p.Image)
		}
		if p.Port < 1 || p.Port > 65535 {
			return fmt.Errorf("an image preset needs the port it listens on")
		}
		if p.Expose != nil || p.Setup != "" {
			return fmt.Errorf("expose and setup are for stacks")
		}
		for _, v := range p.Volumes {
			name, path, ok := strings.Cut(v, ":")
			if !ok || name == "" || !strings.HasPrefix(path, "/") {
				return fmt.Errorf("volume %q must be name:/path/in/container", v)
			}
		}
	case KindGit, KindCompose:
		if p.Kind == KindGit {
			if p.Source == nil || p.Image != "" || p.Compose != "" {
				return fmt.Errorf("a git preset sets source, and not image or compose")
			}
			src := manifest.GitSource{Repo: p.Source.Repo, Ref: p.Source.Ref, Path: p.Source.Path}
			if err := src.Validate(); err != nil {
				return err
			}
			if p.Source.Ref == "" || movingRef.MatchString(p.Source.Ref) {
				return fmt.Errorf("source must be pinned to a tag or release ref")
			}
		} else {
			if p.Compose == "" || p.Image != "" || p.Source != nil {
				return fmt.Errorf("a compose preset sets compose, and not image or source")
			}
			if p.Setup != "" {
				return fmt.Errorf("setup commands are for stacks from Git")
			}
		}
		if p.Expose == nil || p.Expose.Service == "" || p.Expose.Port < 1 || p.Expose.Port > 65535 {
			return fmt.Errorf("a stack preset needs expose, the service and port hostnames route to")
		}
		if p.Port != 0 || len(p.Volumes) > 0 || p.Healthcheck != "" {
			return fmt.Errorf("port, volumes and healthcheck are for images")
		}
	default:
		return fmt.Errorf("kind must be image, git or compose")
	}

	names := map[string]bool{}
	for _, v := range p.Variables {
		if !envPattern.MatchString(v.Name) {
			return fmt.Errorf("variable %q isn't a valid name", v.Name)
		}
		if names[v.Name] {
			return fmt.Errorf("variable %s is listed twice", v.Name)
		}
		names[v.Name] = true
		if secretName.MatchString(v.Name) && !v.Secret {
			return fmt.Errorf("variable %s holds a credential, so it must be secret", v.Name)
		}
		if v.Secret && v.Value != "" {
			return fmt.Errorf("variable %s is secret, so it can't have a value in the catalog; generate it instead", v.Name)
		}
		if g := v.Generate; g != nil {
			if !v.Secret {
				return fmt.Errorf("variable %s is generated, so it must be secret", v.Name)
			}
			switch {
			case g.Kind != "alnum" && g.Kind != "hex" && g.Kind != "base64":
				return fmt.Errorf("variable %s: generate.kind must be alnum, hex or base64", v.Name)
			case g.Length < 16 || g.Length > 128:
				return fmt.Errorf("variable %s: generate.length must be 16 to 128", v.Name)
			}
		}
		if v.FromHostname != "" && !strings.Contains(v.FromHostname, "{hostname}") {
			return fmt.Errorf("variable %s: from_hostname must contain {hostname}", v.Name)
		}
		if v.Required && v.Secret && v.Generate == nil && v.Description == "" {
			return fmt.Errorf("variable %s is a required secret; say what it is or how to generate it", v.Name)
		}
	}
	if r := p.Requirements; r != nil {
		if len(r.Variants) == 0 || r.Variants[len(r.Variants)-1].When != nil {
			return fmt.Errorf("requirements need a last variant without a condition")
		}
		for _, v := range r.Variants {
			if v.MemoryMB <= 0 {
				return fmt.Errorf("every requirements variant needs memory_mb")
			}
		}
	}
	if len(p.Notes) == 0 {
		return fmt.Errorf("say at least one thing to know in notes")
	}
	return nil
}
