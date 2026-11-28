package database

import (
	"testing"

	"github.com/Zeptile/docktrine/internal/manifest"
)

func TestBackupsOutliveTheEnvironment(t *testing.T) {
	db, _ := openTestDB(t)

	spec := &manifest.Spec{App: "shop", Environment: "prod", Node: "local", Image: "postgres:16"}
	env, err := db.UpsertEnvironment(spec)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := db.CreateDeployment(env, spec, "me")
	if err != nil {
		t.Fatal(err)
	}

	b, err := db.CreateBackup(&Backup{
		App: "shop", Environment: "prod", Server: "local", Path: "/b/shop-prod/20261127-153045",
		Reason: "it was asked for", DeploymentID: &dep.ID, Spec: dep.Spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != BackupRunning || len(b.Items) != 0 || b.Skipped == nil {
		t.Fatalf("unexpected new backup %+v", b)
	}
	items := []BackupItem{
		{Kind: BackupItemVolume, Name: "shop-prod-data", File: "volume-shop-prod-data.tar.gz", Size: 1000, ArchiveSize: 300},
		{Kind: BackupItemStack, Name: "/s/shop-prod", File: "stack.tar.gz", Size: 10, ArchiveSize: 5},
	}
	if err := db.FinishBackup(b.ID, BackupSucceeded, "", items, nil); err != nil {
		t.Fatal(err)
	}

	// Another one, interrupted by a restart
	if _, err := db.CreateBackup(&Backup{App: "shop", Environment: "prod", Server: "local", Path: "/b/shop-prod/20261127-160000", Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := db.FailStaleBackups(); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteEnvironment(env.ID); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListBackups("shop", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d backups after destroying the environment", len(list))
	}
	if list[0].Status != BackupFailed || list[0].Message == "" {
		t.Errorf("the interrupted backup: %+v", list[0])
	}
	got := list[1]
	if got.Status != BackupSucceeded || got.Size != 305 || len(got.Items) != 2 || got.FinishedAt == nil {
		t.Errorf("the finished backup: %+v", got)
	}
	// What ran is kept with it, so restoring can redeploy it
	if got.Spec == nil || got.Spec.Image != "postgres:16" || got.DeploymentID == nil || *got.DeploymentID != dep.ID {
		t.Errorf("the backup lost what ran: %+v", got)
	}

	if err := db.SetBackupRestored(got.ID); err != nil {
		t.Fatal(err)
	}
	if again, _ := db.GetBackup(got.ID); again.RestoredAt == nil {
		t.Error("restored_at wasn't set")
	}
	if err := db.DeleteBackup(got.ID); err != nil {
		t.Fatal(err)
	}
	if gone, _ := db.GetBackup(got.ID); gone != nil {
		t.Error("the backup is still there")
	}
}
