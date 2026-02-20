package runtime

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestShouldExecuteConfirmRequiresInteractiveTerminal(t *testing.T) {
	previous := stdinIsInteractive
	stdinIsInteractive = func() bool { return false }
	t.Cleanup(func() {
		stdinIsInteractive = previous
	})

	_, err := ShouldExecute("confirm", false)
	if err == nil {
		t.Fatalf("expected non-interactive confirm to return error")
	}
}

func TestShouldExecuteConfirmYesBypassesPrompt(t *testing.T) {
	previous := stdinIsInteractive
	stdinIsInteractive = func() bool { return false }
	t.Cleanup(func() {
		stdinIsInteractive = previous
	})

	shouldRun, err := ShouldExecute("confirm", true)
	if err != nil {
		t.Fatalf("expected no error when --yes is provided: %v", err)
	}
	if !shouldRun {
		t.Fatalf("expected confirm mode with --yes to execute")
	}
}

func TestNormalizeCommandStripsFenceAndPromptPrefix(t *testing.T) {
	input := "```bash\n$ git status\n```"
	got, err := NormalizeCommand(input)
	if err != nil {
		t.Fatalf("NormalizeCommand returned error: %v", err)
	}
	if got != "git status" {
		t.Fatalf("expected git status, got %q", got)
	}
}

func TestNormalizeCommandRejectsEmpty(t *testing.T) {
	if _, err := NormalizeCommand("   "); err == nil {
		t.Fatalf("expected error for empty command")
	}
}

func TestNormalizeCommandRejectsNullByte(t *testing.T) {
	if _, err := NormalizeCommand("echo hi\x00"); err == nil {
		t.Fatalf("expected error for null byte command")
	}
}

func TestShellCommandInvocationUsesShellEnvWhenValid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell selection test is unix-specific")
	}

	t.Setenv("SHELL", "/bin/sh")
	shell, args := shellCommandInvocation("echo hi")
	if shell != "/bin/sh" {
		t.Fatalf("expected /bin/sh from SHELL, got %q", shell)
	}
	if len(args) != 2 || args[0] != "-lc" {
		t.Fatalf("expected -lc invocation args, got %#v", args)
	}
}

func TestShellCommandInvocationFallsBackWhenShellEnvInvalid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell selection test is unix-specific")
	}

	t.Setenv("SHELL", filepath.Join(t.TempDir(), "missing-shell"))
	shell, _ := shellCommandInvocation("echo hi")
	if shell != "sh" {
		t.Fatalf("expected fallback shell sh, got %q", shell)
	}
}
