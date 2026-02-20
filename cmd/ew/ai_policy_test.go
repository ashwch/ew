package main

import (
	"testing"

	"github.com/ashwch/ew/internal/config"
	"github.com/ashwch/ew/internal/provider"
	"github.com/ashwch/ew/internal/router"
)

func TestEvaluateAIResolutionAskNeverExecutes(t *testing.T) {
	cfg := config.Default()
	resolution := provider.Resolution{
		Action:     "ask",
		Command:    "ls -la",
		Confidence: 0.99,
		Reason:     "need your review",
	}

	decision := evaluateAIResolution(router.IntentFix, cfg, resolution)
	if decision.Allowed {
		t.Fatalf("expected ask action to be blocked")
	}
}

func TestEvaluateAIResolutionAskWithoutCommandIsHandled(t *testing.T) {
	cfg := config.Default()
	resolution := provider.Resolution{
		Action:     "ask",
		Confidence: 0.99,
		Reason:     "need your review",
	}

	decision := evaluateAIResolution(router.IntentFix, cfg, resolution)
	if decision.Allowed {
		t.Fatalf("expected ask action without command to be blocked")
	}
	if decision.Message == "" {
		t.Fatalf("expected informative message for ask without command")
	}
}

func TestEvaluateAIResolutionSuggestBlockedByDefault(t *testing.T) {
	cfg := config.Default()
	resolution := provider.Resolution{
		Action:     "suggest",
		Command:    "git status",
		Confidence: 0.95,
		Reason:     "safe check",
	}

	decision := evaluateAIResolution(router.IntentRun, cfg, resolution)
	if decision.Allowed {
		t.Fatalf("expected suggest action to be blocked when allow_suggest_execution=false")
	}
}

func TestEvaluateAIResolutionRunBlockedByConfidence(t *testing.T) {
	cfg := config.Default()
	cfg.Fix.MinConfidence = 0.80
	resolution := provider.Resolution{
		Action:     "run",
		Command:    "git status",
		Confidence: 0.50,
		Reason:     "low confidence",
	}

	decision := evaluateAIResolution(router.IntentFix, cfg, resolution)
	if decision.Allowed {
		t.Fatalf("expected run action below confidence threshold to be blocked")
	}
}

func TestEvaluateAIResolutionNeedsConfirmationForcesConfirm(t *testing.T) {
	cfg := config.Default()
	cfg.AI.AllowSuggestExecution = true
	cfg.Find.MinConfidence = 0.20
	resolution := provider.Resolution{
		Action:            "suggest",
		Command:           "```bash\n$ aws sts get-caller-identity\n```",
		Confidence:        0.90,
		Reason:            "validate identity first",
		NeedsConfirmation: true,
	}

	decision := evaluateAIResolution(router.IntentRun, cfg, resolution)
	if !decision.Allowed {
		t.Fatalf("expected decision to be allowed")
	}
	if decision.ModeOverride != "confirm" {
		t.Fatalf("expected mode override confirm, got %q", decision.ModeOverride)
	}
	if decision.Command != "aws sts get-caller-identity" {
		t.Fatalf("expected normalized command, got %q", decision.Command)
	}
}
