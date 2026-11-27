package catalog

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/manifest"
)

func TestCatalogLoads(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"uptime-kuma", "n8n", "gitea", "umami", "plausible", "sentry"}
	var got []string
	for _, p := range all {
		got = append(got, p.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("presets = %v, want %v", got, want)
	}
	if p, ok := Get("sentry"); !ok || p.Requirements.Source != "install/_min-requirements.sh" {
		t.Errorf("Get(sentry) = %+v, %v", p, ok)
	}
	if p, _ := Get("plausible"); p.Requirements.Source != "its catalog entry" {
		t.Errorf("plausible's requirements source = %q, want the catalog's", p.Requirements.Source)
	}
}

// Compose presets are read offline, the way the launch form reads them: the
// exposed service must be there with its port, and every variable compose
// refuses to start without must be one the preset asks for.
func TestComposePresetsMatchTheirStack(t *testing.T) {
	all, _ := All()
	for _, p := range all {
		if p.Kind != KindCompose {
			continue
		}
		info, err := deploy.InspectStack(context.Background(), nil, p.Compose)
		if err != nil {
			t.Errorf("%s: %v", p.ID, err)
			continue
		}
		checkStack(t, &p, info)
	}
}

func checkStack(t *testing.T, p *Preset, info *deploy.StackInfo) {
	t.Helper()
	var exposed *deploy.StackService
	for i := range info.Services {
		if info.Services[i].Name == p.Expose.Service {
			exposed = &info.Services[i]
		}
	}
	if exposed == nil {
		t.Errorf("%s: no service %q in the stack", p.ID, p.Expose.Service)
		return
	}
	// A service with no ports or expose in the file still listens; only
	// check the port when the file says which
	if len(exposed.Ports) > 0 {
		found := false
		for _, port := range exposed.Ports {
			found = found || port.Target == p.Expose.Port
		}
		if !found {
			t.Errorf("%s: %s doesn't list port %d", p.ID, p.Expose.Service, p.Expose.Port)
		}
	}
	declared := map[string]Variable{}
	for _, v := range p.Variables {
		declared[v.Name] = v
	}
	for _, v := range info.Variables {
		d, ok := declared[v.Name]
		if v.Required && (!ok || (!d.Required && d.Value == "")) {
			t.Errorf("%s: the stack requires %s, which the preset doesn't ask for", p.ID, v.Name)
		}
		if v.Secret && ok && !d.Secret {
			t.Errorf("%s: %s looks like a credential but isn't secret", p.ID, v.Name)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	base := func() Preset {
		return Preset{
			ID: "app", Name: "App", Description: "Does things.", Homepage: "https://example.com",
			Kind: KindImage, Version: "1.0", Image: "example/app:1.0", Port: 8080,
			Notes: []string{"Something to know."},
		}
	}
	if p := base(); p.Validate() != nil {
		t.Fatalf("the base preset should be valid: %v", p.Validate())
	}
	cases := map[string]func(p *Preset){
		"bad id":              func(p *Preset) { p.ID = "My App" },
		"no notes":            func(p *Preset) { p.Notes = nil },
		"unpinned image":      func(p *Preset) { p.Image = "example/app" },
		"latest image":        func(p *Preset) { p.Image = "example/app:latest" },
		"image without port":  func(p *Preset) { p.Port = 0 },
		"bad volume":          func(p *Preset) { p.Volumes = []string{"data"} },
		"plain credential":    func(p *Preset) { p.Variables = []Variable{{Name: "DB_PASSWORD"}} },
		"secret with a value": func(p *Preset) { p.Variables = []Variable{{Name: "API_KEY", Secret: true, Value: "x"}} },
		"generated in plain": func(p *Preset) {
			p.Variables = []Variable{{Name: "SEED", Generate: &Generate{Kind: "hex", Length: 32}}}
		},
		"bad generate kind": func(p *Preset) {
			p.Variables = []Variable{{Name: "SEED", Secret: true, Generate: &Generate{Kind: "uuid", Length: 32}}}
		},
		"short generate": func(p *Preset) {
			p.Variables = []Variable{{Name: "SEED", Secret: true, Generate: &Generate{Kind: "hex", Length: 4}}}
		},
		"bad variable name":  func(p *Preset) { p.Variables = []Variable{{Name: "1X"}} },
		"twice":              func(p *Preset) { p.Variables = []Variable{{Name: "X"}, {Name: "X"}} },
		"hostname template":  func(p *Preset) { p.Variables = []Variable{{Name: "URL", FromHostname: "https://example.com"}} },
		"unexplained secret": func(p *Preset) { p.Variables = []Variable{{Name: "TOKEN", Secret: true, Required: true}} },
		"conditional minimum": func(p *Preset) {
			p.Requirements = &deploy.Requirements{Variants: []deploy.RequirementVariant{{When: &deploy.Condition{Variable: "A", Value: "b"}, MemoryMB: 1}}}
		},
		"git on a branch": func(p *Preset) {
			p.Kind, p.Image, p.Port = KindGit, "", 0
			p.Source = &Source{Repo: "https://github.com/a/b", Ref: "main"}
			p.Expose = &manifest.Expose{Service: "web", Port: 80}
		},
		"stack without expose": func(p *Preset) {
			p.Kind, p.Image, p.Port = KindCompose, "", 0
			p.Compose = "services: {web: {image: nginx}}"
		},
		"compose with setup": func(p *Preset) {
			p.Kind, p.Image, p.Port = KindCompose, "", 0
			p.Compose = "services: {web: {image: nginx}}"
			p.Expose = &manifest.Expose{Service: "web", Port: 80}
			p.Setup = "./install.sh"
		},
	}
	for name, change := range cases {
		p := base()
		change(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := parse([]byte("presets:\n  - id: x\n    nmae: X\n"))
	if err == nil || !strings.Contains(err.Error(), "nmae") {
		t.Errorf("err = %v, want it to name the misspelt field", err)
	}
}

// TestPresetsUpstream checks every preset against the real repository or
// registry: images exist at their tag with the port the preset routes to,
// and stacks from Git have the services and variables it expects. It needs
// the network, so it only runs when asked:
//
//	DOCKTRINE_CATALOG_LIVE=1 go test ./internal/catalog/
func TestPresetsUpstream(t *testing.T) {
	if os.Getenv("DOCKTRINE_CATALOG_LIVE") == "" {
		t.Skip("set DOCKTRINE_CATALOG_LIVE=1 to check presets against their upstream")
	}
	all, _ := All()
	d := deploy.New(nil, nil, deploy.Config{})
	for _, p := range all {
		p := p
		t.Run(p.ID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			switch p.Kind {
			case KindImage:
				info, err := d.InspectImage(ctx, p.Image)
				if err != nil {
					t.Fatal(err)
				}
				listens := len(info.Ports) == 0
				for _, port := range info.Ports {
					listens = listens || port == p.Port
				}
				if !listens {
					t.Errorf("%s listens on %v, not %d", p.Image, info.Ports, p.Port)
				}
				// Data the image declares should be in a volume
				for _, path := range info.Volumes {
					kept := false
					for _, v := range p.Volumes {
						_, target, _ := strings.Cut(v, ":")
						kept = kept || target == path
					}
					if !kept {
						t.Errorf("%s keeps data in %s, which no volume holds", p.Image, path)
					}
				}
			case KindCompose:
				// Every image the file pins must exist at its tag
				info, err := deploy.InspectStack(ctx, nil, p.Compose)
				if err != nil {
					t.Fatal(err)
				}
				for _, s := range info.Services {
					if _, err := d.InspectImage(ctx, s.Image); err != nil {
						t.Errorf("%s: %v", s.Name, err)
					}
				}
			case KindGit:
				src := &manifest.GitSource{Repo: p.Source.Repo, Ref: p.Source.Ref, Path: p.Source.Path}
				info, err := deploy.InspectStack(ctx, src, "")
				if err != nil {
					t.Fatal(err)
				}
				checkStack(t, &p, info)
				// The catalog's minimums repeat the install script's, to
				// show them before the stack is read
				if info.Requirements != nil && len(info.Requirements.Variants) > 0 &&
					!reflect.DeepEqual(info.Requirements.Variants, p.Requirements.Variants) {
					t.Errorf("the stack needs %+v, the catalog says %+v", info.Requirements.Variants, p.Requirements.Variants)
				}
			}
		})
	}
}
