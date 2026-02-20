package systemprofile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ashwch/ew/internal/appdirs"
)

func TestEnsureCreatesProfileOnFirstRun(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	profile, status, err := Ensure(Options{AutoTrain: true, RefreshHours: 168})
	if err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	if !status.Created {
		t.Fatalf("expected created=true for first run")
	}
	if strings.TrimSpace(profile.CapturedAt) == "" {
		t.Fatalf("expected captured timestamp")
	}
	if profile.OS != runtime.GOOS {
		t.Fatalf("expected os=%q got=%q", runtime.GOOS, profile.OS)
	}
}

func TestSaveAndEnsureRoundTrip(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	original := Profile{
		Version:         1,
		CapturedAt:      time.Now().UTC().Format(time.RFC3339),
		OS:              "darwin",
		Arch:            "arm64",
		Shell:           "zsh",
		Locale:          "en_US.UTF-8",
		ConfigFiles:     []string{"~/.zshrc", "~/.gitconfig"},
		Tools:           []string{"git", "go", "uv"},
		GitGlobalIgnore: "~/.config/git/ignore",
		UserNote:        "use fish aliases when possible",
	}
	if err := Save(original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	profile, status, err := Ensure(Options{AutoTrain: false, RefreshHours: 168})
	if err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	if status.Created || status.Refreshed {
		t.Fatalf("did not expect capture refresh when profile exists and autotraining disabled")
	}
	if profile.UserNote != original.UserNote {
		t.Fatalf("expected preserved user note, got %q", profile.UserNote)
	}
	if profile.GitGlobalIgnore != original.GitGlobalIgnore {
		t.Fatalf("expected git ignore path, got %q", profile.GitGlobalIgnore)
	}
}

func TestPromptContextIncludesNoteAndTools(t *testing.T) {
	profile := Profile{
		Version:     1,
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		OS:          "darwin",
		Arch:        "arm64",
		Shell:       "zsh",
		ConfigFiles: []string{"~/.zshrc"},
		Tools:       []string{"git", "go", "uv"},
		UserNote:    "prefer uv for python tasks",
	}
	context := profile.PromptContext(8)
	if !strings.Contains(context, "os=darwin") {
		t.Fatalf("expected os context, got %q", context)
	}
	if !strings.Contains(context, "tools=git, go, uv") {
		t.Fatalf("expected tools context, got %q", context)
	}
	if !strings.Contains(context, "user_note=prefer uv for python tasks") {
		t.Fatalf("expected note in context, got %q", context)
	}
}

func TestSaveUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}

	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	profile := Capture()
	if err := Save(profile); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	path, err := appdirs.StateFilePath(profileFileName)
	if err != nil {
		t.Fatalf("state path failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile failed: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected private permissions, got %o", info.Mode().Perm())
	}
}

func TestIsStale(t *testing.T) {
	fresh := Profile{CapturedAt: time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)}
	if fresh.IsStale(24) {
		t.Fatalf("expected profile to be fresh")
	}
	old := Profile{CapturedAt: time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)}
	if !old.IsStale(24) {
		t.Fatalf("expected profile to be stale")
	}
}
