package database

import (
	"testing"
	"time"

	"github.com/Zeptile/docktrine/internal/manifest"
)

func TestUpdateChecks(t *testing.T) {
	db, _ := openTestDB(t)

	env, err := db.UpsertEnvironment(&manifest.Spec{App: "sentry", Environment: "prod", Node: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if c, err := db.GetUpdateCheck(env.ID); err != nil || c != nil {
		t.Fatalf("no check yet: %v, %v", c, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	check := &UpdateCheck{
		EnvironmentID: env.ID, DeploymentID: 7, Status: UpdateAvailable,
		Git:       &GitUpdate{Repo: "https://github.com/getsentry/self-hosted", Ref: "master", RefKind: "branch", Current: "aaaaaaa", Latest: "bbbbbbb", Behind: 3, Update: true},
		Images:    []ImageUpdate{{Service: "web", Image: "nginx:alpine", Current: "sha256:a", Latest: "sha256:a"}},
		Failures:  0,
		CheckedAt: now, NextCheckAt: now.Add(6 * time.Hour),
	}
	if err := db.SaveUpdateCheck(check); err != nil {
		t.Fatal(err)
	}
	check.Status, check.Failures = UpdateError, 2
	if err := db.SaveUpdateCheck(check); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetUpdateCheck(env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != UpdateError || got.Failures != 2 || got.Git.Behind != 3 || len(got.Images) != 1 || !got.NextCheckAt.Equal(now.Add(6*time.Hour)) {
		t.Errorf("unexpected check %+v", got)
	}
	all, err := db.ListUpdateChecks()
	if err != nil || len(all) != 1 || all[env.ID] == nil {
		t.Errorf("ListUpdateChecks = %v, %v", all, err)
	}

	// Destroying the environment takes its check with it
	if err := db.DeleteEnvironment(env.ID); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetUpdateCheck(env.ID); c != nil {
		t.Error("the check outlived its environment")
	}
}
