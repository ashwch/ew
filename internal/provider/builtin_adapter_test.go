package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/ashwch/ew/internal/config"
)

func TestBuiltinAdapterFindGitPushCurrentBranch(t *testing.T) {
	adapter, err := NewBuiltinAdapter("ew", config.ProviderConfig{})
	if err != nil {
		t.Fatalf("NewBuiltinAdapter failed: %v", err)
	}

	resolution, err := adapter.Resolve(context.Background(), Request{
		Intent: IntentFind,
		Prompt: `Return only JSON matching schema. Find the best shell command for this request: "how to git push current branch".`,
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolution.Action != "run" {
		t.Fatalf("expected run action, got %q", resolution.Action)
	}
	if !strings.Contains(resolution.Command, "git push -u origin") {
		t.Fatalf("expected git push command, got %q", resolution.Command)
	}
	if !resolution.NeedsConfirmation {
		t.Fatalf("expected builtin run to require confirmation")
	}
}

func TestBuiltinAdapterFindPushToGitHubAlias(t *testing.T) {
	adapter, err := NewBuiltinAdapter("ew", config.ProviderConfig{})
	if err != nil {
		t.Fatalf("NewBuiltinAdapter failed: %v", err)
	}

	resolution, err := adapter.Resolve(context.Background(), Request{
		Intent: IntentFind,
		Prompt: `Return only JSON matching schema. Find the best shell command for this request: "push to gh".`,
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !strings.Contains(resolution.Command, `git push -u origin`) {
		t.Fatalf("expected git push command, got %q", resolution.Command)
	}
}

func TestBuiltinAdapterFixUsesDeterministicSuggestion(t *testing.T) {
	adapter, err := NewBuiltinAdapter("ew", config.ProviderConfig{})
	if err != nil {
		t.Fatalf("NewBuiltinAdapter failed: %v", err)
	}

	resolution, err := adapter.Resolve(context.Background(), Request{
		Intent: IntentFix,
		Prompt: `Return only JSON matching schema. Failed command: "gti status".`,
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if resolution.Command != "git status" {
		t.Fatalf("expected git status fix, got %q", resolution.Command)
	}
}

func TestServiceResolveFallsBackToBuiltinProvider(t *testing.T) {
	registry := NewRegistry()
	service := NewService(registry)
	cfg := config.Default()
	cfg.Provider = "auto"
	enabled := true
	cfg.Providers = map[string]config.ProviderConfig{
		"codex": {
			Type:     "command",
			Command:  "missing-codex-binary",
			Enabled:  &enabled,
			Model:    "gpt-5-codex",
			Thinking: "medium",
		},
		"claude": {
			Type:     "command",
			Command:  "missing-claude-binary",
			Enabled:  &enabled,
			Model:    "sonnet",
			Thinking: "medium",
		},
		"ew": {
			Type:     "builtin",
			Command:  "ew",
			Enabled:  &enabled,
			Model:    "ew-core",
			Thinking: "minimal",
			Models: map[string]config.ModelConfig{
				"ew-core": {ProviderModel: "ew-core", Speed: "fast"},
			},
		},
	}

	resolution, providerName, err := service.Resolve(context.Background(), cfg, Request{
		Intent: IntentFind,
		Prompt: `Find the best shell command for this request: "how to git push current branch".`,
		Mode:   "confirm",
	}, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if providerName != "ew" {
		t.Fatalf("expected fallback provider ew, got %q", providerName)
	}
	if !strings.Contains(resolution.Command, "git push -u origin") {
		t.Fatalf("expected builtin git push command, got %q", resolution.Command)
	}
}
