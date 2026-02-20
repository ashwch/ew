package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type SystemProfileDecision struct {
	DisableContext bool
	SetUserNote    bool
	UserNote       string
}

type onboardingMode int

const (
	onboardingModeMenu onboardingMode = iota
	onboardingModeEditNote
)

type systemProfileOnboardingModel struct {
	summaryLines []string
	noteInput    textinput.Model
	mode         onboardingMode
	decision     SystemProfileDecision
	done         bool
	frameIndex   int
	pulseIndex   int
	messageIndex int
}

type onboardingTickMsg struct{}

func SystemProfileOnboarding(backend string, summary string, currentNote string) (SystemProfileDecision, bool, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return SystemProfileDecision{}, false, nil
	}

	var firstErr error
	for _, candidate := range backendCandidates(backend) {
		if candidate != BackendBubbleTea {
			continue
		}
		decision, err := systemProfileOnboardingWithBubbleTea(summary, currentNote)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return decision, true, nil
	}
	if firstErr != nil {
		return SystemProfileDecision{}, false, firstErr
	}
	return SystemProfileDecision{}, false, nil
}

func systemProfileOnboardingWithBubbleTea(summary string, currentNote string) (SystemProfileDecision, error) {
	model := newSystemProfileOnboardingModel(summary, currentNote)
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return SystemProfileDecision{}, err
	}
	out, ok := final.(systemProfileOnboardingModel)
	if !ok {
		return SystemProfileDecision{}, nil
	}
	return out.decision, nil
}

func newSystemProfileOnboardingModel(summary string, currentNote string) systemProfileOnboardingModel {
	noteInput := textinput.New()
	noteInput.Placeholder = "optional correction note"
	noteInput.CharLimit = 240
	noteInput.Width = 72
	noteInput.SetValue(strings.TrimSpace(currentNote))

	return systemProfileOnboardingModel{
		summaryLines: summarizeOnboardingLines(summary, 14),
		noteInput:    noteInput,
		mode:         onboardingModeMenu,
	}
}

func (m systemProfileOnboardingModel) Init() tea.Cmd {
	return onboardingTickCmd()
}

func (m systemProfileOnboardingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch k := msg.(type) {
	case onboardingTickMsg:
		if m.done {
			return m, nil
		}
		m.frameIndex = (m.frameIndex + 1) % len(onboardingMarkFrames)
		m.pulseIndex = (m.pulseIndex + 1) % len(onboardingPulseDots)
		if m.frameIndex == 0 {
			m.messageIndex = (m.messageIndex + 1) % len(onboardingPulseMessages)
		}
		return m, onboardingTickCmd()
	case tea.KeyMsg:
		if m.mode == onboardingModeEditNote {
			return m.updateEditMode(k)
		}
		return m.updateMenuMode(k)
	}
	if m.mode == onboardingModeEditNote {
		var cmd tea.Cmd
		m.noteInput, cmd = m.noteInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m systemProfileOnboardingModel) updateMenuMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "enter", "y":
		m.done = true
		return m, tea.Quit
	case "d", "n":
		m.done = true
		m.decision.DisableContext = true
		return m, tea.Quit
	case "e":
		m.mode = onboardingModeEditNote
		m.noteInput.Focus()
		return m, textinput.Blink
	case "esc", "q", "ctrl+c":
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m systemProfileOnboardingModel) updateEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "enter":
		m.done = true
		m.decision.SetUserNote = true
		m.decision.UserNote = strings.TrimSpace(m.noteInput.Value())
		return m, tea.Quit
	case "esc":
		m.mode = onboardingModeMenu
		m.noteInput.Blur()
		return m, nil
	case "ctrl+c":
		m.done = true
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.noteInput, cmd = m.noteInput.Update(msg)
	return m, cmd
}

func (m systemProfileOnboardingModel) View() string {
	if m.mode == onboardingModeEditNote {
		return m.editView()
	}
	return m.menuView()
}

func (m systemProfileOnboardingModel) menuView() string {
	header := onboardingTitleStyle.Render("ew onboarding")
	lines := []string{
		header,
		"",
		onboardingAnimatedStatusLine(m.frameIndex, m.messageIndex, m.pulseIndex),
		"",
		onboardingSectionStyle.Render("learned local machine context"),
	}
	for _, summaryLine := range m.summaryLines {
		lines = append(lines, onboardingSummaryStyle.Render(summaryLine))
	}
	lines = append(lines, "")
	lines = append(lines, onboardingHintStyle.Render("[enter] keep context and continue"))
	lines = append(lines, onboardingHintStyle.Render("[d] disable machine context"))
	lines = append(lines, onboardingHintStyle.Render("[e] edit correction note"))
	lines = append(lines, onboardingHintStyle.Render("[esc] continue without changes"))
	return onboardingCardStyle.Render(strings.Join(lines, "\n"))
}

func (m systemProfileOnboardingModel) editView() string {
	lines := []string{
		onboardingTitleStyle.Render("ew onboarding: correction note"),
		"",
		onboardingAnimatedStatusLine(m.frameIndex, m.messageIndex, m.pulseIndex),
		"",
		onboardingBodyStyle.Render("Add a short machine-specific note for better future suggestions."),
		"",
		m.noteInput.View(),
		"",
		onboardingHintStyle.Render("[enter] save note  [esc] back"),
	}
	return onboardingCardStyle.Render(strings.Join(lines, "\n"))
}

func summarizeOnboardingLines(summary string, maxLines int) []string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	if maxLines <= 0 {
		maxLines = 14
	}
	raw := strings.Split(summary, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) <= maxLines {
		return lines
	}
	remaining := len(lines) - maxLines
	out := append([]string{}, lines[:maxLines]...)
	out = append(out, fmt.Sprintf("- +%d more", remaining))
	return out
}

func onboardingTickCmd() tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg {
		return onboardingTickMsg{}
	})
}

func onboardingAnimatedStatusLine(frameIndex int, messageIndex int, pulseIndex int) string {
	frame := onboardingMarkFrames[frameIndex%len(onboardingMarkFrames)]
	message := onboardingPulseMessages[messageIndex%len(onboardingPulseMessages)]
	dots := onboardingPulseDots[pulseIndex%len(onboardingPulseDots)]
	return onboardingSubtleStyle.Render(
		fmt.Sprintf("%s %s%s", onboardingMarkStyle.Render(frame), message, dots),
	)
}

var (
	onboardingMarkFrames = []string{
		"ew",
		"we",
		"EW",
		"WE",
	}

	onboardingPulseMessages = []string{
		"mapping your shell habits",
		"lining up your command context",
		"calibrating local command hints",
		"tuning ew to your machine",
	}

	onboardingPulseDots = []string{
		".",
		"..",
		"...",
	}

	onboardingCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Padding(1, 2)

	onboardingTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("87"))

	onboardingSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("153"))

	onboardingMarkStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("45"))

	onboardingSubtleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("248"))

	onboardingSummaryStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	onboardingBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	onboardingHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("109"))
)
