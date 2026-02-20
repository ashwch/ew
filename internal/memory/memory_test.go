package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRememberAndSearchExact(t *testing.T) {
	store := Store{}
	if err := store.Remember("clear aws vault", "aws sso logout"); err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	matches := store.Search("clear aws vault", 5)
	if len(matches) == 0 {
		t.Fatalf("expected memory match")
	}
	if matches[0].Command != "aws sso logout" {
		t.Fatalf("unexpected command: %q", matches[0].Command)
	}
	if !matches[0].Exact {
		t.Fatalf("expected exact match")
	}
}

func TestPromoteDemoteAndForget(t *testing.T) {
	store := Store{}
	if err := store.Remember("push current branch", "git push origin HEAD"); err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	original := store.Search("push current branch", 1)[0].Score

	if err := store.Promote("push current branch", "git push origin HEAD"); err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	promoted := store.Search("push current branch", 1)[0].Score
	if promoted <= original {
		t.Fatalf("expected promoted score > original")
	}

	if err := store.Demote("push current branch", "git push origin HEAD"); err != nil {
		t.Fatalf("demote failed: %v", err)
	}
	demoted := store.Search("push current branch", 1)[0].Score
	if demoted >= promoted {
		t.Fatalf("expected demoted score < promoted")
	}

	removed := store.ForgetQuery("push current branch")
	if removed == 0 {
		t.Fatalf("expected entries removed")
	}
	if got := store.Search("push current branch", 5); len(got) != 0 {
		t.Fatalf("expected query memory forgotten")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	store := Store{}
	if err := store.Remember("switch profile staging", "export AWS_PROFILE=staging"); err != nil {
		t.Fatalf("remember failed: %v", err)
	}

	loaded, path, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded.Entries) != 0 {
		t.Fatalf("expected empty store before save")
	}
	if err := Save(path, store); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	again, _, err := Load()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(again.Entries) != 1 {
		t.Fatalf("expected one entry after save, got %d", len(again.Entries))
	}
	if again.Entries[0].Command != "export AWS_PROFILE=staging" {
		t.Fatalf("unexpected command: %q", again.Entries[0].Command)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected private perms, got %o", info.Mode().Perm())
	}
}

func TestLearnBoostsSuccessfulRuns(t *testing.T) {
	store := Store{}
	if err := store.Learn("logout aws sso", "aws sso logout", true); err != nil {
		t.Fatalf("learn success failed: %v", err)
	}
	first := store.Search("logout aws sso", 1)[0].Score
	if err := store.Learn("logout aws sso", "aws sso logout", true); err != nil {
		t.Fatalf("learn success failed: %v", err)
	}
	second := store.Search("logout aws sso", 1)[0].Score
	if second <= first {
		t.Fatalf("expected repeated success to boost memory score")
	}
}
