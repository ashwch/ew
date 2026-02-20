package provider

import (
	"testing"

	"github.com/ashwch/ew/internal/config"
)

func TestResolveModelAutoFastUsesFastAlias(t *testing.T) {
	cfg := config.ProviderConfig{
		Model: "gpt-5-codex",
		Models: map[string]config.ModelConfig{
			"gpt-5-codex": {ProviderModel: "gpt-5-codex", Speed: "quality"},
			"gpt-5-mini":  {ProviderModel: "gpt-5-mini", Speed: "fast"},
		},
	}

	got := resolveModel(cfg, "auto-fast")
	if got != "gpt-5-mini" {
		t.Fatalf("expected fast model provider id gpt-5-mini, got %q", got)
	}
}

func TestResolveModelAutoMainUsesQualityAlias(t *testing.T) {
	cfg := config.ProviderConfig{
		Model: "sonnet",
		Models: map[string]config.ModelConfig{
			"sonnet": {ProviderModel: "claude-sonnet", Speed: "quality"},
			"haiku":  {ProviderModel: "claude-haiku", Speed: "fast"},
		},
	}

	got := resolveModel(cfg, "auto-main")
	if got != "claude-sonnet" {
		t.Fatalf("expected quality model provider id claude-sonnet, got %q", got)
	}
}

func TestResolveModelUnknownAutoFallsBackToProviderDefault(t *testing.T) {
	cfg := config.ProviderConfig{
		Model: "default-alias",
		Models: map[string]config.ModelConfig{
			"default-alias": {ProviderModel: "provider-default"},
		},
	}

	got := resolveModel(cfg, "auto-foo")
	if got != "provider-default" {
		t.Fatalf("expected fallback provider-default, got %q", got)
	}
}

func TestResolveModelUnknownExplicitModelFallsBackToProviderDefault(t *testing.T) {
	cfg := config.ProviderConfig{
		Model: "sonnet",
		Models: map[string]config.ModelConfig{
			"sonnet": {ProviderModel: "sonnet"},
			"haiku":  {ProviderModel: "haiku"},
		},
	}

	got := resolveModel(cfg, "gpt-5-codex")
	if got != "sonnet" {
		t.Fatalf("expected fallback provider default sonnet, got %q", got)
	}
}

func TestResolveModelInvalidProviderDefaultFallsBackToKnownAlias(t *testing.T) {
	cfg := config.ProviderConfig{
		Model: "haiku",
		Models: map[string]config.ModelConfig{
			"gpt-5-codex": {ProviderModel: "gpt-5-codex", Speed: "quality"},
			"gpt-5-mini":  {ProviderModel: "gpt-5-mini", Speed: "fast"},
		},
	}

	got := resolveModel(cfg, "haiku")
	if got != "gpt-5-codex" {
		t.Fatalf("expected fallback to known quality alias provider id, got %q", got)
	}
}
