package database

import (
	"errors"
	"testing"
)

func TestInviteAndSignIn(t *testing.T) {
	db, _ := openTestDB(t)

	u, token, err := db.CreateUser("  Alex ", "Alex Doe", RoleDeployer)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "alex" || !u.Invited || u.InviteExpiresAt == nil {
		t.Fatalf("unexpected new user %+v", u)
	}
	if got, _ := db.CheckPassword("alex", ""); got != nil {
		t.Error("an invited user signed in without a password")
	}

	if _, err := db.AcceptInvite(token, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("weak password: got %v", err)
	}
	if _, err := db.AcceptInvite(token, "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if again, _ := db.AcceptInvite(token, "another password"); again != nil {
		t.Error("an invite worked twice")
	}

	if got, _ := db.CheckPassword("ALEX", "correct horse battery"); got == nil || got.Invited {
		t.Errorf("sign-in failed: %+v", got)
	}
	if got, _ := db.CheckPassword("alex", "wrong password!"); got != nil {
		t.Error("signed in with a wrong password")
	}
	if got, _ := db.CheckPassword("nobody", "correct horse battery"); got != nil {
		t.Error("signed in as a user that doesn't exist")
	}
}

func TestUsernamesAreValidatedAndUnique(t *testing.T) {
	db, _ := openTestDB(t)

	for _, name := range []string{"", "has space", "-dash", "ünï"} {
		if _, _, err := db.CreateUser(name, "", RoleViewer); !errors.Is(err, ErrInvalidUsername) {
			t.Errorf("%q: got %v", name, err)
		}
	}
	if _, _, err := db.CreateUser("sam", "", "owner"); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("bad role: got %v", err)
	}
	db.CreateUser("sam", "", RoleViewer)
	if _, _, err := db.CreateUser("Sam", "", RoleViewer); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("duplicate: got %v", err)
	}
}

func TestLastAdminStays(t *testing.T) {
	db, _ := openTestDB(t)

	admin, _, _ := db.CreateUser("root", "", RoleAdmin)
	if _, err := db.UpdateUser(admin.ID, "", RoleViewer); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demoting the last admin: got %v", err)
	}
	if _, err := db.DeleteUser(admin.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("removing the last admin: got %v", err)
	}

	other, _, _ := db.CreateUser("ops", "", RoleAdmin)
	if ok, err := db.DeleteUser(admin.ID); !ok || err != nil {
		t.Errorf("removing an admin while another remains: %v %v", ok, err)
	}
	if u, _ := db.GetUser(other.ID); u == nil || u.Role != RoleAdmin {
		t.Error("the remaining admin changed")
	}
}

func TestSessionsAndPasswordChanges(t *testing.T) {
	db, _ := openTestDB(t)

	u, token, _ := db.CreateUser("kim", "", RoleViewer)
	db.AcceptInvite(token, "first password")

	phone, _ := db.CreateSession(u.ID, "phone")
	laptop, _ := db.CreateSession(u.ID, "laptop")
	s, got, err := db.AuthenticateSession(laptop)
	if err != nil || got == nil || got.ID != u.ID {
		t.Fatalf("session didn't authenticate: %v %v", got, err)
	}

	if err := db.ChangePassword(u.ID, "wrong one!!", "second password", s.ID); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("wrong current password: got %v", err)
	}
	if err := db.ChangePassword(u.ID, "first password", "second password", s.ID); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := db.AuthenticateSession(phone); s != nil {
		t.Error("other sessions survived a password change")
	}
	if s, _, _ := db.AuthenticateSession(laptop); s == nil {
		t.Error("the session that changed the password was signed out")
	}

	if _, err := db.ResetUserPassword(u.ID); err != nil {
		t.Fatal(err)
	}
	if s, _, _ := db.AuthenticateSession(laptop); s != nil {
		t.Error("sessions survived a password reset")
	}
	if got, _ := db.CheckPassword("kim", "second password"); got != nil {
		t.Error("the old password works after a reset")
	}
}

func TestKeysBelongToUsers(t *testing.T) {
	db, _ := openTestDB(t)

	u, _, _ := db.CreateUser("lee", "", RoleDeployer)
	k, err := db.CreateAPIKey("lee-laptop", &u.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := db.AuthenticateAPIKey(k.Key)
	if got == nil || got.UserID == nil || *got.UserID != u.ID || got.Owner != "lee" {
		t.Fatalf("key owner not recorded: %+v", got)
	}

	mine, _ := db.ListAPIKeys(&u.ID)
	all, _ := db.ListAPIKeys(nil)
	if len(mine) != 1 || len(all) != 2 {
		t.Errorf("got %d own keys and %d in all, want 1 and 2", len(mine), len(all))
	}

	db.DeleteUser(u.ID)
	if got, _ := db.AuthenticateAPIKey(k.Key); got != nil {
		t.Error("a removed user's key still works")
	}
}

func TestLastKeyRevocableOnceSomeoneHasAPassword(t *testing.T) {
	db, _ := openTestDB(t)

	keys, _ := db.ListAPIKeys(nil)
	if _, err := db.RevokeAPIKey(keys[0].ID); !errors.Is(err, ErrLastAPIKey) {
		t.Fatalf("revoking the only key with no users: got %v", err)
	}

	_, token, _ := db.CreateUser("root", "", RoleAdmin)
	db.AcceptInvite(token, "a long password")
	if ok, err := db.RevokeAPIKey(keys[0].ID); !ok || err != nil {
		t.Errorf("revoking the last key with a password user: %v %v", ok, err)
	}
}
