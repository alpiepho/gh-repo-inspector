package ui

import (
	"fmt"

	"github.com/al/gh-repo-inspector/internal/gh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ActionCallback is called with confirmed=true/false after the user responds.
type ActionCallback func(confirmed bool) tea.Cmd

// Confirm is a single-action confirmation dialog.
type Confirm struct {
	app      *App
	repo     gh.Repo
	action   string
	callback ActionCallback
}

func NewConfirm(app *App, repo gh.Repo, action string, callback ActionCallback) *Confirm {
	return &Confirm{app: app, repo: repo, action: action, callback: callback}
}

func (c *Confirm) Init() tea.Cmd { return nil }

func (c *Confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "y", "Y", "enter":
			return c, c.callback(true)
		case "n", "N", "esc", "q":
			return c, c.callback(false)
		}
	}
	return c, nil
}

func (c *Confirm) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("11")).
		Padding(1, 3).
		Margin(2, 4)

	var prompt string
	if c.app.DryRun {
		label := c.actionLabel()
		prompt = StyleDryRunBanner.Render("⚠  DRY RUN") + "\n\n" +
			fmt.Sprintf("Would %s: %s\n\n", label, c.repo.Name) +
			StyleHelp.Render("y/enter = simulate  n/esc = cancel")
	} else {
		label := c.actionLabel()
		prompt = fmt.Sprintf("Are you sure you want to %s:\n\n  %s\n\n", label, c.repo.Name) +
			StyleHelp.Render("y/enter = confirm  n/esc = cancel")
	}

	return boxStyle.Render(prompt)
}

func (c *Confirm) actionLabel() string {
	switch c.action {
	case "delete":
		return "DELETE"
	case "private":
		return "make PRIVATE"
	default:
		return c.action
	}
}
