package history

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDedupeEntriesKeepsNewestTimestampPerCommand(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-2 * time.Hour)
	newer := now.Add(-1 * time.Hour)

	entries := []Entry{
		{Command: "git status", Timestamp: old, Source: "zsh"},
		{Command: "git status", Timestamp: newer, Source: "bash"},
		{Command: "ls -la", Timestamp: now, Source: "zsh"},
	}

	deduped := dedupeEntries(entries)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduped entries, got %d", len(deduped))
	}

	var git Entry
	found := false
	for _, entry := range deduped {
		if entry.Command == "git status" {
			git = entry
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected deduped output to contain git status")
	}
	if !git.Timestamp.Equal(newer) {
		t.Fatalf("expected newest git status timestamp %s, got %s", newer.Format(time.RFC3339), git.Timestamp.Format(time.RFC3339))
	}
}

func TestDedupeEntriesSkipsInternalEWCommands(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{
		{Command: "go run ./cmd/ew how to git push current branch", Timestamp: now, Source: "zsh"},
		{Command: "ew find kube logs", Timestamp: now.Add(-time.Minute), Source: "zsh"},
		{Command: "git status", Timestamp: now.Add(-2 * time.Minute), Source: "zsh"},
	}

	deduped := dedupeEntries(entries)
	if len(deduped) != 1 {
		t.Fatalf("expected only non-internal entry to remain, got %d", len(deduped))
	}
	if deduped[0].Command != "git status" {
		t.Fatalf("expected git status to remain, got %q", deduped[0].Command)
	}
}

func TestDedupeEntriesSkipsLikelyShellOutputLines(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{
		{Command: "zsh: killed     uv run scripts/create_worktree.py", Timestamp: now, Source: "zsh"},
		{Command: "aws: [ERROR]: argument command", Timestamp: now.Add(-time.Second), Source: "zsh"},
		{Command: "npm error ENOTEMPTY: directory not empty", Timestamp: now.Add(-1200 * time.Millisecond), Source: "zsh"},
		{Command: "Worktree created successfully", Timestamp: now.Add(-1500 * time.Millisecond), Source: "zsh"},
		{Command: "⃼�� Worktree created", Timestamp: now.Add(-1700 * time.Millisecond), Source: "zsh"},
		{Command: "? Do you want to create a detached worktree instead? Yes", Timestamp: now.Add(-1800 * time.Millisecond), Source: "zsh"},
		{Command: "git worktree add ../repo-wt -b feat/new", Timestamp: now.Add(-2 * time.Second), Source: "zsh"},
	}

	deduped := dedupeEntries(entries)
	if len(deduped) != 1 {
		t.Fatalf("expected one valid command after filtering, got %d", len(deduped))
	}
	if deduped[0].Command != "git worktree add ../repo-wt -b feat/new" {
		t.Fatalf("unexpected command retained: %q", deduped[0].Command)
	}
}

func TestIsLikelyShellOutputForNpmErrorLine(t *testing.T) {
	if !isLikelyShellOutput("npm error ENOTEMPTY: directory not empty") {
		t.Fatalf("expected npm error output line to be filtered")
	}
	if isLikelyShellOutput("npm run build") {
		t.Fatalf("did not expect normal npm command to be filtered")
	}
}

func TestIsLikelyCommandStarter(t *testing.T) {
	if !isLikelyCommandStarter('g') {
		t.Fatalf("expected lowercase letter starter to be valid")
	}
	if !isLikelyCommandStarter('A') {
		t.Fatalf("expected uppercase letter starter to be valid for env-prefixed commands")
	}
	if isLikelyCommandStarter('?') {
		t.Fatalf("expected question mark starter to be invalid")
	}
}

func TestDedupeEntriesKeepsLatestWhenTimestampsEqual(t *testing.T) {
	now := time.Now().UTC()
	entries := []Entry{
		{Command: "git status", Timestamp: now, Source: "zsh", order: 1},
		{Command: "git status", Timestamp: now, Source: "zsh", order: 2},
	}

	deduped := dedupeEntries(entries)
	if len(deduped) != 1 {
		t.Fatalf("expected one deduped entry, got %d", len(deduped))
	}
	if deduped[0].order != 2 {
		t.Fatalf("expected latest entry (order=2), got order=%d", deduped[0].order)
	}
}

func TestScoreCommandRejectsWeakSingleTokenMatchForLongQuery(t *testing.T) {
	query := "git push current branch"
	tokens := splitTokens(query)

	score := scoreCommand(query, tokens, strings.ToLower("python install-skill-from-github.py"), 0, time.Minute)
	if score != 0 {
		t.Fatalf("expected weak single-token match to score zero, got %f", score)
	}
}

func TestScoreCommandBoostsGitPushCurrentBranch(t *testing.T) {
	query := "git push current branch"
	tokens := splitTokens(query)

	score := scoreCommand(query, tokens, strings.ToLower(`git push -u origin "$(git branch --show-current)"`), 5, time.Minute)
	if score <= 0 {
		t.Fatalf("expected git push current-branch command to have positive score")
	}
}

func TestScoreCommandPrefersHigherTokenCoverage(t *testing.T) {
	query := "git push current branch"
	tokens := splitTokens(query)

	partial := scoreCommand(query, tokens, strings.ToLower("git push origin"), 5, time.Minute)
	full := scoreCommand(query, tokens, strings.ToLower(`git push -u origin "$(git branch --show-current)"`), 5, time.Minute)
	if full <= partial {
		t.Fatalf("expected fuller token coverage to score higher: full=%f partial=%f", full, partial)
	}
}

func TestScoreCommandPenalizesMissingDistinctiveToken(t *testing.T) {
	query := "find my global gitignore file"
	tokens := splitTokens(query)

	bad := scoreCommand(query, tokens, strings.ToLower("poetry run report --input global-opps-company-user-data-file.csv"), 500, 30*24*time.Hour)
	good := scoreCommand(query, tokens, strings.ToLower("git config --global --get ~/.gitignore_global"), 500, 30*24*time.Hour)
	if good <= bad {
		t.Fatalf("expected command with distinctive token match to score higher: good=%f bad=%f", good, bad)
	}
}

func TestSplitTokensDropsVeryShortNoiseTokens(t *testing.T) {
	tokens := splitTokens("push to gh")
	if len(tokens) != 1 || tokens[0] != "push" {
		t.Fatalf("expected only push token, got %#v", tokens)
	}
}

func TestSplitTokensDropsCommandMetaWords(t *testing.T) {
	tokens := splitTokens("command to create new worktree")
	if len(tokens) != 3 {
		t.Fatalf("expected command stopword to be removed, got %#v", tokens)
	}
	if strings.Join(tokens, " ") != "create new worktree" {
		t.Fatalf("unexpected token output: %#v", tokens)
	}
}

func TestSplitTokensDropsPathAndFileMetaWords(t *testing.T) {
	tokens := splitTokens("path to global gitignore file")
	if len(tokens) != 2 {
		t.Fatalf("expected two content tokens, got %#v", tokens)
	}
	if strings.Join(tokens, " ") != "global gitignore" {
		t.Fatalf("expected path/file words dropped, got %#v", tokens)
	}
}

func TestNormalizeHistoryCommandStripsTrailingContinuationSlash(t *testing.T) {
	got := normalizeHistoryCommand(`git push origin master\`)
	if got != "git push origin master" {
		t.Fatalf("expected cleaned command without trailing slash, got %q", got)
	}
}

func TestNormalizeHistoryCommandStripsPromptClockSuffix(t *testing.T) {
	got := normalizeHistoryCommand("uv run scripts/create_worktree.py                                                                                           03:44")
	if got != "uv run scripts/create_worktree.py" {
		t.Fatalf("expected trailing prompt clock to be removed, got %q", got)
	}
}

func TestIsLikelyShellOutputForNarrativeLine(t *testing.T) {
	line := "Executes the dedicated worktree creation script via uv. This is the only legitimate command."
	if !isLikelyShellOutput(line) {
		t.Fatalf("expected narrative output line to be filtered")
	}
}

func TestIsLikelyShellOutputForEnumeratedLine(t *testing.T) {
	if !isLikelyShellOutput("1. git push origin HEAD") {
		t.Fatalf("expected numbered output line to be filtered")
	}
}

func TestLatestEntryReturnsMostRecentNonInternalCommand(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	now := time.Now().UTC()
	old := now.Add(-2 * time.Minute).Unix()
	recent := now.Add(-30 * time.Second).Unix()

	content := strings.Join([]string{
		": " + formatUnix(old) + ":0;git status",
		": " + formatUnix(recent) + ":0;aws llo sso",
		": " + formatUnix(recent+1) + ":0;go run ./cmd/ew",
		"",
	}, "\n")
	path := filepath.Join(tempDir, ".zsh_history")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write zsh history failed: %v", err)
	}

	latest, err := LatestEntry(5 * time.Minute)
	if err != nil {
		t.Fatalf("LatestEntry failed: %v", err)
	}
	if latest == nil {
		t.Fatalf("expected latest entry, got nil")
	}
	if latest.Command != "aws llo sso" {
		t.Fatalf("expected latest non-internal command, got %q", latest.Command)
	}
}

func TestLatestEntryReturnsNilWhenOlderThanMaxAge(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	old := time.Now().UTC().Add(-2 * time.Hour).Unix()
	content := ": " + formatUnix(old) + ":0;git status\n"
	path := filepath.Join(tempDir, ".zsh_history")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write zsh history failed: %v", err)
	}

	latest, err := LatestEntry(5 * time.Minute)
	if err != nil {
		t.Fatalf("LatestEntry failed: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected no recent entry, got %q", latest.Command)
	}
}

func TestLatestEntrySkipsApproximateTimestamps(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	content := "aws llo sso\n"
	path := filepath.Join(tempDir, ".zsh_history")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write zsh history failed: %v", err)
	}

	latest, err := LatestEntry(5 * time.Minute)
	if err != nil {
		t.Fatalf("LatestEntry failed: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil for approximate timestamp-only history, got %q", latest.Command)
	}
}

func formatUnix(ts int64) string {
	return strconv.FormatInt(ts, 10)
}
