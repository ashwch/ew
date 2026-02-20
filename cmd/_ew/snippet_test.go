package main

import (
	"strings"
	"testing"
)

func TestBashSnippetUsesHistoryInsteadOfDebugTrap(t *testing.T) {
	snippet := bashSnippet()
	if strings.Contains(snippet, "trap '_EW_LAST_COMMAND=$BASH_COMMAND' DEBUG") {
		t.Fatalf("bash snippet should not use DEBUG trap capture")
	}
	if !strings.Contains(snippet, "fc -ln -1") {
		t.Fatalf("bash snippet should derive last command from fc builtin")
	}
	if !strings.Contains(snippet, "2>/dev/null") {
		t.Fatalf("bash snippet should suppress fc stderr noise when history is unavailable")
	}
	if !strings.Contains(snippet, "_EW_LAST_HISTCMD") {
		t.Fatalf("bash snippet should guard duplicate prompt records using HISTCMD")
	}
	if !strings.Contains(snippet, `_EW_LAST_HISTCMD="$HISTCMD"`) {
		t.Fatalf("bash snippet should initialize HISTCMD guard to avoid stale startup capture")
	}
	if !strings.Contains(snippet, `case ";$PROMPT_COMMAND;" in`) {
		t.Fatalf("bash snippet should avoid duplicate PROMPT_COMMAND injection")
	}
}

func TestHookSnippetsSetStableSessionID(t *testing.T) {
	zsh := zshSnippet()
	if !strings.Contains(zsh, `EW_SESSION_ID=${EW_SESSION_ID:-"$$.$(date +%s)"}`) {
		t.Fatalf("zsh snippet should set deterministic session id")
	}
	if !strings.Contains(zsh, `EW_LAST_COMMAND=""`) {
		t.Fatalf("zsh snippet should clear last command after recording")
	}

	bash := bashSnippet()
	if !strings.Contains(bash, `EW_SESSION_ID=${EW_SESSION_ID:-"$$.$(date +%s)"}`) {
		t.Fatalf("bash snippet should set deterministic session id")
	}

	fish := fishSnippet()
	if !strings.Contains(fish, `set -q EW_SESSION_ID; or set -gx EW_SESSION_ID "$fish_pid".(date +%s)`) {
		t.Fatalf("fish snippet should set deterministic session id")
	}
	if !strings.Contains(fish, `set -e EW_LAST_COMMAND`) {
		t.Fatalf("fish snippet should clear last command after recording")
	}
}
