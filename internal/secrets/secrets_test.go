package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndKeyPersistence(t *testing.T) {
	t.Setenv("DOCKTRINE_SECRET_KEY", "")
	dir := t.TempDir()

	box, err := LoadBox(dir)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Encrypt("s3cret")
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("key file mode = %v, want 0600", info.Mode().Perm())
	}

	// A second box loads the same key from disk.
	reloaded, err := LoadBox(dir)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := reloaded.Decrypt(encrypted)
	if err != nil || plain != "s3cret" {
		t.Fatalf("Decrypt = %q, %v", plain, err)
	}

	encrypted[len(encrypted)-1] ^= 1
	if _, err := reloaded.Decrypt(encrypted); err == nil {
		t.Error("expected tampered ciphertext to fail")
	}
}

func TestFingerprint(t *testing.T) {
	t.Setenv("DOCKTRINE_SECRET_KEY", "")
	dir := t.TempDir()
	a, _ := LoadBox(dir)
	again, _ := LoadBox(dir)
	other, _ := LoadBox(t.TempDir())

	fp := a.Fingerprint("shells")
	if len(fp) != 16 {
		t.Fatalf("fingerprint %q, want 16 hex characters", fp)
	}
	if again.Fingerprint("shells") != fp {
		t.Error("the same key gave a different fingerprint")
	}
	if a.Fingerprint("backups") == fp {
		t.Error("two purposes share a fingerprint")
	}
	if other.Fingerprint("shells") == fp {
		t.Error("two keys share a fingerprint")
	}
}

func TestInvalidKey(t *testing.T) {
	t.Setenv("DOCKTRINE_SECRET_KEY", "abcd")
	if _, err := LoadBox(t.TempDir()); err == nil {
		t.Error("expected an error for a short key")
	}
}
