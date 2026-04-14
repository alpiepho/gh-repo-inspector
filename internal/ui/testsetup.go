package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type setupStep struct {
	label   string
	command string
	purpose string
	done    bool
}

// TestSetup is a paginated checklist for creating test repos.
type TestSetup struct {
	app    *App
	steps  []setupStep
	cursor int
}

func NewTestSetup(app *App) *TestSetup {
	return &TestSetup{
		app: app,
		steps: []setupStep{
			{
				label:   "Create public repo with description",
				command: `gh repo create test-public-alpha --public --description "Alpha test repo"`,
				purpose: "Tests Phase 1 display, Phase 3 (make private)",
			},
			{
				label:   "Create public repo without description",
				command: `gh repo create test-public-beta --public`,
				purpose: "Tests Phase 3 with missing description",
			},
			{
				label:   "Create private repo",
				command: `gh repo create test-private-one --private --description "Private test repo"`,
				purpose: "Tests Phase 1 visibility display",
			},
			{
				label:   "Fork a public repo",
				command: `gh repo fork cli/cli --clone=false`,
				purpose: "Tests Phase 2 (fork review + delete)",
			},
			{
				label:   "Create old/inactive test repo",
				command: `gh repo create test-old-repo --public --description "Stale repo for age testing"`,
				purpose: "Tests Phase 4 age sort (appears newest initially — age naturally over time)",
			},
			{
				label:   "Create a repo to test cloning",
				command: `gh repo create test-clone-me --public --description "For clone testing"`,
				purpose: "Tests Phase 5 clone workflow",
			},
		},
	}
}

func (ts *TestSetup) Init() tea.Cmd { return nil }

func (ts *TestSetup) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			return ts, PopScreen()
		case "up", "k":
			if ts.cursor > 0 {
				ts.cursor--
			}
		case "down", "j":
			if ts.cursor < len(ts.steps)-1 {
				ts.cursor++
			}
		case " ", "enter":
			ts.steps[ts.cursor].done = !ts.steps[ts.cursor].done
		}
	}
	return ts, nil
}

func (ts *TestSetup) View() string {
	title := StyleTitle.Render("Test Setup Guide")
	subtitle := StyleHelp.Render("  Run each command in a separate terminal, then mark it done here.")

	doneCount := 0
	for _, s := range ts.steps {
		if s.done {
			doneCount++
		}
	}
	progress := StyleHelp.Render(fmt.Sprintf("  Progress: %d / %d", doneCount, len(ts.steps)))

	checkDone := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	checkPending := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).PaddingLeft(6)
	purposeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(6)

	var rows strings.Builder
	for i, s := range ts.steps {
		checkbox := checkPending.Render("[ ]")
		if s.done {
			checkbox = checkDone.Render("[✓]")
		}
		cursor := "  "
		label := s.label
		if i == ts.cursor {
			cursor = cursorStyle.Render("❯ ")
			label = cursorStyle.Render(s.label)
		}
		rows.WriteString(fmt.Sprintf("\n\n%s%s %s", cursor, checkbox, label))
		if i == ts.cursor {
			rows.WriteString("\n" + cmdStyle.Render("$ "+s.command))
			rows.WriteString("\n" + purposeStyle.Render("→ "+s.purpose))
		}
	}

	help := "\n\n" + StyleHelp.Render("  ↑/k up  ↓/j down  space/enter toggle done  esc back")

	return title + "\n" + subtitle + "\n" + progress + rows.String() + help
}
