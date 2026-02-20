package main

import (
	"fmt"
	"strings"

	"github.com/ashwch/ew/internal/config"
	"github.com/ashwch/ew/internal/provider"
	"github.com/ashwch/ew/internal/router"
	ewrt "github.com/ashwch/ew/internal/runtime"
)

type aiExecutionDecision struct {
	Allowed      bool
	Command      string
	Reason       string
	RiskHint     string
	ModeOverride string
	Message      string
}

func evaluateAIResolution(intent router.Intent, cfg config.Config, resolution provider.Resolution) aiExecutionDecision {
	action := strings.ToLower(strings.TrimSpace(resolution.Action))
	switch action {
	case "run", "suggest", "ask":
	default:
		action = "ask"
	}
	if action == "" {
		action = "ask"
	}

	command := strings.TrimSpace(resolution.Command)
	if action == "ask" {
		if command == "" {
			return aiExecutionDecision{
				Allowed: false,
				Message: "provider requested confirmation/manual action and did not provide a runnable command",
			}
		}
		normalized, err := ewrt.NormalizeCommand(command)
		if err != nil {
			return aiExecutionDecision{
				Allowed: false,
				Message: fmt.Sprintf("provider returned invalid command: %v", err),
			}
		}
		return aiExecutionDecision{
			Allowed: false,
			Command: normalized,
			Message: "provider requested confirmation instead of an auto-runnable action",
		}
	}

	if command == "" {
		return aiExecutionDecision{
			Allowed: false,
			Message: "provider did not return a runnable command",
		}
	}

	normalized, err := ewrt.NormalizeCommand(command)
	if err != nil {
		return aiExecutionDecision{
			Allowed: false,
			Message: fmt.Sprintf("provider returned invalid command: %v", err),
		}
	}

	confidence := clampConfidence(resolution.Confidence)
	minConfidence := confidenceThresholdForIntent(cfg, intent)

	if confidence < minConfidence {
		return aiExecutionDecision{
			Allowed: false,
			Command: normalized,
			Message: fmt.Sprintf("provider confidence %.2f is below threshold %.2f", confidence, minConfidence),
		}
	}

	if action == "suggest" && !cfg.AI.AllowSuggestExecution {
		return aiExecutionDecision{
			Allowed: false,
			Command: normalized,
			Message: "provider returned suggest action and policy blocks suggest execution",
		}
	}

	reason := strings.TrimSpace(resolution.Reason)
	if reason == "" {
		reason = "provider suggestion"
	}

	decision := aiExecutionDecision{
		Allowed:  true,
		Command:  normalized,
		Reason:   reason,
		RiskHint: normalizeRiskHint(resolution.Risk),
	}
	if resolution.NeedsConfirmation {
		decision.ModeOverride = "confirm"
	}
	return decision
}

func confidenceThresholdForIntent(cfg config.Config, intent router.Intent) float64 {
	switch intent {
	case router.IntentFix:
		if cfg.Fix.MinConfidence > 0 && cfg.Fix.MinConfidence <= 1 {
			return cfg.Fix.MinConfidence
		}
	case router.IntentFind, router.IntentRun:
		if cfg.Find.MinConfidence > 0 && cfg.Find.MinConfidence <= 1 {
			return cfg.Find.MinConfidence
		}
	}
	if cfg.AI.MinConfidence > 0 && cfg.AI.MinConfidence <= 1 {
		return cfg.AI.MinConfidence
	}
	return 0.60
}

func clampConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func normalizeRiskHint(risk string) string {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}
