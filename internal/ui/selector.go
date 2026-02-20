package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ashwch/ew/internal/history"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/rivo/tview"
)

type Selection struct {
	Command string
	Reason  string
	Source  string
}

type selectorOption struct {
	Label     string
	Selection Selection
}

func SelectSuggestedCommand(backend string, query string, suggested Selection, matches []history.Match) (Selection, bool, error) {
	options := buildSelectionOptions(suggested, matches)
	if len(options) < 2 {
		return Selection{}, false, nil
	}

	var firstErr error
	for _, candidate := range backendCandidates(backend) {
		var (
			selected Selection
			used     bool
			err      error
		)
		switch candidate {
		case BackendBubbleTea:
			selected, used, err = selectWithBubbleTea(query, options)
		case BackendHuh:
			selected, used, err = selectWithHuh(query, options)
		case BackendTView:
			selected, used, err = selectWithTView(query, options)
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
		if used {
			return selected, true, nil
		}
	}
	if firstErr != nil {
		return Selection{}, false, firstErr
	}
	return Selection{}, false, nil
}

func buildSelectionOptions(suggested Selection, matches []history.Match) []selectorOption {
	options := make([]selectorOption, 0, len(matches)+1)
	seen := map[string]struct{}{}

	add := func(sel Selection, labelPrefix string) {
		command := strings.TrimSpace(sel.Command)
		if command == "" {
			return
		}
		key := strings.ToLower(command)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		options = append(options, selectorOption{
			Label:     labelPrefix + command,
			Selection: sel,
		})
	}

	if strings.TrimSpace(suggested.Command) != "" {
		add(suggested, "[recommended] ")
	}

	for _, match := range matches {
		add(Selection{
			Command: match.Command,
			Reason:  fmt.Sprintf("history match score %.2f", match.Score),
			Source:  match.Source,
		}, "[history] ")
	}

	return options
}

func selectWithHuh(query string, options []selectorOption) (Selection, bool, error) {
	huhOptions := make([]huh.Option[string], 0, len(options))
	lookup := map[string]Selection{}
	for _, option := range options {
		command := strings.TrimSpace(option.Selection.Command)
		huhOptions = append(huhOptions, huh.NewOption(option.Label, command))
		lookup[strings.ToLower(command)] = option.Selection
	}

	initial := huhOptions[0].Value
	choice := initial

	prompt := huh.NewSelect[string]().
		Title("ew command picker").
		Description(fmt.Sprintf("Choose command for: %q", strings.TrimSpace(query))).
		Options(huhOptions...).
		Filtering(true).
		Height(huhSelectHeight(len(huhOptions))).
		Value(&choice).
		WithTheme(huh.ThemeCharm())

	err := prompt.Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return Selection{}, true, nil
		}
		return Selection{}, false, err
	}
	selected, ok := lookup[strings.ToLower(strings.TrimSpace(choice))]
	if !ok {
		return Selection{}, true, nil
	}
	return selected, true, nil
}

type bubbleSelectorItem struct {
	label   string
	command string
}

func (i bubbleSelectorItem) Title() string       { return i.label }
func (i bubbleSelectorItem) Description() string { return "" }
func (i bubbleSelectorItem) FilterValue() string { return i.label + " " + i.command }

type bubbleSelectorModel struct {
	list      list.Model
	selection string
	cancelled bool
	options   int
}

func (m bubbleSelectorModel) Init() tea.Cmd { return nil }

func (m bubbleSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch k := msg.(type) {
	case tea.WindowSizeMsg:
		width, height := bubblePickerSize(k.Width, k.Height, m.options)
		m.list.SetSize(width, height)
		return m, nil
	case tea.KeyMsg:
		switch k.String() {
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(bubbleSelectorItem); ok {
				m.selection = item.command
			}
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m bubbleSelectorModel) View() string {
	return m.list.View()
}

func selectWithBubbleTea(query string, options []selectorOption) (Selection, bool, error) {
	items := make([]list.Item, 0, len(options))
	lookup := map[string]Selection{}
	for _, option := range options {
		command := strings.TrimSpace(option.Selection.Command)
		lookup[strings.ToLower(command)] = option.Selection
		items = append(items, bubbleSelectorItem{
			label:   option.Label,
			command: command,
		})
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)

	initialWidth, initialHeight := bubblePickerSize(80, 24, len(items))
	picker := list.New(items, delegate, initialWidth, initialHeight)
	picker.Title = fmt.Sprintf("ew command picker: %s", strings.TrimSpace(query))
	picker.SetShowHelp(false)
	picker.SetFilteringEnabled(true)

	model := bubbleSelectorModel{list: picker, options: len(items)}
	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return Selection{}, false, err
	}
	out, ok := final.(bubbleSelectorModel)
	if !ok {
		return Selection{}, true, nil
	}
	if out.cancelled {
		return Selection{}, true, nil
	}

	selection := strings.ToLower(strings.TrimSpace(out.selection))
	if selection == "" {
		return Selection{}, true, nil
	}
	selected, ok := lookup[selection]
	if !ok {
		return Selection{}, true, nil
	}
	return selected, true, nil
}

func selectWithTView(query string, options []selectorOption) (Selection, bool, error) {
	app := tview.NewApplication()
	listView := tview.NewList()
	listView.SetBorder(true)
	listView.SetTitle(fmt.Sprintf("ew command picker: %s", strings.TrimSpace(query)))
	listView.ShowSecondaryText(false)

	selected := Selection{}
	used := false
	for _, option := range options {
		current := option
		listView.AddItem(current.Label, "", 0, func() {
			selected = current.Selection
			used = true
			app.Stop()
		})
	}
	listView.SetDoneFunc(func() {
		app.Stop()
	})

	if err := app.SetRoot(listView, true).SetFocus(listView).Run(); err != nil {
		return Selection{}, false, err
	}
	if !used {
		return Selection{}, true, nil
	}
	return selected, true, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func bubblePickerSize(termWidth, termHeight, optionCount int) (int, int) {
	if termWidth <= 0 {
		termWidth = 80
	}
	if termHeight <= 0 {
		termHeight = 24
	}
	if optionCount < 1 {
		optionCount = 1
	}

	maxWidth := termWidth
	minWidth := 32
	if maxWidth < minWidth {
		minWidth = maxWidth
	}
	width := clampInt(termWidth-4, minWidth, maxWidth)

	visibleItems := clampInt(optionCount, 3, 12)
	desiredHeight := visibleItems + 6

	maxHeight := termHeight - 2
	if maxHeight <= 0 {
		maxHeight = termHeight
	}
	if maxHeight <= 0 {
		maxHeight = 1
	}
	minHeight := 8
	if maxHeight < minHeight {
		minHeight = maxHeight
	}
	height := clampInt(desiredHeight, minHeight, maxHeight)
	return width, height
}

func huhSelectHeight(optionCount int) int {
	if optionCount < 1 {
		optionCount = 1
	}
	return clampInt(optionCount+1, 4, 10)
}
