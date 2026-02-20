package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadBashHistoryUsesEmbeddedEpochWhenPresent(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "bash_history")
	content := "#1700000000\ngit status\n#1700000100\nls -la\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp bash history failed: %v", err)
	}

	entries, err := loadBashHistory(path)
	if err != nil {
		t.Fatalf("loadBashHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Timestamp.Unix() != 1700000000 {
		t.Fatalf("expected first timestamp 1700000000, got %d", entries[0].Timestamp.Unix())
	}
	if entries[1].Timestamp.Unix() != 1700000100 {
		t.Fatalf("expected second timestamp 1700000100, got %d", entries[1].Timestamp.Unix())
	}
}

func TestLoadBashHistoryInvalidCommentClearsPendingTimestamp(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "bash_history")
	content := "#1700000000\n# not-a-timestamp\necho hello\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp bash history failed: %v", err)
	}

	entries, err := loadBashHistory(path)
	if err != nil {
		t.Fatalf("loadBashHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Timestamp.Unix() == 1700000000 {
		t.Fatalf("expected stale pending timestamp to be cleared on invalid comment line")
	}
}

func TestLoadFishHistoryParsesWhenField(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "fish_history")
	content := "- cmd: git status\n  when: 1700000200\n- cmd: ls -la\n  when: 1700000300\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp fish history failed: %v", err)
	}

	entries, err := loadFishHistory(path)
	if err != nil {
		t.Fatalf("loadFishHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Timestamp.Unix() != 1700000200 {
		t.Fatalf("expected first timestamp 1700000200, got %d", entries[0].Timestamp.Unix())
	}
	if entries[1].Timestamp.Unix() != 1700000300 {
		t.Fatalf("expected second timestamp 1700000300, got %d", entries[1].Timestamp.Unix())
	}
}

func TestLoadFishHistoryFallbackTimestamp(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "fish_history")
	content := "- cmd: echo hello\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp fish history failed: %v", err)
	}

	before := time.Now().UTC().Add(-5 * time.Second)
	entries, err := loadFishHistory(path)
	if err != nil {
		t.Fatalf("loadFishHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Timestamp.Before(before) {
		t.Fatalf("expected fallback timestamp near now, got %s", entries[0].Timestamp.Format(time.RFC3339))
	}
}

func TestLoadBashHistoryFallbackTimestampsPreserveCommandOrder(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "bash_history")
	content := "echo old\n echo newer\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp bash history failed: %v", err)
	}

	entries, err := loadBashHistory(path)
	if err != nil {
		t.Fatalf("loadBashHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if !entries[1].Timestamp.After(entries[0].Timestamp) {
		t.Fatalf("expected newer command to have newer timestamp; got %s then %s", entries[0].Timestamp.Format(time.RFC3339), entries[1].Timestamp.Format(time.RFC3339))
	}
}
