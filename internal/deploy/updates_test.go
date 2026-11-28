package deploy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/distribution/reference"
)

const lsRemoteOutput = `1ce75d01b6978863647da42557a707a479da3a51	HEAD
1ce75d01b6978863647da42557a707a479da3a51	refs/heads/master
5f1e0c2d9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d	refs/heads/v2
11602f083ca275dcfd4341641ae7fe338b7f6f69	refs/tags/v1.30.0
7c48c2bd72b9bf5c44d21d7338cc7bea77d0ad2a	refs/tags/v1.30.0^{}
932937c4e8307a4ed8b5152bd871022746453de6	refs/tags/v2
warning: not a ref line
`

func TestParseLsRemote(t *testing.T) {
	refs := parseLsRemote(lsRemoteOutput)
	if len(refs) != 6 {
		t.Fatalf("got %d refs, want 6: %v", len(refs), refs)
	}
	if refs["refs/tags/v1.30.0^{}"] != "7c48c2bd72b9bf5c44d21d7338cc7bea77d0ad2a" {
		t.Errorf("peeled tag = %q", refs["refs/tags/v1.30.0^{}"])
	}
	if len(parseLsRemote("")) != 0 {
		t.Error("empty output should have no refs")
	}
}

func TestResolveRemoteRef(t *testing.T) {
	refs := parseLsRemote(lsRemoteOutput)
	cases := []struct {
		ref, commit, kind string
		ok                bool
	}{
		{"", "1ce75d01b6978863647da42557a707a479da3a51", "default", true},
		{"master", "1ce75d01b6978863647da42557a707a479da3a51", "branch", true},
		// An annotated tag resolves to the commit it points to
		{"v1.30.0", "7c48c2bd72b9bf5c44d21d7338cc7bea77d0ad2a", "tag", true},
		// A branch wins over a tag of the same name
		{"v2", "5f1e0c2d9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d", "branch", true},
		{"refs/tags/v2", "932937c4e8307a4ed8b5152bd871022746453de6", "tag", true},
		{"missing", "", "", false},
	}
	for _, c := range cases {
		commit, kind, ok := resolveRemoteRef(refs, c.ref)
		if commit != c.commit || kind != c.kind || ok != c.ok {
			t.Errorf("resolveRemoteRef(%q) = %q, %q, %v; want %q, %q, %v", c.ref, commit, kind, ok, c.commit, c.kind, c.ok)
		}
	}
}

func TestLsRemotePatterns(t *testing.T) {
	cases := map[string]string{
		"":            "HEAD",
		"main":        "refs/heads/main refs/tags/main refs/tags/main^{}",
		"refs/tags/x": "refs/tags/x refs/tags/x^{}",
	}
	for ref, want := range cases {
		if got := strings.Join(lsRemotePatterns(ref), " "); got != want {
			t.Errorf("lsRemotePatterns(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestSameCommit(t *testing.T) {
	full := "1ce75d01b6978863647da42557a707a479da3a51"
	cases := []struct {
		a, b string
		want bool
	}{
		{full, full, true},
		{full, "1ce75d0", true},
		{"1CE75D0", full, true},
		{full, "1ce75d1", false},
		{full, "", false},
		// Too short to mean anything
		{full, "1ce", false},
	}
	for _, c := range cases {
		if got := sameCommit(c.a, c.b); got != c.want {
			t.Errorf("sameCommit(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestLooksLikeCommit(t *testing.T) {
	for ref, want := range map[string]bool{
		"1ce75d01b6978863647da42557a707a479da3a51": true,
		"1ce75d0": true,
		"main":    false,
		"v1.2.3":  false,
		"24.1.0":  false,
		"":        false,
		"abc":     false,
	} {
		if got := looksLikeCommit(ref); got != want {
			t.Errorf("looksLikeCommit(%q) = %v", ref, got)
		}
	}
}

func TestCheckGitPinnedNeedsNoNetwork(t *testing.T) {
	d := New(nil, nil, Config{})
	full := "1ce75d01b6978863647da42557a707a479da3a51"
	git, err := d.checkGit(context.Background(), &manifest.GitSource{Repo: "https://example.invalid/x/y", Ref: full, Commit: full}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if git.RefKind != "commit" || git.Update {
		t.Errorf("a commit ref should be pinned: %+v", git)
	}
}

func TestCompareURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/getsentry/self-hosted":     "https://github.com/getsentry/self-hosted/compare/a...b",
		"https://github.com/getsentry/self-hosted.git": "https://github.com/getsentry/self-hosted/compare/a...b",
		"https://gitlab.com/group/sub/project":         "https://gitlab.com/group/sub/project/-/compare/a...b",
		"https://codeberg.org/forgejo/forgejo/":        "https://codeberg.org/forgejo/forgejo/compare/a...b",
		"https://git.example.com/me/stack":             "",
		"https://github.com/onlyowner":                 "",
	}
	for repo, want := range cases {
		if got := compareURL(repo, "a", "b"); got != want {
			t.Errorf("compareURL(%q) = %q, want %q", repo, got, want)
		}
	}
}

func TestJSONFields(t *testing.T) {
	body := `{"url":"x","status":"ahead","ahead_by":3,"behind_by":0,"commits":[{"sha":"a"}],"files":[`
	status, ahead := jsonFields(strings.NewReader(body), "status", "ahead_by")
	if status != "ahead" || ahead != "3" {
		t.Errorf("got %q, %q", status, ahead)
	}
	_, total := jsonFields(strings.NewReader(`{"total_commits":12,"commits":[]}`), "", "total_commits")
	if total != "12" {
		t.Errorf("total_commits = %q", total)
	}
}

func TestUpdatesCheckable(t *testing.T) {
	env := &database.Environment{App: "a", Name: "prod"}
	preview := &database.Environment{App: "a", Name: "preview-x", IsPreview: true}
	cases := []struct {
		name string
		env  *database.Environment
		spec *manifest.Spec
		want bool
	}{
		{"git stack", env, &manifest.Spec{Source: &manifest.GitSource{Repo: "https://github.com/a/b"}}, true},
		{"image", env, &manifest.Spec{Image: "postgres:16"}, true},
		{"inline compose", env, &manifest.Spec{Compose: "services: {}"}, true},
		{"preview", preview, &manifest.Spec{Image: "registry/a:abc"}, false},
		{"no spec", env, nil, false},
	}
	for _, c := range cases {
		if got := UpdatesCheckable(c.env, c.spec); got != c.want {
			t.Errorf("%s: UpdatesCheckable = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSummarizeCheck(t *testing.T) {
	branch := func(update bool) *database.GitUpdate {
		return &database.GitUpdate{RefKind: "branch", Current: "aaaaaaa", Latest: "bbbbbbb", Update: update}
	}
	pinned := &database.GitUpdate{RefKind: "commit", Current: "aaaaaaa", Latest: "aaaaaaa"}
	img := func(update bool, err string) database.ImageUpdate {
		return database.ImageUpdate{Image: "nginx:alpine", Update: update, Error: err}
	}
	cases := []struct {
		name     string
		git      *database.GitUpdate
		images   []database.ImageUpdate
		problems []string
		want     string
	}{
		{"new commit", branch(true), nil, nil, database.UpdateAvailable},
		{"same commit", branch(false), nil, nil, database.UpdateCurrent},
		{"pinned commit", pinned, nil, nil, database.UpdatePinned},
		{"pinned commit, image by tag moved", pinned, []database.ImageUpdate{img(true, "")}, nil, database.UpdateAvailable},
		{"image current", nil, []database.ImageUpdate{img(false, "")}, nil, database.UpdateCurrent},
		{"digest-only images", nil, nil, nil, database.UpdatePinned},
		{"registry down", nil, []database.ImageUpdate{img(false, "rate limited")}, nil, database.UpdateError},
		{"git down", &database.GitUpdate{Current: "aaaaaaa"}, nil, []string{"git ls-remote: no"}, database.UpdateError},
		// One image failing doesn't hide another's update
		{"partial", nil, []database.ImageUpdate{img(true, ""), img(false, "private")}, nil, database.UpdateAvailable},
	}
	for _, c := range cases {
		if got, _ := summarizeCheck(c.git, c.images, c.problems); got != c.want {
			t.Errorf("%s: status %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUpdatedSpec(t *testing.T) {
	spec := &manifest.Spec{
		App: "sentry", Environment: "prod",
		Source: &manifest.GitSource{Repo: "https://github.com/getsentry/self-hosted", Ref: "master", Commit: "1ce75d01b6978863647da42557a707a479da3a51"},
		Setup:  &manifest.Setup{Command: "./install.sh", Force: true},
	}
	updated, err := updatedSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source.Commit != "" || updated.Setup.Force {
		t.Errorf("an update takes the latest of the ref without forcing setup: %+v %+v", updated.Source, updated.Setup)
	}
	if spec.Source.Commit == "" {
		t.Error("the running spec must not change")
	}

	pinned := &manifest.Spec{Source: &manifest.GitSource{Repo: "https://github.com/a/b", Ref: "1ce75d0", Commit: "1ce75d01b6978863647da42557a707a479da3a51"}}
	if updated, _ := updatedSpec(pinned); updated.Source.Commit == "" {
		t.Error("a commit ref keeps its recorded commit")
	}
}

func TestUpdateRunsSetup(t *testing.T) {
	spec := &manifest.Spec{Source: &manifest.GitSource{Repo: "https://github.com/a/b"}, Setup: &manifest.Setup{Command: "./install.sh"}}
	gitUpdate := &database.UpdateCheck{Status: database.UpdateAvailable, Git: &database.GitUpdate{Update: true}}
	imageUpdate := &database.UpdateCheck{Status: database.UpdateAvailable, Git: &database.GitUpdate{}, Images: []database.ImageUpdate{{Update: true}}}

	if got := updateRunsSetup(spec, gitUpdate); got != "./install.sh" {
		t.Errorf("a new commit reruns setup, got %q", got)
	}
	if got := updateRunsSetup(spec, imageUpdate); got != "" {
		t.Errorf("new images alone don't rerun setup, got %q", got)
	}
	spec.Setup.Always = true
	if got := updateRunsSetup(spec, imageUpdate); got != "./install.sh" {
		t.Errorf("setup that always runs runs on updates too, got %q", got)
	}
}

func TestMatchingDigests(t *testing.T) {
	named, _ := reference.ParseNormalizedNamed("nginx:alpine")
	got := matchingDigests([]string{
		"nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"docker.io/library/nginx@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"registry.example.com/nginx@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}, named)
	if len(got) != 2 || !strings.HasSuffix(got[0], "aaaa") || !strings.HasSuffix(got[1], "bbbb") {
		t.Errorf("got %v", got)
	}
}

func TestRetryDelay(t *testing.T) {
	if retryDelay(1) != updateRetryBase || retryDelay(2) != 2*updateRetryBase {
		t.Errorf("retries should double: %s, %s", retryDelay(1), retryDelay(2))
	}
	if retryDelay(50) != updateRetryMax {
		t.Errorf("retries should stop growing at %s, got %s", updateRetryMax, retryDelay(50))
	}
}

func TestGateRefusesWhileRateLimited(t *testing.T) {
	u := newUpdateState()
	u.noteRateLimit("registry-1.docker.io", &errRateLimited{host: "auth.docker.io", until: time.Now().Add(time.Hour)})
	if err := u.gate(context.Background(), "registry-1.docker.io"); err == nil {
		t.Error("a rate-limited host should be left alone")
	}
	if err := u.gate(context.Background(), "ghcr.io"); err != nil {
		t.Errorf("other hosts are fine: %v", err)
	}
}
