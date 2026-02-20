package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ashwch/ew/internal/config"
	"github.com/ashwch/ew/internal/router"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe create failed: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("stdout close failed: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("stdout read failed: %v", err)
	}
	return string(out)
}

func TestExecuteSuggestedJSONConfirmDoesNotPrompt(t *testing.T) {
	cfg := config.Default()
	opts := options{
		JSON: true,
	}

	var outcome executionOutcome
	out := captureStdout(t, func() {
		outcome = executeSuggested("echo hi", "test reason", "low", cfg, opts, router.IntentRun)
	})

	if strings.Contains(out, "Run this command? [y/N]:") {
		t.Fatalf("expected json mode to not print interactive prompt, got %q", out)
	}

	var payload response
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("expected valid json output, got error %v with payload %q", err, out)
	}
	if payload.Executed {
		t.Fatalf("expected executed=false in json confirm mode without --yes")
	}
	if !strings.Contains(strings.ToLower(payload.Message), "confirmation required") {
		t.Fatalf("expected confirmation-required message, got %q", payload.Message)
	}
	if outcome.Executed || outcome.Success {
		t.Fatalf("expected no execution outcome, got %+v", outcome)
	}
}
