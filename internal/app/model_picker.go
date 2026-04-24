package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matt/sysiphus/internal/state"
)

type modelItem struct {
	option state.ModelOption
}

func (m modelItem) FilterValue() string { return m.option.ID + " " + m.option.Name }
func (m modelItem) Title() string       { return m.option.ID }

func (m modelItem) Description() string {
	parts := []string{m.option.Name}
	if m.option.SupportsReasoning {
		if m.option.DefaultReasoning != "" {
			parts = append(parts, fmt.Sprintf("reasoning %s", m.option.DefaultReasoning))
		} else {
			parts = append(parts, "reasoning supported")
		}
	}
	return strings.Join(parts, "  ")
}

type modelsLoadedMsg struct {
	items []state.ModelOption
	err   error
}

type modelSwitchedMsg struct {
	modelID string
	err     error
}

func newModelList(width, height int, items []state.ModelOption) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(1)

	listItems := make([]list.Item, 0, len(items))
	for _, item := range items {
		listItems = append(listItems, modelItem{option: item})
	}

	l := list.New(listItems, delegate, width, height)
	l.Title = "Select Model"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	return l
}

func (m *model) refreshModelPicker() {
	width := min(72, max(36, m.width-8))
	height := min(18, max(8, m.height-8))
	m.modelPicker = newModelList(width, height, m.availableModels)
	for i, item := range m.availableModels {
		if item.ID == m.modelName {
			m.modelPicker.Select(i)
			break
		}
	}
}

func (m *model) updateModelPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.showModelPicker = false
			return m, nil
		case "enter":
			selected := m.modelPicker.SelectedItem()
			item, ok := selected.(modelItem)
			if !ok {
				m.showModelPicker = false
				return m, nil
			}
			m.showModelPicker = false
			m.appendActivity("model", fmt.Sprintf("switching model to %s", item.option.ID), "", "running")
			m.refreshPaneContent(true)
			return m, m.switchModelCmd(item.option.ID)
		}
	}
	var cmd tea.Cmd
	m.modelPicker, cmd = m.modelPicker.Update(msg)
	return m, cmd
}
