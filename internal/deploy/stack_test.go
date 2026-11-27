package deploy

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zeptile/docktrine/internal/manifest"
)

func TestParsePort(t *testing.T) {
	cases := []struct {
		in        any
		target    int
		published string
	}{
		{"80", 80, ""},
		{"9000:80", 80, "9000"},
		{"127.0.0.1:9000:80/tcp", 80, "9000"},
		{"$SENTRY_BIND:80/tcp", 80, "$SENTRY_BIND"},
		{"${BIND:-127.0.0.1:9000}:80", 80, "${BIND:-127.0.0.1:9000}"},
		{"3000-3005", 3000, ""},
		{8080, 8080, ""},
		{map[string]any{"target": 5432, "published": 15432}, 5432, "15432"},
	}
	for _, c := range cases {
		got := parsePort(c.in)
		if got.Target != c.target || got.Published != c.published {
			t.Errorf("parsePort(%v) = %+v, want target %d published %q", c.in, got, c.target, c.published)
		}
	}
}

const sampleStack = `
x-defaults: &defaults
  image: "$APP_IMAGE"
  restart: unless-stopped
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD:?set a database password}
  web:
    <<: *defaults
    ports: ["${WEB_PORT:-8000}:8000"]
    environment:
      LOG_LEVEL: ${LOG_LEVEL:-info}
      PRICE: "$$5"
      # IGNORED: ${COMMENTED_OUT}
  nginx:
    image: nginx
    ports: ["$BIND:80/tcp"]
    volumes: ["./nginx.conf:/etc/nginx/nginx.conf:ro"]
`

func TestStackVariablesAndSuggestion(t *testing.T) {
	services, order, err := parseComposeServices(sampleStack)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(order); got != 3 || order[0] != "db" || order[2] != "nginx" {
		t.Fatalf("services not in file order: %v", order)
	}
	described := []StackService{}
	for _, name := range order {
		described = append(described, describeService(name, services[name]))
	}
	if service, port := suggestExposed(described); service != "nginx" || port != 80 {
		t.Errorf("suggested %s:%d, want nginx:80", service, port)
	}

	dotEnv := []envLine{{key: "BIND", value: "9000", comment: "Where nginx listens"}}
	example := []envLine{{key: "SMTP_HOST", value: "smtp.example.com"}, {key: "LOG_LEVEL", value: "debug"}}
	vars := stackVariables(sampleStack, dotEnv, example)

	byName := map[string]StackVariable{}
	for _, v := range vars {
		byName[v.Name] = v
	}
	if _, ok := byName["COMMENTED_OUT"]; ok {
		t.Error("a variable in a comment was picked up")
	}
	if _, ok := byName["5"]; ok {
		t.Error("$$ was read as a variable")
	}
	if v := byName["DB_PASSWORD"]; !v.Required || !v.Secret || v.Description != "set a database password" {
		t.Errorf("DB_PASSWORD = %+v", v)
	}
	if v := byName["LOG_LEVEL"]; v.Default != "info" || !v.HasDefault || v.Suggested != "" {
		t.Errorf("LOG_LEVEL = %+v, the compose default should win over the example", v)
	}
	if v := byName["BIND"]; v.Default != "9000" || v.Description != "Where nginx listens" {
		t.Errorf("BIND = %+v", v)
	}
	if v := byName["SMTP_HOST"]; v.Suggested != "smtp.example.com" || v.HasDefault {
		t.Errorf("SMTP_HOST = %+v", v)
	}
	if v := byName["APP_IMAGE"]; v.HasDefault || v.Required {
		t.Errorf("APP_IMAGE = %+v", v)
	}
}

func TestReadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte(`# The public URL
# of the app
URL="https://example.com"
export MODE=prod # trailing comment
# OLD=1
SINGLE='a b'

# Unrelated note
EMPTY=
not a line
`), 0600)

	lines := readEnvFile(path)
	want := []envLine{
		{"URL", "https://example.com", "The public URL of the app"},
		{"MODE", "prod", ""},
		{"SINGLE", "a b", ""},
		{"EMPTY", "", "Unrelated note"},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %+v", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, lines[i], want[i])
		}
	}
}

func TestGitSourceValidate(t *testing.T) {
	good := []manifest.GitSource{
		{Repo: "https://github.com/getsentry/self-hosted"},
		{Repo: "https://github.com/getsentry/self-hosted.git", Ref: "25.11.0", Path: "deploy/compose.yml"},
	}
	for _, g := range good {
		if err := g.Validate(); err != nil {
			t.Errorf("%+v: %v", g, err)
		}
	}
	bad := []manifest.GitSource{
		{Repo: "git@github.com:a/b.git"},
		{Repo: "file:///etc"},
		{Repo: "https://github.com/a/b", Ref: "--upload-pack=x"},
		{Repo: "https://github.com/a/b", Ref: "a..b"},
		{Repo: "https://github.com/a/b", Path: "../outside.yml"},
		{Repo: "https://github.com/a/b", Path: "/etc/passwd"},
	}
	for _, g := range bad {
		if err := g.Validate(); err == nil {
			t.Errorf("%+v should be rejected", g)
		}
	}

	src := manifest.GitSource{Repo: "https://github.com/getsentry/self-hosted.git", Ref: "master", Commit: "0123456789abcdef"}
	if got := src.Label(); got != "git:github.com/getsentry/self-hosted#0123456" {
		t.Errorf("Label() = %q", got)
	}
}

// resolveStack needs only the compose CLI, not a daemon
func TestResolveStackMovesBindsToTheNode(t *testing.T) {
	if exec.Command("docker", "compose", "version").Run() != nil {
		t.Skip("docker compose is not installed")
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "deploy"), 0700)
	os.WriteFile(filepath.Join(root, "deploy", "compose.yml"), []byte(`
services:
  web:
    image: nginx
    environment: { PASS: "${PASS}" }
    configs: [app]
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ../shared:/shared
      - /var/run/docker.sock:/var/run/docker.sock
      - data:/data
configs:
  app: { file: ./app.conf }
volumes:
  data: {}
`), 0600)
	os.WriteFile(filepath.Join(root, "deploy", ".env"), []byte("PASS='a$b'\n"), 0600)

	projectDir := filepath.Join(root, "deploy")
	args := []string{"compose", "--project-name", "x-prod", "--project-directory", projectDir,
		"-f", filepath.Join(projectDir, "compose.yml")}
	out, err := resolveStack(context.Background(), os.Environ(), args, root, "/srv/stacks/x-prod")
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
			Volumes     []struct {
				Type, Source string
			} `json:"volumes"`
		} `json:"services"`
		Configs map[string]struct {
			File string `json:"file"`
		} `json:"configs"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	sources := []string{}
	for _, v := range doc.Services["web"].Volumes {
		sources = append(sources, v.Source)
	}
	want := []string{"/srv/stacks/x-prod/deploy/nginx.conf", "/srv/stacks/x-prod/shared", "/var/run/docker.sock", "data"}
	for i := range want {
		if i >= len(sources) || sources[i] != want[i] {
			t.Fatalf("volume sources = %v, want %v", sources, want)
		}
	}
	if got := doc.Configs["app"].File; got != "/srv/stacks/x-prod/deploy/app.conf" {
		t.Errorf("config file = %q", got)
	}
	// Escaped, so the second pass through compose leaves the $ alone
	if got := doc.Services["web"].Environment["PASS"]; got != "a$$b" {
		t.Errorf("PASS = %q", got)
	}
}

func TestWriteTarSkipsGitAndEnv(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".git", "objects"), 0700)
	os.MkdirAll(filepath.Join(root, "conf"), 0700)
	os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("x"), 0600)
	os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0600)
	os.WriteFile(filepath.Join(root, "conf", "app.conf"), []byte("ok"), 0600)
	os.Symlink("app.conf", filepath.Join(root, "conf", "link.conf"))

	tarPath := filepath.Join(t.TempDir(), "out.tar")
	f, _ := os.Create(tarPath)
	if err := writeTar(f, root, func(rel string) bool { return rel == ".env" }); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out, err := exec.Command("tar", "-tvf", tarPath).CombinedOutput()
	if err != nil {
		t.Skip("tar is not available")
	}
	listing := string(out)
	for _, want := range []string{"conf/", "conf/app.conf", "conf/link.conf -> app.conf"} {
		if !strings.Contains(listing, want) {
			t.Errorf("tar is missing %q:\n%s", want, listing)
		}
	}
	for _, unwanted := range []string{".git", ".env"} {
		if strings.Contains(listing, unwanted) {
			t.Errorf("tar has %q:\n%s", unwanted, listing)
		}
	}
}
