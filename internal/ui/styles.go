package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// screenMsg is sent by child models when they want to push a new screen.
type screenMsg struct{ model tea.Model }

// popMsg is sent by child models when they want to pop themselves off the stack.
type popMsg struct{}

// reloadMsg is sent after a mutating action to trigger a repo refresh.
type reloadMsg struct{}

// PushScreen returns a command that pushes a new model onto the app stack.
func PushScreen(m tea.Model) tea.Cmd {
	return func() tea.Msg { return screenMsg{model: m} }
}

// PopScreen returns a command that pops the current model from the app stack.
func PopScreen() tea.Cmd {
	return func() tea.Msg { return popMsg{} }
}

// Reload returns a command that signals the app to reload repos.
func Reload() tea.Cmd {
	return func() tea.Msg { return reloadMsg{} }
}

// Styles shared across all views.
var (
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	StyleDryRunBanner = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("11")).
				Padding(0, 1)

	StyleErrorBanner = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("1")).
				Padding(0, 1)

	StyleHelp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	StyleFork = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3")) // yellow

	StylePublic = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")) // orange

	StyleLargeSize = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")) // red

	StyleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10")) // bright green
)
