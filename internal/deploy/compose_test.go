package deploy

import (
	"strings"
	"testing"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/manifest"
	"gopkg.in/yaml.v3"
)

func TestComposeOverride(t *testing.T) {
	spec := &manifest.Spec{
		App: "site", Environment: "dev", Port: 3000,
		Hostnames: []string{"site.dev.example.com", "www.site.dev.example.com"},
		Compose:   "services:\n  web:\n    image: web\n  db:\n    image: postgres\n",
	}
	tg := target{server: &database.Server{}, spec: spec, deploymentID: 7, env: map[string]string{"TOKEN": "x"}}

	services, err := composeServices(spec.Compose)
	if err != nil {
		t.Fatal(err)
	}
	data, err := tg.composeOverride(services, "web")
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Services map[string]struct {
			Labels      map[string]string `yaml:"labels"`
			Networks    []string          `yaml:"networks"`
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
		Networks map[string]map[string]bool `yaml:"networks"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}

	web, db := doc.Services["web"], doc.Services["db"]
	if web.Labels[LabelEnv] != "dev" || db.Labels[LabelApp] != "site" {
		t.Errorf("missing docktrine labels: %s", data)
	}
	if _, ok := web.Labels[LabelDeployment]; ok {
		t.Errorf("deployment label would force compose to recreate every service: %s", data)
	}
	rule := web.Labels["traefik.http.routers.site-dev.rule"]
	if rule != "Host(`site.dev.example.com`) || Host(`www.site.dev.example.com`)" {
		t.Errorf("router rule = %q", rule)
	}
	if web.Labels["traefik.http.services.site-dev.loadbalancer.server.port"] != "3000" {
		t.Errorf("missing service port label")
	}
	if _, ok := db.Labels["traefik.enable"]; ok {
		t.Error("only the exposed service should be routed")
	}
	if strings.Join(web.Networks, ",") != "default,"+Network || len(db.Networks) != 0 {
		t.Errorf("networks web=%v db=%v", web.Networks, db.Networks)
	}
	if web.Environment["TOKEN"] != "x" || db.Environment != nil {
		t.Errorf("env should only be injected into the exposed service")
	}
	if !doc.Networks[Network]["external"] {
		t.Errorf("docktrine network should be external")
	}
}

func TestDotenvQuoting(t *testing.T) {
	got := string(dotenv(map[string]string{"B": "it's $HOME", "A": "plain"}))
	want := "A='plain'\nB='it'\\''s $HOME'\n"
	if got != want {
		t.Errorf("dotenv = %q, want %q", got, want)
	}
}

func TestRegistryAuth(t *testing.T) {
	d := &Deployer{cfg: Config{Registry: "registry.example.com/team", RegistryUser: "u", RegistryPassword: "p"}}
	if d.registryAuth("registry.example.com/team/app:1") == "" {
		t.Error("expected credentials for the central registry")
	}
	if d.registryAuth("docker.io/library/nginx") != "" {
		t.Error("credentials must not be sent to other registries")
	}
}
