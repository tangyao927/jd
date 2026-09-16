package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"jd/internal/nav"
)

func TestModelSelectsHighlightedCandidate(t *testing.T) {
	model := NewModel([]nav.Match{
		{Candidate: nav.Candidate{Path: "/work/api"}},
		{Candidate: nav.Candidate{Path: "/work/web"}},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, cmd := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if cmd == nil || got.Selected() != "/work/web" || got.Cancelled() {
		t.Fatalf("selection = %q cancelled=%v cmd=%v", got.Selected(), got.Cancelled(), cmd)
	}
}

func TestModelEscapeCancelsWithoutSelection(t *testing.T) {
	model := NewModel([]nav.Match{{Candidate: nav.Candidate{Path: "/work/api"}}})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if cmd == nil || !got.Cancelled() || got.Selected() != "" {
		t.Fatalf("selection = %q cancelled=%v cmd=%v", got.Selected(), got.Cancelled(), cmd)
	}
}
