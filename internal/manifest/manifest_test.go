package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `app: invoicer
build:
  dockerfile: Dockerfile
environments:
  prod:
    node: prd1-tor
    hostnames: [invoicer.zeptile.software]
    port: 8080
    secrets: [DATABASE_URL]
  preview:
    node: onsite
    hostnames: ["{branch}.invoicer.dev.zeptile.software"]
    port: 8080
    ttl: 72h
`

func TestParseAndResolve(t *testing.T) {
	m, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.EnvironmentNames(); len(got) != 1 || got[0] != "prod" {
		t.Fatalf("EnvironmentNames() = %v, want [prod]", got)
	}

	spec, err := m.Resolve("prod", "", "registry.example.com/invoicer:abc")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Environment != "prod" || spec.Image != "registry.example.com/invoicer:abc" || spec.Preview {
		t.Fatalf("unexpected spec %+v", spec)
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePreview(t *testing.T) {
	m, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}

	spec, err := m.Resolve("preview", "feature/New_Login", "img:1")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Environment != "preview-feature-new-login" {
		t.Errorf("Environment = %q", spec.Environment)
	}
	if spec.Hostnames[0] != "feature-new-login.invoicer.dev.zeptile.software" {
		t.Errorf("Hostnames = %v", spec.Hostnames)
	}
	if !spec.Preview || spec.TTL != "72h" || spec.Branch != "feature/New_Login" {
		t.Errorf("unexpected preview spec %+v", spec)
	}

	if _, err := m.Resolve("preview", "", "img:1"); err == nil {
		t.Error("expected an error for a preview without a branch")
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]string{
		"bad app name":           "app: Invoicer\nimage: x\nenvironments: {prod: {node: a}}",
		"no source":              "app: a\nenvironments: {prod: {node: a}}",
		"two sources":            "app: a\nimage: x\ncompose: c.yml\nenvironments: {prod: {node: a}}",
		"missing node":           "app: a\nimage: x\nenvironments: {prod: {}}",
		"hostname no port":       "app: a\nimage: x\nenvironments: {prod: {node: a, hostnames: [a.b]}}",
		"ttl outside preview":    "app: a\nimage: x\nenvironments: {prod: {node: a, ttl: 1h}}",
		"branch outside preview": "app: a\nimage: x\nenvironments: {prod: {node: a, port: 1, hostnames: ['{branch}.a']}}",
		"unknown field":          "app: a\nimage: x\nenvironments: {prod: {node: a, replicas: 2}}",
		"compose no expose":      "app: a\ncompose: c.yml\nenvironments: {prod: {node: a, hostnames: [a.b]}}",
	}
	for name, input := range cases {
		if _, err := Parse([]byte(input)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestResolveCompose(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yml")
	os.WriteFile(compose, []byte("services:\n  web:\n    image: nginx\n"), 0644)

	m, err := Parse([]byte("app: site\ncompose: " + compose + "\nexpose: {service: web, port: 80}\n" +
		"environments: {dev: {node: onsite, hostnames: [site.dev.example.com]}}"))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := m.Resolve("dev", "", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Image != "" || !strings.Contains(spec.Compose, "nginx") || spec.Port != 80 {
		t.Fatalf("unexpected compose spec %+v", spec)
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateParses(t *testing.T) {
	for _, compose := range []string{"", "docker-compose.yml"} {
		if _, err := Parse([]byte(Template("my-app", compose))); err != nil {
			t.Errorf("Template(%q) does not parse: %v\n%s", compose, err, Template("my-app", compose))
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"main":                  "main",
		"feature/Add-Login":     "feature-add-login",
		"--weird__branch--":     "weird-branch",
		strings.Repeat("a", 60): strings.Repeat("a", 40),
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
