package hook

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ashwch/ew/internal/appdirs"
)

func TestShouldIgnoreCommand(t *testing.T) {
	if !shouldIgnoreCommand("_ew hook-record --command \"ls\"") {
		t.Fatalf("expected internal hook record command to be ignored")
	}
	if !shouldIgnoreCommand("ew find kube logs") {
		t.Fatalf("expected ew command to be ignored")
	}
	if !shouldIgnoreCommand("/usr/local/bin/_ew doctor") {
		t.Fatalf("expected path-based _ew command to be ignored")
	}
	if !shouldIgnoreCommand("env FOO=bar ew find kube logs") {
		t.Fatalf("expected env-prefixed ew command to be ignored")
	}
	if !shouldIgnoreCommand("sudo ew doctor") {
		t.Fatalf("expected sudo ew command to be ignored")
	}
	if !shouldIgnoreCommand("sudo -E ew doctor") {
		t.Fatalf("expected sudo flag-wrapped ew command to be ignored")
	}
	if !shouldIgnoreCommand("env -i FOO=bar ew doctor") {
		t.Fatalf("expected env flag-wrapped ew command to be ignored")
	}
	if !shouldIgnoreCommand("go run ./cmd/ew logout from aws sso") {
		t.Fatalf("expected go run ew command to be ignored")
	}
	if !shouldIgnoreCommand("go run ./cmd/_ew doctor") {
		t.Fatalf("expected go run _ew command to be ignored")
	}
	if shouldIgnoreCommand("git status") {
		t.Fatalf("did not expect normal commands to be ignored")
	}
	if shouldIgnoreCommand("newscript --help") {
		t.Fatalf("did not expect commands containing ew substring to be ignored")
	}
}

func TestPrimaryCommandTokenSkipsWrappersAndEnv(t *testing.T) {
	got := primaryCommandToken([]string{"FOO=bar", "env", "sudo", "/usr/local/bin/ew", "fix"})
	if got != "/usr/local/bin/ew" {
		t.Fatalf("expected primary command token ew path, got %q", got)
	}
}

func TestRecordEventSecuresEventsFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on windows")
	}

	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	err := RecordEvent(Event{
		Command:  "git status",
		ExitCode: 1,
		Shell:    "zsh",
	})
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		t.Fatalf("StateFilePath failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat events file failed: %v", err)
	}
	if perms := info.Mode().Perm(); perms&0o077 != 0 {
		t.Fatalf("expected private events file permissions, got %o", perms)
	}
}

func TestLatestFailureSkipsSyntheticSessionIDs(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	if err := RecordEvent(Event{
		Command:   "foobarcmd --badflag",
		ExitCode:  127,
		Shell:     "zsh",
		SessionID: "ew-prov-test",
		Timestamp: "2026-02-20T10:49:17Z",
	}); err != nil {
		t.Fatalf("RecordEvent synthetic failed: %v", err)
	}

	if err := RecordEvent(Event{
		Command:   "aws llo sso",
		ExitCode:  252,
		Shell:     "zsh",
		SessionID: "12345.67890",
		Timestamp: "2026-02-20T18:25:00Z",
	}); err != nil {
		t.Fatalf("RecordEvent real failed: %v", err)
	}

	ev, err := LatestFailure("")
	if err != nil {
		t.Fatalf("LatestFailure failed: %v", err)
	}
	if ev == nil {
		t.Fatalf("expected failure event")
	}
	if ev.Command != "aws llo sso" {
		t.Fatalf("expected non-synthetic latest failure, got %q", ev.Command)
	}
}

func TestRecordEventRedactsSecretsBeforePersisting(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	rawSecret := "supersecretvalue123"
	rawToken := "tokenvalue456"
	command := "AWS_SECRET_ACCESS_KEY=" + rawSecret + " curl -H 'Authorization: Bearer " + rawToken + "' https://example.com"
	if err := RecordEvent(Event{
		Command:  command,
		ExitCode: 1,
		Shell:    "zsh",
	}); err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		t.Fatalf("StateFilePath failed: %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file failed: %v", err)
	}
	payload := string(bytes)
	if strings.Contains(payload, rawSecret) {
		t.Fatalf("expected secret assignment to be redacted from events file")
	}
	if strings.Contains(payload, rawToken) {
		t.Fatalf("expected bearer token to be redacted from events file")
	}
	if !strings.Contains(payload, "redacted") {
		t.Fatalf("expected redaction marker in persisted event")
	}

	ev, err := LatestFailure("")
	if err != nil {
		t.Fatalf("LatestFailure failed: %v", err)
	}
	if ev == nil {
		t.Fatalf("expected latest failure event")
	}
	if strings.Contains(ev.Command, rawSecret) || strings.Contains(ev.Command, rawToken) {
		t.Fatalf("expected latest failure command to be redacted, got %q", ev.Command)
	}
}

func TestRecordEventRedactsFlagStyleSecretsBeforePersisting(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	rawPassword := "hunter2"
	rawToken := "abc123"
	command := "mycli --password " + rawPassword + " --token=" + rawToken + " --profile dev"
	if err := RecordEvent(Event{
		Command:  command,
		ExitCode: 1,
		Shell:    "zsh",
	}); err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		t.Fatalf("StateFilePath failed: %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file failed: %v", err)
	}
	payload := string(bytes)
	if strings.Contains(payload, rawPassword) || strings.Contains(payload, rawToken) {
		t.Fatalf("expected flag-style secrets to be redacted from events file, got %q", payload)
	}
	if !strings.Contains(payload, "--password") || !strings.Contains(payload, "--token=") || !strings.Contains(payload, "redacted") {
		t.Fatalf("expected flag-style redaction markers in persisted event, got %q", payload)
	}
}

func TestRecordEventRedactsShortSecretFlagsBeforePersisting(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	rawPassword := "hunter2"
	rawKey := "abc123"
	command := "mycli login -p " + rawPassword + " -k=" + rawKey + " --profile dev"
	if err := RecordEvent(Event{
		Command:  command,
		ExitCode: 1,
		Shell:    "zsh",
	}); err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		t.Fatalf("StateFilePath failed: %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file failed: %v", err)
	}
	payload := string(bytes)
	if strings.Contains(payload, rawPassword) || strings.Contains(payload, rawKey) {
		t.Fatalf("expected short-flag secrets to be redacted from events file, got %q", payload)
	}
	if !strings.Contains(payload, "-p") || !strings.Contains(payload, "-k=") || !strings.Contains(payload, "redacted") {
		t.Fatalf("expected short-flag redaction markers in persisted event, got %q", payload)
	}
}

func TestRecordEventRedactsPositionalSecretKeywordsBeforePersisting(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	rawToken := "abc123"
	rawPassword := "hunter2"
	command := "mycli login token " + rawToken + " password " + rawPassword + " --profile dev"
	if err := RecordEvent(Event{
		Command:  command,
		ExitCode: 1,
		Shell:    "zsh",
	}); err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		t.Fatalf("StateFilePath failed: %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file failed: %v", err)
	}
	payload := string(bytes)
	if strings.Contains(payload, rawToken) || strings.Contains(payload, rawPassword) {
		t.Fatalf("expected positional secrets to be redacted from events file, got %q", payload)
	}
	lowerPayload := strings.ToLower(payload)
	if !strings.Contains(lowerPayload, "token") || !strings.Contains(lowerPayload, "password") || !strings.Contains(lowerPayload, "redacted") {
		t.Fatalf("expected positional redaction markers in persisted event, got %q", payload)
	}
}

func TestRecordEventRedactsPrefixedPositionalSecretKeyNameBeforePersisting(t *testing.T) {
	home := t.TempDir()
	stateBase := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", stateBase)

	rawSecret := "ABC123"
	command := "aws configure set aws_secret_access_key " + rawSecret + " --profile dev"
	if err := RecordEvent(Event{
		Command:  command,
		ExitCode: 1,
		Shell:    "zsh",
	}); err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	path, err := appdirs.StateFilePath(eventsFileName)
	if err != nil {
		t.Fatalf("StateFilePath failed: %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file failed: %v", err)
	}
	payload := string(bytes)
	if strings.Contains(payload, rawSecret) {
		t.Fatalf("expected prefixed positional secret to be redacted from events file, got %q", payload)
	}
	lowerPayload := strings.ToLower(payload)
	if !strings.Contains(lowerPayload, "aws_secret_access_key") || !strings.Contains(lowerPayload, "redacted") {
		t.Fatalf("expected prefixed positional redaction marker in persisted event, got %q", payload)
	}
}
