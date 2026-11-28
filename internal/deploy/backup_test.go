package deploy

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/docker/docker/api/types/volume"
)

func TestBackupReason(t *testing.T) {
	cases := []struct {
		policy, setup, upgrade, want string
	}{
		// The default backs up before setup, and only then
		{manifest.BackupBeforeSetup, "the stack changed", "the stack changes from a to b", "the setup command runs"},
		{manifest.BackupBeforeSetup, "", "the image changes from a to b", ""},
		{manifest.BackupBeforeSetup, "", "", ""},
		{manifest.BackupBeforeUpgrade, "", "the image changes from a to b", "the image changes from a to b"},
		{manifest.BackupBeforeUpgrade, "this is the first deploy", "", "the setup command runs"},
		{manifest.BackupBeforeUpgrade, "", "", ""},
		{manifest.BackupNever, "the setup command changed", "the image changes from a to b", ""},
	}
	for _, c := range cases {
		if got := backupReason(c.policy, c.setup, c.upgrade); got != c.want {
			t.Errorf("backupReason(%q, %q, %q) = %q, want %q", c.policy, c.setup, c.upgrade, got, c.want)
		}
	}
}

func TestBackupBeforeDefault(t *testing.T) {
	spec := &manifest.Spec{}
	if got := spec.BackupBefore(); got != manifest.BackupBeforeSetup {
		t.Errorf("without a backup setting: %q", got)
	}
	spec.Backup = &manifest.Backup{Keep: 3}
	if got := spec.BackupBefore(); got != manifest.BackupBeforeSetup {
		t.Errorf("with only keep: %q", got)
	}
	if err := (&manifest.Backup{Before: "always"}).Validate(); err == nil {
		t.Error("an unknown policy was accepted")
	}
	if err := (&manifest.Backup{Keep: -1}).Validate(); err == nil {
		t.Error("a negative keep was accepted")
	}
	// Specs stored before backups existed still deploy
	if (target{spec: &manifest.Spec{}}).backupPolicy() != manifest.BackupBeforeSetup {
		t.Error("an old spec doesn't back up before setup")
	}
	if (target{spec: &manifest.Spec{Backup: &manifest.Backup{Before: "upgrade"}}, skipBackup: true}).backupPolicy() != manifest.BackupNever {
		t.Error("skipBackup didn't turn backups off")
	}
}

func TestVersionChange(t *testing.T) {
	git := func(repo, commit string) *manifest.Spec {
		return &manifest.Spec{Source: &manifest.GitSource{Repo: repo, Commit: commit}}
	}
	image := func(ref string) *manifest.Spec { return &manifest.Spec{Image: ref} }
	compose := func(dbImage string) *manifest.Spec {
		return &manifest.Spec{Compose: "services:\n  web:\n    image: nginx:1.27\n  db:\n    image: " + dbImage + "\n"}
	}

	cases := []struct {
		name       string
		prev, next *manifest.Spec
		want       string
	}{
		{"first deploy", nil, image("postgres:16"), ""},
		{"same image", image("postgres:16"), image("postgres:16"), ""},
		{"other image", image("postgres:15"), image("postgres:16"), "the image changes from postgres:15 to postgres:16"},
		{"other digest", image("app@sha256:aaa"), image("app@sha256:bbb"), "the image changes from app@sha256:aaa to app@sha256:bbb"},
		{"same commit", git("https://x/y", "1234567890"), git("https://x/y", "1234567890"), ""},
		{"other commit", git("https://x/y", "1234567890"), git("https://x/y", "abcdef0123"), "the stack changes from 1234567 to abcdef0"},
		{"commit not known", git("https://x/y", ""), git("https://x/y", "abcdef0123"), ""},
		{"other repository", git("https://x/y", "1"), git("https://x/z", "1"), "the stack's repository changes"},
		{"same compose images", compose("postgres:16"), compose("postgres:16"), ""},
		{"other compose image", compose("postgres:15"), compose("postgres:16"), "db changes from postgres:15 to postgres:16"},
		{"other kind", image("postgres:16"), compose("postgres:16"), "what runs changes from an image to a compose file"},
	}
	for _, c := range cases {
		if got := versionChange(c.prev, c.next); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEnvironmentVolumes(t *testing.T) {
	all := []*volume.Volume{
		{Name: "shop-prod-data", Labels: map[string]string{LabelApp: "shop", LabelEnv: "prod"}},
		{Name: "shop-prod_pgdata", Labels: map[string]string{"com.docker.compose.project": "shop-prod", "com.docker.compose.volume": "pgdata"}},
		// Another environment of the same app, and one whose name only looks alike
		{Name: "shop-dev-data", Labels: map[string]string{LabelApp: "shop", LabelEnv: "dev"}},
		{Name: "shop-prod-old", Labels: map[string]string{}},
		{Name: "shop-prod-extra_cache", Labels: map[string]string{"com.docker.compose.project": "shop-prod-extra"}},
		// Both labels: listed once
		{Name: "shop-prod-both", Labels: map[string]string{LabelApp: "shop", LabelEnv: "prod", "com.docker.compose.project": "shop-prod"}},
		nil,
	}
	got := []string{}
	for _, v := range environmentVolumes(all, "shop", "prod") {
		got = append(got, v.Name)
	}
	want := []string{"shop-prod-both", "shop-prod-data", "shop-prod_pgdata"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSkippedMounts(t *testing.T) {
	stackDir := "/var/lib/docktrine/stacks/sentry-prod"
	mounts := []mountInfo{
		{Container: "db", Type: "volume", Name: "sentry-prod_pg", Target: "/var/lib/postgresql/data"},
		{Container: "web", Type: "bind", Source: stackDir + "/sentry", Target: "/etc/sentry"},
		{Container: "web", Type: "bind", Source: "/srv/uploads", Target: "/uploads"},
		{Container: "worker", Type: "bind", Source: "/srv/uploads", Target: "/uploads"},
		{Container: "web", Type: "volume", Name: "sentry-data", Target: "/data"},
		{Container: "web", Type: "volume", Name: strings.Repeat("a", 64), Target: "/tmp/cache"},
		{Container: "web", Type: "tmpfs", Target: "/run"},
		// Looks like the stack's directory, but isn't inside it
		{Container: "web", Type: "bind", Source: stackDir + "-other", Target: "/x"},
	}
	got := skippedMounts(mounts, map[string]bool{"sentry-prod_pg": true}, stackDir)
	sources := []string{}
	for _, s := range got {
		sources = append(sources, s.Source)
	}
	want := []string{"/srv/uploads", "sentry-data", strings.Repeat("a", 64), stackDir + "-other"}
	if !reflect.DeepEqual(sources, want) {
		t.Fatalf("got %v, want %v", sources, want)
	}
	if got[0].Container != "web, worker" {
		t.Errorf("containers sharing a mount: %q", got[0].Container)
	}
	if !strings.Contains(got[0].Reason, "directory on the node") || !strings.Contains(got[1].Reason, "didn't create") ||
		!strings.Contains(got[2].Reason, "anonymous") {
		t.Errorf("unexpected reasons: %+v", got)
	}

	// Without the stack's directory in the backup, its binds are skipped too
	if got := skippedMounts(mounts[1:2], nil, ""); len(got) != 1 {
		t.Errorf("a bind into an unbacked stack directory wasn't listed: %+v", got)
	}
}

func TestExpiredBackups(t *testing.T) {
	ok, failed, running := database.BackupSucceeded, database.BackupFailed, database.BackupRunning
	list := func(statuses ...string) []database.Backup {
		backups := []database.Backup{}
		// Newest first, as the database lists them
		for i, s := range statuses {
			backups = append(backups, database.Backup{ID: int64(len(statuses) - i), Status: s})
		}
		return backups
	}
	ids := func(backups []database.Backup) []int64 {
		out := []int64{}
		for _, b := range backups {
			out = append(out, b.ID)
		}
		return out
	}

	cases := []struct {
		name     string
		backups  []database.Backup
		keep     int
		expected []int64
	}{
		{"under the limit", list(ok, ok, ok), 5, []int64{}},
		{"over the limit", list(ok, ok, ok, ok), 2, []int64{2, 1}},
		{"failed ones older than a success go", list(ok, failed, ok, failed), 5, []int64{3, 1}},
		{"a failed one newer than every success stays", list(failed, ok), 5, []int64{}},
		{"running ones stay", list(running, ok, ok), 1, []int64{1}},
		// Keeping none still keeps the newest successful backup
		{"keep 0", list(failed, ok, ok), 0, []int64{1}},
		{"only failures", list(failed, failed), 1, []int64{}},
	}
	for _, c := range cases {
		if got := ids(expiredBackups(c.backups, c.keep)); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.expected)
		}
	}
}

func TestValidBackupPath(t *testing.T) {
	valid := []string{
		"/var/lib/docktrine/backups/sentry-prod/20261127-153045",
		"/home/me/backups/a-b/20261127-153045-2",
	}
	invalid := []string{
		"",
		"relative/sentry-prod/20261127-153045",
		"/var/lib/docktrine/backups/sentry-prod/../20261127-153045",
		"/var/lib/docktrine/backups/../../etc/20261127-153045",
		"/var/lib/docktrine/backups/sentry-prod/20261127-153045/..",
		"/var/lib/docktrine/backups/sentry-prod/./20261127-153045",
		"/var/lib/docktrine/backups/sentry-prod/20261127-153045/",
		"/var/lib/docktrine/backups//sentry-prod/20261127-153045",
		"/var/lib/docktrine/backups/sentry-prod",
		"/var/lib/docktrine/backups/Sentry_Prod/20261127-153045",
		"/var/lib/docktrine/backups/sentry-prod/latest",
		// Right under the root, the environment directory would be /x
		"/sentry-prod/20261127-153045",
	}
	for _, p := range valid {
		if !validBackupPath(p) {
			t.Errorf("%q was rejected", p)
		}
	}
	for _, p := range invalid {
		if validBackupPath(p) {
			t.Errorf("%q was accepted", p)
		}
	}
}

func TestArchiveNames(t *testing.T) {
	if got := archiveName(database.BackupItem{Kind: database.BackupItemVolume, Name: "stack"}); got != "volume-stack.tar.gz" {
		t.Errorf("a volume named stack: %q", got)
	}
	if got := archiveName(database.BackupItem{Kind: database.BackupItemStack, Name: "/x/stacks/a-b"}); got != "stack.tar.gz" {
		t.Errorf("the stack directory: %q", got)
	}
	for _, name := range []string{"volume-pg_data.tar.gz", "stack.tar.gz", "volume-a.b.tar.gz"} {
		if !validArchiveName(name) {
			t.Errorf("%q was rejected", name)
		}
	}
	for _, name := range []string{"../x.tar.gz", "a/b.tar.gz", "..tar.gz", ".tar.gz", "-x.tar.gz", "x.tar", "a..b.tar.gz", "x\x00.tar.gz"} {
		if validArchiveName(name) {
			t.Errorf("%q was accepted", name)
		}
	}

	if !validStackDir("/var/lib/docktrine/stacks/sentry-prod", "sentry-prod") {
		t.Error("the stack directory was rejected")
	}
	for _, dir := range []string{"/sentry-prod", "/x/../sentry-prod", "stacks/sentry-prod", "/x/stacks/other-prod", "/x/stacks/sentry-prod/"} {
		if validStackDir(dir, "sentry-prod") {
			t.Errorf("%q was accepted", dir)
		}
	}
}

func TestBackupDirName(t *testing.T) {
	at := time.Date(2026, 11, 27, 15, 30, 45, 0, time.UTC)
	if got := backupDirName(at, nil); got != "20261127-153045" {
		t.Errorf("got %q", got)
	}
	used := map[string]bool{"20261127-153045": true, "20261127-153045-2": true}
	if got := backupDirName(at, used); got != "20261127-153045-3" {
		t.Errorf("got %q", got)
	}
	if !stampPattern.MatchString(backupDirName(at, used)) {
		t.Error("a deduplicated name doesn't pass validation")
	}
}

func TestParseSpace(t *testing.T) {
	out := "1024\t/v/0\n8\t/v/1\nFilesystem           1024-blocks    Used Available Capacity Mounted on\n/dev/nvme0n1p2       487110000 300000000 187110000  62% /backups\n"
	sizes := parseDu(out)
	if sizes["/v/0"] != 1024*1024 || sizes["/v/1"] != 8*1024 {
		t.Errorf("sizes: %v", sizes)
	}
	free, ok := parseDfAvailable(out)
	if !ok || free != 187110000*1024 {
		t.Errorf("free: %d %v", free, ok)
	}
	if _, ok := parseDfAvailable("du: /v/0: No such file\n"); ok {
		t.Error("read free space from output without df")
	}

	// A backup that fits only with no room to spare doesn't
	if spaceNeeded(1<<30) <= 1<<30 {
		t.Error("no headroom")
	}
	if formatBytes(1536) != "1.5 KiB" || formatBytes(12) != "12 B" || formatBytes(3<<30) != "3.0 GiB" {
		t.Errorf("formatBytes: %s %s %s", formatBytes(1536), formatBytes(12), formatBytes(3<<30))
	}
}
