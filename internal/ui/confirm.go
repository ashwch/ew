package ui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/rivo/tview"
)

func ConfirmExecution(backend string, command string, risk string) (bool, bool, error) {
	var firstErr error
	for _, candidate := range backendCandidates(backend) {
		var (
			approved bool
			err      error
		)
		switch candidate {
		case BackendBubbleTea:
			approved, err = confirmWithBubbleTea(command, risk)
		case BackendHuh:
			approved, err = confirmWithHuh(command, risk)
		case BackendTView:
			approved, err = confirmWithTView(command, risk)
		case BackendPlain:
			continue
		default:
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return approved, true, nil
	}
	if firstErr != nil {
		return false, false, firstErr
	}
	return false, false, nil
}

type bubbleConfirmModel struct {
	command  string
	risk     string
	approved bool
	done     bool
}

func (m bubbleConfirmModel) Init() tea.Cmd { return nil }

func (m bubbleConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch k := msg.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(k.String()) {
		case "y":
			m.approved = true
			m.done = true
			return m, tea.Quit
		case "n", "esc", "ctrl+c", "enter":
			m.approved = false
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m bubbleConfirmModel) View() string {
	return fmt.Sprintf(
		"Run this command?\n\n%s\n\nrisk: %s\n\n[y] run  [n] cancel",
		m.command,
		strings.TrimSpace(m.risk),
	)
}

func confirmWithBubbleTea(command string, risk string) (bool, error) {
	model := bubbleConfirmModel{command: strings.TrimSpace(command), risk: strings.TrimSpace(risk)}
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return false, err
	}
	out, ok := final.(bubbleConfirmModel)
	if !ok {
		return false, nil
	}
	if !out.done {
		return false, nil
	}
	return out.approved, nil
}

func confirmWithHuh(command string, risk string) (bool, error) {
	approved := false
	prompt := huh.NewConfirm().
		Title("Run this command?").
		Description(fmt.Sprintf("%s\nrisk: %s", strings.TrimSpace(command), strings.TrimSpace(risk))).
		Affirmative("Run").
		Negative("Cancel").
		Value(&approved).
		WithTheme(huh.ThemeCharm())
	err := prompt.Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}
		return false, err
	}
	return approved, nil
}

func confirmWithTView(command string, risk string) (bool, error) {
	app := tview.NewApplication()
	approved := false
	done := false

	text := fmt.Sprintf("Run this command?\n\n%s\n\nrisk: %s", strings.TrimSpace(command), strings.TrimSpace(risk))
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Run", "Cancel"}).
		SetDoneFunc(func(_ int, label string) {
			done = true
			approved = strings.EqualFold(strings.TrimSpace(label), "run")
			app.Stop()
		})

	if err := app.SetRoot(modal, true).Run(); err != nil {
		return false, err
	}
	if !done {
		return false, nil
	}
	return approved, nil
}
