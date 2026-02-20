package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ashwch/ew/internal/config"
)

func TestParseResolutionHandlesWrapperResultWithFencedJSON(t *testing.T) {
	wrapper := map[string]any{
		"type":    "result",
		"subtype": "success",
		"result":  "```json\n{\n  \"action\": \"fix\",\n  \"command\": \"aws sso logout\",\n  \"reason\": \"logout command\",\n  \"risk\": \"low\",\n  \"confidence\": 0.95,\n  \"needs_confirmation\": false\n}\n```",
	}
	bytes, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatalf("marshal wrapper failed: %v", err)
	}
	raw := string(bytes)

	parsed, err := parseResolution(raw)
	if err != nil {
		t.Fatalf("parseResolution failed: %v", err)
	}

	normalized := normalizeResolution(parsed)
	if normalized.Action != "run" {
		t.Fatalf("expected normalized action run, got %q", normalized.Action)
	}
	if normalized.Command != "aws sso logout" {
		t.Fatalf("expected aws sso logout command, got %q", normalized.Command)
	}
}

func TestPreprocessStructuredTextStripsCodeFence(t *testing.T) {
	raw := "```json\n{\"action\":\"run\"}\n```"
	got := preprocessStructuredText(raw)
	if got != `{"action":"run"}` {
		t.Fatalf("expected stripped JSON, got %q", got)
	}
}

func TestNormalizeActionSupportsSynonyms(t *testing.T) {
	if got := normalizeAction("fix"); got != "run" {
		t.Fatalf("expected fix->run, got %q", got)
	}
	if got := normalizeAction("recommend"); got != "suggest" {
		t.Fatalf("expected recommend->suggest, got %q", got)
	}
}

func TestAdaptLooseResolutionDefaultsConfidenceByActionWhenMissing(t *testing.T) {
	payload := map[string]any{
		"action":  "fix",
		"command": "aws sso logout",
		"reason":  "logout from aws sso",
		"risk":    "low",
	}
	resolution, ok := adaptLooseResolution(payload)
	if !ok {
		t.Fatalf("expected loose adaptation to succeed")
	}
	if resolution.Confidence < 0.80 {
		t.Fatalf("expected elevated default confidence for structured fix action, got %.2f", resolution.Confidence)
	}
}

func TestResolveFailsWhenProviderExitsNonZeroEvenWithParseableJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is not portable on windows")
	}

	tempDir := t.TempDir()
	scriptPath := filepath.Join(tempDir, "provider.sh")
	script := `#!/bin/sh
echo '{"action":"run","command":"aws sso logout","reason":"ok","risk":"low","confidence":0.99,"needs_confirmation":false}'
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script failed: %v", err)
	}

	adapter, err := NewCommandAdapter("test", config.ProviderConfig{
		Type:    "command",
		Command: scriptPath,
		Model:   "test-model",
		Args:    []string{"{prompt}"},
	})
	if err != nil {
		t.Fatalf("NewCommandAdapter failed: %v", err)
	}

	_, resolveErr := adapter.Resolve(context.Background(), Request{
		Intent:   IntentFind,
		Prompt:   "logout from aws sso",
		Model:    "test-model",
		Thinking: "low",
	})
	if resolveErr == nil {
		t.Fatalf("expected non-zero provider exit to fail even when output contains JSON")
	}
	if !strings.Contains(resolveErr.Error(), "provider command failed") {
		t.Fatalf("expected provider command failure error, got: %v", resolveErr)
	}
}
