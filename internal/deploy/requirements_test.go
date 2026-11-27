package deploy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sentryMinRequirements = `# Don't forget to update the README and other docs when you change these!
MIN_DOCKER_VERSION='19.03.6'
MIN_COMPOSE_VERSION='2.32.2'

MIN_PODMAN_VERSION='4.9.3'
MIN_PODMAN_COMPOSE_VERSION='1.3.0'

MIN_BASH_VERSION='4.4.0'

# 16 GB minimum host RAM, but there'll be some overhead outside of what
# can be allotted to docker
if [[ "$COMPOSE_PROFILES" == "errors-only" ]]; then
  MIN_RAM_HARD=7000 # MB
  MIN_CPU_HARD=2
else
  MIN_RAM_HARD=14000 # MB
  MIN_CPU_HARD=4
fi
`

func TestParseRequirementsSentry(t *testing.T) {
	req, ok := parseRequirements(sentryMinRequirements)
	if !ok {
		t.Fatal("expected requirements to be found")
	}
	if req.DockerVersion != "19.03.6" {
		t.Errorf("DockerVersion = %q, want 19.03.6", req.DockerVersion)
	}
	cases := []struct {
		name string
		vars map[string]string
		mem  int
		cpus float64
	}{
		{"errors-only", map[string]string{"COMPOSE_PROFILES": "errors-only"}, 7000, 2},
		{"nil vars", nil, 14000, 4},
		{"other value", map[string]string{"COMPOSE_PROFILES": "feature-complete"}, 14000, 4},
		{"empty value", map[string]string{"COMPOSE_PROFILES": ""}, 14000, 4},
	}
	for _, c := range cases {
		got := req.Applicable(c.vars)
		if got.MemoryMB != c.mem || got.CPUs != c.cpus {
			t.Errorf("%s: Applicable = %d MB / %v CPUs, want %d / %v", c.name, got.MemoryMB, got.CPUs, c.mem, c.cpus)
		}
	}
}

func TestParseRequirements(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		ok     bool
		docker string
		// Expected Applicable results for the given variables
		checks []struct {
			vars map[string]string
			mem  int
			cpus float64
		}
	}{
		{
			name: "plain top-level assignments",
			text: "MIN_RAM=4096\nMIN_CPU=2\nMIN_DOCKER_VERSION=\"20.10.0\"\n",
			ok:   true, docker: "20.10.0",
			checks: []struct {
				vars map[string]string
				mem  int
				cpus float64
			}{
				{nil, 4096, 2},
				{map[string]string{"X": "y"}, 4096, 2},
			},
		},
		{
			name: "exported and hard variants with fractional cpus",
			text: "export MIN_MEM_HARD='2048'\nreadonly MIN_CPUS=1.5\n",
			ok:   true,
			checks: []struct {
				vars map[string]string
				mem  int
				cpus float64
			}{{nil, 2048, 1.5}},
		},
		{
			name: "if with !=",
			text: "if [[ \"$MODE\" != \"lite\" ]]; then\n  MIN_RAM=8000\n  MIN_CPU=4\nfi\n",
			ok:   true,
			checks: []struct {
				vars map[string]string
				mem  int
				cpus float64
			}{
				{map[string]string{"MODE": "full"}, 8000, 4},
				{nil, 8000, 4},
				// No else and no fallback: the condition fails, nothing applies
				{map[string]string{"MODE": "lite"}, 0, 0},
			},
		},
		{
			name: "unparseable if is ignored",
			text: "if command -v foo; then\n  MIN_RAM=9999\n  MIN_CPU=9\nfi\n",
			ok:   false,
		},
		{
			name: "unparseable if ignored but top-level kept",
			text: "MIN_RAM=1000\nif command -v foo; then\n  MIN_RAM=9999\nfi\n",
			ok:   true,
			checks: []struct {
				vars map[string]string
				mem  int
				cpus float64
			}{{nil, 1000, 0}},
		},
		{
			name: "nested ifs are ignored",
			text: "if [[ \"$A\" == \"1\" ]]; then\n  MIN_RAM=2000\n  if [[ \"$B\" == \"2\" ]]; then\n    MIN_RAM=9000\n    MIN_CPU=8\n  fi\nfi\n",
			ok:   true,
			checks: []struct {
				vars map[string]string
				mem  int
				cpus float64
			}{{map[string]string{"A": "1"}, 2000, 0}},
		},
		{
			name: "elif makes the block ignored",
			text: "if [[ \"$A\" == \"1\" ]]; then\n  MIN_RAM=2000\nelif [[ \"$A\" == \"2\" ]]; then\n  MIN_RAM=3000\nelse\n  MIN_RAM=4000\nfi\n",
			ok:   false,
		},
		{
			name: "only docker version",
			text: "MIN_DOCKER_VERSION=24.0.0\n",
			ok:   true, docker: "24.0.0",
		},
		{
			name: "branch inherits cpus from top-level",
			text: "MIN_CPU=4\nif [[ \"$P\" == \"small\" ]]; then\n  MIN_RAM=7000\nfi\n",
			ok:   true,
			checks: []struct {
				vars map[string]string
				mem  int
				cpus float64
			}{
				{map[string]string{"P": "small"}, 7000, 4},
				{nil, 0, 4},
			},
		},
		{
			name: "no minimums",
			text: "#!/bin/bash\nset -e\necho hello\nMIN_BASH_VERSION='4.4.0'\n",
			ok:   false,
		},
		{name: "empty", text: "", ok: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, ok := parseRequirements(c.text)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, c.ok, req)
			}
			if req.DockerVersion != c.docker {
				t.Errorf("DockerVersion = %q, want %q", req.DockerVersion, c.docker)
			}
			for _, ch := range c.checks {
				got := req.Applicable(ch.vars)
				if got.MemoryMB != ch.mem || got.CPUs != ch.cpus {
					t.Errorf("Applicable(%v) = %d MB / %v CPUs, want %d / %v", ch.vars, got.MemoryMB, got.CPUs, ch.mem, ch.cpus)
				}
			}
		})
	}
}

func TestParseRequirementsElseBecomesFallback(t *testing.T) {
	req, _ := parseRequirements(sentryMinRequirements)
	if len(req.Variants) != 2 {
		t.Fatalf("got %d variants, want 2", len(req.Variants))
	}
	if w := req.Variants[0].When; w == nil || w.Variable != "COMPOSE_PROFILES" || w.Value != "errors-only" || w.Not {
		t.Errorf("first variant condition = %+v", w)
	}
	if req.Variants[1].When != nil {
		t.Errorf("else variant should be the fallback, got condition %+v", req.Variants[1].When)
	}
}

func TestScriptRequirements(t *testing.T) {
	t.Run("sentry layout", func(t *testing.T) {
		dir := t.TempDir()
		write := func(rel, content string) {
			p := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("install.sh", "#!/usr/bin/env bash\nset -eE\nsource install/_min-requirements.sh\n")
		write("install/_min-requirements.sh", sentryMinRequirements)
		write("install/other.sh", "echo nothing\n")

		req, ok := scriptRequirements(dir)
		if !ok {
			t.Fatal("expected requirements")
		}
		if req.Source != "install/_min-requirements.sh" {
			t.Errorf("Source = %q", req.Source)
		}
		if req.DockerVersion != "19.03.6" || len(req.Variants) != 2 {
			t.Errorf("unexpected requirements %+v", req)
		}
	})
	t.Run("minimums in install.sh itself", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte("MIN_RAM=2048\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		req, ok := scriptRequirements(dir)
		if !ok || req.Source != "install.sh" || req.Applicable(nil).MemoryMB != 2048 {
			t.Errorf("got %+v, %v", req, ok)
		}
	})
	t.Run("no install.sh", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "install"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "install", "_min-requirements.sh"), []byte(sentryMinRequirements), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := scriptRequirements(dir); ok {
			t.Error("expected ok=false without install.sh")
		}
	})
	t.Run("install.sh without minimums", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte("echo hi\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok := scriptRequirements(dir); ok {
			t.Error("expected ok=false")
		}
	})
}

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{"512m", 512 << 20, true},
		{"512M", 512 << 20, true},
		{"1g", 1 << 30, true},
		{"1gb", 1 << 30, true},
		{"1.5g", 3 << 29, true},
		{"1.5gb", 3 << 29, true},
		{"2GiB", 2 << 30, true},
		{"64k", 64 << 10, true},
		{"1t", 1 << 40, true},
		{"1024", 1024, true},
		{"100b", 100, true},
		{" 256m ", 256 << 20, true},
		{1048576, 1048576, true},
		{float64(2048), 2048, true},
		{"lots", 0, false},
		{"", 0, false},
		{"-1g", 0, false},
		{nil, 0, false},
		{true, 0, false},
	}
	for _, c := range cases {
		got, ok := parseBytes(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parseBytes(%#v) = %d, %v; want %d, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestParseNumber(t *testing.T) {
	cases := []struct {
		in   any
		want float64
		ok   bool
	}{
		{2, 2, true},
		{0.5, 0.5, true},
		{"1.25", 1.25, true},
		{" 3 ", 3, true},
		{"abc", 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		got, ok := parseNumber(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parseNumber(%#v) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestComposeReservations(t *testing.T) {
	cases := []struct {
		name    string
		compose string
		mem     int
		cpus    float64
	}{
		{
			name: "deploy reservations summed over services",
			compose: `
services:
  web:
    image: a
    deploy:
      resources:
        reservations:
          memory: 512m
          cpus: "0.5"
  worker:
    image: b
    deploy:
      resources:
        reservations:
          memory: 1g
          cpus: 1.25
`,
			mem: 1536, cpus: 1.75,
		},
		{
			name: "mem_reservation fallback",
			compose: `
services:
  db:
    image: postgres
    mem_reservation: 256m
  cache:
    image: redis
    mem_reservation: 1gb
`,
			mem: 1280, cpus: 0,
		},
		{
			name: "deploy memory wins over mem_reservation",
			compose: `
services:
  web:
    image: a
    mem_reservation: 4g
    deploy:
      resources:
        reservations:
          memory: 512m
`,
			mem: 512, cpus: 0,
		},
		{
			name: "bare integer bytes and integer cpus",
			compose: `
services:
  web:
    image: a
    deploy:
      resources:
        reservations:
          memory: 1073741824
          cpus: 2
`,
			mem: 1024, cpus: 2,
		},
		{
			name: "limits only reserve nothing",
			compose: `
services:
  web:
    image: a
    deploy:
      resources:
        limits:
          memory: 4g
          cpus: "2"
  plain:
    image: b
`,
			mem: 0, cpus: 0,
		},
		{
			name: "fractional gigabytes and unparseable values ignored",
			compose: `
services:
  a:
    image: a
    deploy:
      resources:
        reservations:
          memory: 1.5g
  b:
    image: b
    deploy:
      resources:
        reservations:
          memory: "${MEM:-1g}"
          cpus: "${CPUS}"
`,
			mem: 1536, cpus: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			services, _, err := parseComposeServices(c.compose)
			if err != nil {
				t.Fatal(err)
			}
			mem, cpus := composeReservations(services)
			if mem != c.mem || cpus != c.cpus {
				t.Errorf("got %d MB / %v CPUs, want %d / %v", mem, cpus, c.mem, c.cpus)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"27.3.1", "19.03.6", true},
		{"19.03.5", "19.03.6", false},
		{"19.03.6", "19.03.6", true},
		{"v20.10.0", "19.03.6", true},
		{"24.0.7-ce", "19.03.6", true},
		{"19.03.6-ce", "19.03.6", true},
		{"18.09.9", "19.03.6", false},
		{"19.3", "19.03.6", false},
		{"20", "19.03.6", true},
		{"19.03.10", "19.03.6", true},
		{"", "19.03.6", false},
	}
	for _, c := range cases {
		if got := versionAtLeast(c.have, c.want); got != c.ok {
			t.Errorf("versionAtLeast(%q, %q) = %v, want %v", c.have, c.want, got, c.ok)
		}
	}
}

func TestResolvePublished(t *testing.T) {
	cases := []struct {
		name string
		in   string
		vars map[string]string
		want []int
		ok   bool
	}{
		{"plain", "9000", nil, []int{9000}, true},
		{"var set", "${SENTRY_BIND}", map[string]string{"SENTRY_BIND": "9000"}, []int{9000}, true},
		{"var with default, unset", "${PORT:-8080}", nil, []int{8080}, true},
		{"var with default, set", "${PORT:-8080}", map[string]string{"PORT": "9090"}, []int{9090}, true},
		{"var with default, empty value", "${PORT:-8080}", map[string]string{"PORT": ""}, []int{8080}, true},
		{"bare var unset", "$PORT", nil, nil, false},
		{"bare var set", "$PORT", map[string]string{"PORT": "81"}, []int{81}, true},
		{"braced var unset no default", "${PORT}", nil, nil, false},
		{"range", "8000-8002", nil, []int{8000, 8001, 8002}, true},
		{"host address", "127.0.0.1:9000", nil, []int{9000}, true},
		{"var expands to host and port", "${SENTRY_BIND}", map[string]string{"SENTRY_BIND": "127.0.0.1:9000"}, []int{9000}, true},
		{"empty", "", nil, nil, true},
		{"whitespace", "  ", nil, nil, true},
		{"not a number", "abc", nil, nil, false},
		{"zero", "0", nil, nil, false},
		{"too high", "70000", nil, nil, false},
		{"backwards range", "8002-8000", nil, nil, false},
		{"huge range", "1000-3000", nil, nil, false},
		{"bad range end", "8000-abc", nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := resolvePublished(c.in, c.vars)
			if ok != c.ok || !reflect.DeepEqual(got, c.want) {
				t.Errorf("resolvePublished(%q, %v) = %v, %v; want %v, %v", c.in, c.vars, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestParseNetstat(t *testing.T) {
	out := `Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN
tcp        0      0 127.0.0.53:53           0.0.0.0:*               LISTEN
tcp        0      0 0.0.0.0:9000            0.0.0.0:*               LISTEN
tcp        0      0 :::80                   :::*                    LISTEN
tcp        0      0 :::22                   :::*                    LISTEN
tcp6       0      0 ::1:5432                :::*                    LISTEN
tcp6       0      0 :::8080                 :::*                    LISTEN
udp        0      0 0.0.0.0:68              0.0.0.0:*
tcp        0      0 10.0.0.1:44321          10.0.0.2:443            ESTABLISHED
`
	got := parseNetstat(out)
	want := []int{22, 53, 80, 5432, 8080, 9000, 44321}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNetstat = %v, want %v", got, want)
	}
	if got := parseNetstat(""); len(got) != 0 {
		t.Errorf("parseNetstat(\"\") = %v, want none", got)
	}
}

func collectChecks(req PreflightRequest, cpus, memMB int, version string) map[string]Check {
	checks := map[string]Check{}
	resourceChecks(req, "node1", cpus, memMB, version, func(c Check) { checks[c.ID] = c })
	return checks
}

func TestResourceChecks(t *testing.T) {
	sentry, _ := parseRequirements(sentryMinRequirements)
	sentry.Source = "install/_min-requirements.sh"

	t.Run("failed memory mentions errors-only alternative", func(t *testing.T) {
		got := collectChecks(PreflightRequest{Requirements: &sentry}, 8, 8000, "27.0.0")
		mem := got["memory"]
		if mem.Status != CheckFailed {
			t.Fatalf("memory status = %q", mem.Status)
		}
		if !strings.Contains(mem.Detail, "COMPOSE_PROFILES=errors-only") {
			t.Errorf("detail doesn't mention the alternative: %q", mem.Detail)
		}
		if !strings.Contains(mem.Detail, "install/_min-requirements.sh") {
			t.Errorf("detail doesn't mention the source: %q", mem.Detail)
		}
		if got["cpus"].Status != CheckOK {
			t.Errorf("cpus status = %q, want ok", got["cpus"].Status)
		}
		if got["docker"].Status != CheckOK {
			t.Errorf("docker status = %q, want ok", got["docker"].Status)
		}
	})

	t.Run("errors-only profile fits", func(t *testing.T) {
		req := PreflightRequest{Requirements: &sentry, Variables: map[string]string{"COMPOSE_PROFILES": "errors-only"}}
		got := collectChecks(req, 2, 8000, "27.0.0")
		if got["memory"].Status != CheckOK || got["cpus"].Status != CheckOK {
			t.Errorf("memory %q, cpus %q; want ok", got["memory"].Status, got["cpus"].Status)
		}
		if !strings.Contains(got["memory"].Detail, `COMPOSE_PROFILES is "errors-only"`) {
			t.Errorf("detail = %q", got["memory"].Detail)
		}
	})

	t.Run("too few cpus and old docker", func(t *testing.T) {
		got := collectChecks(PreflightRequest{Requirements: &sentry}, 2, 16000, "19.03.5")
		if got["memory"].Status != CheckOK {
			t.Errorf("memory status = %q, want ok", got["memory"].Status)
		}
		if got["cpus"].Status != CheckFailed {
			t.Errorf("cpus status = %q, want failed", got["cpus"].Status)
		}
		if got["docker"].Status != CheckFailed {
			t.Errorf("docker status = %q, want failed", got["docker"].Status)
		}
	})

	t.Run("no requirements", func(t *testing.T) {
		got := collectChecks(PreflightRequest{}, 4, 4096, "27.0.0")
		if got["memory"].Status != CheckInfo || got["cpus"].Status != CheckInfo {
			t.Errorf("memory %q, cpus %q; want info", got["memory"].Status, got["cpus"].Status)
		}
		if _, ok := got["docker"]; ok {
			t.Error("unexpected docker check")
		}
	})

	t.Run("reservations raise the need", func(t *testing.T) {
		req := PreflightRequest{Requirements: &Requirements{
			Variants:         []RequirementVariant{{MemoryMB: 1024, CPUs: 1}},
			ReservedMemoryMB: 4096,
			ReservedCPUs:     3.5,
		}}
		got := collectChecks(req, 2, 2048, "27.0.0")
		if got["memory"].Status != CheckFailed || !strings.Contains(got["memory"].Detail, "reserve") {
			t.Errorf("memory = %+v", got["memory"])
		}
		if got["cpus"].Status != CheckFailed || !strings.Contains(got["cpus"].Title, "3.5") {
			t.Errorf("cpus = %+v", got["cpus"])
		}
	})

	t.Run("reservations only", func(t *testing.T) {
		req := PreflightRequest{Requirements: &Requirements{ReservedMemoryMB: 512, ReservedCPUs: 1}}
		got := collectChecks(req, 2, 2048, "")
		if got["memory"].Status != CheckOK || got["cpus"].Status != CheckOK {
			t.Errorf("memory %q, cpus %q; want ok", got["memory"].Status, got["cpus"].Status)
		}
	})
}

func TestFormatMemory(t *testing.T) {
	cases := map[int]string{512: "512 MB", 1024: "1 GB", 1536: "1.5 GB", 7000: "6.8 GB", 14000: "14 GB", 16384: "16 GB"}
	for mb, want := range cases {
		if got := formatMemory(mb); got != want {
			t.Errorf("formatMemory(%d) = %q, want %q", mb, got, want)
		}
	}
}
