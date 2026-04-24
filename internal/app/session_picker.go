package app

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/matt/sysiphus/internal/sessionstore"
	"github.com/matt/sysiphus/internal/state"
)

type sessionItem struct {
	summary state.SessionSummary
}

func (s sessionItem) FilterValue() string {
	return s.summary.ID + " " + s.summary.Title + " " + s.summary.Model + " " + s.summary.Mode
}
func (s sessionItem) Title() string { return s.summary.Title }
func (s sessionItem) Description() string {
	return fmt.Sprintf("%s  %s  %s", s.summary.UpdatedAt.Format("2006-01-02 15:04"), s.summary.Model, s.summary.Mode)
}

type sessionLoadedMsg struct {
	meta   sessionstore.SessionMeta
	events []sessionstore.EventRecord
	err    error
}

func newSessionList(width, height int, items []state.SessionSummary) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(1)
	listItems := make([]list.Item, 0, len(items))
	for _, item := range items {
		listItems = append(listItems, sessionItem{summary: item})
	}
	l := list.New(listItems, delegate, width, height)
	l.Title = "Sessions"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	return l
}
