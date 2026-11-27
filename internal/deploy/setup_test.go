package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/manifest"
	_ "github.com/mattn/go-sqlite3"
)

func TestSetupReason(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir())
	db, err := database.NewDatabaseConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	d := New(db, nil, Config{})

	spec := func(commit, command string, mods ...func(*manifest.Spec)) *manifest.Spec {
		s := &manifest.Spec{
			App: "sentry", Environment: "prod", Node: "local",
			Source: &manifest.GitSource{Repo: "https://github.com/getsentry/self-hosted", Commit: commit},
		}
		if command != "" {
			s.Setup = &manifest.Setup{Command: command}
		}
		for _, m := range mods {
			m(s)
		}
		return s
	}
	// deploy records a deployment with the given outcome and returns its id
	deploy := func(s *manifest.Spec, status string) int64 {
		env, err := db.UpsertEnvironment(s)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := db.CreateDeployment(env, s, "test")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.SetDeploymentStatus(dep.ID, status, ""); err != nil {
			t.Fatal(err)
		}
		return dep.ID
	}
	expect := func(s *manifest.Spec, id int64, wantRun bool, contains string) {
		t.Helper()
		reason, err := d.setupReason(s, id)
		if err != nil {
			t.Fatal(err)
		}
		if (reason != "") != wantRun || !strings.Contains(reason, contains) {
			t.Errorf("reason = %q, want run=%v containing %q", reason, wantRun, contains)
		}
	}

	first := spec("aaaaaaaa", "./install.sh")
	expect(first, deploy(first, database.DeploymentRunning), true, "first deploy")
	deploy(first, database.DeploymentSucceeded)

	// Same commit and command: already done
	again := spec("aaaaaaaa", "./install.sh")
	expect(again, deploy(again, database.DeploymentRunning), false, "")

	// A failed deploy doesn't count as having run it
	deploy(spec("bbbbbbbb", "./install.sh"), database.DeploymentFailed)
	expect(again, 0, false, "")

	newer := spec("cccccccc", "./install.sh")
	expect(newer, 0, true, "aaaaaaa to ccccccc")

	changed := spec("aaaaaaaa", "./install.sh --skip-user-creation")
	expect(changed, 0, true, "command changed")

	forced := spec("aaaaaaaa", "./install.sh", func(s *manifest.Spec) { s.Setup.Force = true })
	expect(forced, 0, true, "asked for")

	always := spec("aaaaaaaa", "./install.sh", func(s *manifest.Spec) { s.Setup.Always = true })
	expect(always, 0, true, "every deploy")

	// The setup command was added after the environment ran without one
	plain := spec("dddddddd", "")
	plain.Environment = "dev"
	deploy(plain, database.DeploymentSucceeded)
	added := spec("dddddddd", "./install.sh")
	added.Environment = "dev"
	expect(added, 0, true, "hasn't run")
}

func TestSuggestSetup(t *testing.T) {
	dir := t.TempDir()
	if got := suggestSetup(dir); got != "" {
		t.Errorf("no script: got %q", got)
	}

	os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/bash\nsource install/parse-cli.sh\n"), 0755)
	os.MkdirAll(filepath.Join(dir, "install"), 0755)
	os.WriteFile(filepath.Join(dir, "install", "parse-cli.sh"), []byte(`
 --skip-user-creation   Skip the initial user creation prompt
 --no-report-self-hosted-issues
 --apply-automatic-config-updates
 --skip-commit-check
`), 0644)
	want := "./install.sh --skip-user-creation --no-report-self-hosted-issues --apply-automatic-config-updates --skip-commit-check"
	if got := suggestSetup(dir); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSpecSetupNeedsGit(t *testing.T) {
	s := &manifest.Spec{App: "a", Environment: "prod", Compose: "services: {}", Setup: &manifest.Setup{Command: "./install.sh"}}
	if err := s.Validate(); err == nil {
		t.Error("a setup command without a git source should be rejected")
	}
	s.Compose = ""
	s.Source = &manifest.GitSource{Repo: "https://github.com/a/b"}
	s.Setup.Command = "  "
	if err := s.Validate(); err == nil {
		t.Error("a blank setup command should be rejected")
	}
}
