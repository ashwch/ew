package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSystemProfileOnboardingModelKeep(t *testing.T) {
	model := newSystemProfileOnboardingModel("- os=darwin", "")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	out, ok := updated.(systemProfileOnboardingModel)
	if !ok {
		t.Fatalf("expected systemProfileOnboardingModel")
	}
	if !out.done {
		t.Fatalf("expected done=true")
	}
	if out.decision.DisableContext {
		t.Fatalf("did not expect disable context")
	}
	if out.decision.SetUserNote {
		t.Fatalf("did not expect set user note")
	}
}

func TestSystemProfileOnboardingModelDisable(t *testing.T) {
	model := newSystemProfileOnboardingModel("- os=darwin", "")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	out, ok := updated.(systemProfileOnboardingModel)
	if !ok {
		t.Fatalf("expected systemProfileOnboardingModel")
	}
	if !out.done {
		t.Fatalf("expected done=true")
	}
	if !out.decision.DisableContext {
		t.Fatalf("expected disable context")
	}
}

func TestSystemProfileOnboardingModelEditNote(t *testing.T) {
	model := newSystemProfileOnboardingModel("- os=darwin", "")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	out, ok := updated.(systemProfileOnboardingModel)
	if !ok {
		t.Fatalf("expected systemProfileOnboardingModel")
	}
	if out.mode != onboardingModeEditNote {
		t.Fatalf("expected edit mode")
	}

	out.noteInput.SetValue("prefer zsh aliases")
	updated, _ = out.Update(tea.KeyMsg{Type: tea.KeyEnter})
	final, ok := updated.(systemProfileOnboardingModel)
	if !ok {
		t.Fatalf("expected systemProfileOnboardingModel")
	}
	if !final.done {
		t.Fatalf("expected done=true")
	}
	if !final.decision.SetUserNote {
		t.Fatalf("expected SetUserNote=true")
	}
	if final.decision.UserNote != "prefer zsh aliases" {
		t.Fatalf("unexpected user note: %q", final.decision.UserNote)
	}
}

func TestSummarizeOnboardingLinesLimit(t *testing.T) {
	lines := summarizeOnboardingLines(strings.Join([]string{
		"- a", "- b", "- c", "- d",
	}, "\n"), 2)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[2] != "- +2 more" {
		t.Fatalf("unexpected overflow line: %q", lines[2])
	}
}
