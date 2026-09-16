package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"jd/internal/nav"
)

var ErrCancelled = errors.New("selection cancelled")

type Model struct {
	all       []nav.Match
	visible   []nav.Match
	cursor    int
	filter    string
	selected  string
	cancelled bool
}

func NewModel(matches []nav.Match) Model {
	all := append([]nav.Match(nil), matches...)
	return Model{all: all, visible: append([]nav.Match(nil), all...)}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.cancelled = true
		m.selected = ""
		return m, tea.Quit
	case tea.KeyEnter:
		if len(m.visible) > 0 {
			m.selected = m.visible[m.cursor].Path
		}
		return m, tea.Quit
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case tea.KeyDown:
		if m.cursor+1 < len(m.visible) {
			m.cursor++
		}
	case tea.KeyBackspace, tea.KeyDelete:
		if len(m.filter) > 0 {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.applyFilter()
		}
	case tea.KeyRunes:
		m.filter += string(key.Runes)
		m.applyFilter()
	}
	return m, nil
}

func (m Model) View() string {
	var builder strings.Builder
	builder.WriteString("Select directory")
	if m.filter != "" {
		fmt.Fprintf(&builder, "  filter: %s", m.filter)
	}
	builder.WriteString("\n")
	for i, match := range m.visible {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		label := match.Path
		if match.Pin != "" {
			label = match.Pin + "  " + label
		}
		fmt.Fprintf(&builder, "%s%s\n", marker, label)
	}
	if len(m.visible) == 0 {
		builder.WriteString("  no matches\n")
	}
	builder.WriteString("enter select • esc cancel • type to filter\n")
	return builder.String()
}

func (m Model) Selected() string { return m.selected }

func (m Model) Cancelled() bool { return m.cancelled }

func (m *Model) applyFilter() {
	m.visible = m.visible[:0]
	needle := strings.ToLower(m.filter)
	for _, match := range m.all {
		if strings.Contains(strings.ToLower(match.Path), needle) || strings.Contains(strings.ToLower(match.Pin), needle) {
			m.visible = append(m.visible, match)
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

func Select(matches []nav.Match, input io.Reader, output io.Writer) (string, error) {
	program := tea.NewProgram(NewModel(matches), tea.WithInput(input), tea.WithOutput(output))
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	model, ok := result.(Model)
	if !ok {
		return "", errors.New("unexpected selector result")
	}
	if model.Cancelled() || model.Selected() == "" {
		return "", ErrCancelled
	}
	return model.Selected(), nil
}
