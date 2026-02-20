package appdirs

import (
	"os"
	"runtime"
	"testing"
)

func TestEnsureConfigDirUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := EnsureConfigDir()
	if err != nil {
		t.Fatalf("EnsureConfigDir failed: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat config dir failed: %v", err)
	}
	if perms := info.Mode().Perm(); perms&0o077 != 0 {
		t.Fatalf("expected private config dir permissions, got %o", perms)
	}
}

func TestEnsureStateDirUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	dir, err := EnsureStateDir()
	if err != nil {
		t.Fatalf("EnsureStateDir failed: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat state dir failed: %v", err)
	}
	if perms := info.Mode().Perm(); perms&0o077 != 0 {
		t.Fatalf("expected private state dir permissions, got %o", perms)
	}
}
