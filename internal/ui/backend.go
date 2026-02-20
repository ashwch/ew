package ui

import "strings"

const (
	BackendAuto      = "auto"
	BackendBubbleTea = "bubbletea"
	BackendHuh       = "huh"
	BackendTView     = "tview"
	BackendPlain     = "plain"
)

func NormalizeBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case BackendAuto, "":
		return BackendAuto
	case BackendBubbleTea:
		return BackendBubbleTea
	case BackendHuh:
		return BackendHuh
	case BackendTView:
		return BackendTView
	case BackendPlain:
		return BackendPlain
	default:
		return BackendAuto
	}
}

func IsInteractiveBackend(backend string) bool {
	switch NormalizeBackend(backend) {
	case BackendPlain:
		return false
	default:
		return true
	}
}

func backendCandidates(backend string) []string {
	switch NormalizeBackend(backend) {
	case BackendBubbleTea:
		return []string{BackendBubbleTea, BackendHuh, BackendTView}
	case BackendHuh:
		return []string{BackendHuh, BackendBubbleTea, BackendTView}
	case BackendTView:
		return []string{BackendTView, BackendBubbleTea, BackendHuh}
	case BackendPlain:
		return []string{BackendPlain}
	case BackendAuto, "":
		return []string{BackendBubbleTea, BackendHuh, BackendTView}
	default:
		return []string{BackendBubbleTea, BackendHuh, BackendTView}
	}
}
