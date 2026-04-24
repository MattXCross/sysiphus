package app

import "github.com/charmbracelet/lipgloss"

type styles struct {
	title      lipgloss.Style
	section    lipgloss.Style
	panel      lipgloss.Style
	muted      lipgloss.Style
	user       lipgloss.Style
	assistant  lipgloss.Style
	reasoning  lipgloss.Style
	tool       lipgloss.Style
	permission lipgloss.Style
	subagent   lipgloss.Style
	input      lipgloss.Style
	help       lipgloss.Style
	modal      lipgloss.Style
	error      lipgloss.Style
	warning    lipgloss.Style
}

func defaultStyles() styles {
	border := lipgloss.RoundedBorder()
	return styles{
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		section:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
		panel:      lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color("240")),
		muted:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		user:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111")),
		assistant:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
		reasoning:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("150")),
		tool:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81")),
		permission: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")),
		subagent:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("177")),
		input:      lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color("99")).Padding(0, 1),
		help:       lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color("243")).Padding(1).Foreground(lipgloss.Color("252")),
		modal:      lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color("212")).Background(lipgloss.Color("235")).Padding(1),
		error:      lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		warning:    lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	}
}
