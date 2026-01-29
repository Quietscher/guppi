package main

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	statusCleanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusDirtyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	favoriteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	branchStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	successStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	pullResultStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	detailTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1)
	detailBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
)
