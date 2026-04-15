package ui

import (
	"fmt"

	"github.com/al/gh-repo-inspector/internal/gh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type menuItem struct {
	label string
	desc  string
}

var menuItems = []menuItem{
	{"Inspect All Repos", "Browse all repos with full metadata"},
	{"Review Forks", "Review forked repos — delete unwanted ones"},
	{"Review Public Repos", "Review public repos — make private"},
	{"Review Private Repos", "Review private repos — make public"},
	{"Review by Age", "Oldest repos first — delete stale ones"},
	{"Clone Repos", "Select repos to clone locally"},
	{"Refresh Clones", "Run git pull on previously cloned repos"},
	{"Test Setup Guide", "Step-by-step guide for creating test repos"},
	{"Migrate to GitLab", "Push local repos to your local GitLab instance"},
}

// Menu is the main menu model.
type Menu struct {
	app      *App
	cursor   int
	repos    []gh.Repo
	repoErr  error
	loading  bool
}

func NewMenu(app *App) *Menu {
	return &Menu{app: app, loading: true}
}

func (m *Menu) onRepos(repos []gh.Repo, err error) {
	m.repos = repos
	m.repoErr = err
	m.loading = false
}

func (m *Menu) Init() tea.Cmd { return nil }

func (m *Menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		case "enter", " ":
			if !m.loading {
				return m, m.selectItem()
			}
		default:
			if len(msg.Runes) == 1 {
				switch msg.Runes[0] {
				case '0': // Reload repo list
					m.loading = true
					m.repos = nil
					m.repoErr = nil
					return m, fetchRepos
				default:
					// Number shortcuts: 1–N jump directly to that item.
					n := int(msg.Runes[0] - '1')
					if n >= 0 && n < len(menuItems) && !m.loading {
						m.cursor = n
						return m, m.selectItem()
					}
				}
			}
		}
	}
	return m, nil
}

func (m *Menu) selectItem() tea.Cmd {
	switch m.cursor {
	case 0: // Inspect all
		return PushScreen(NewRepoList(m.app, m.repos, FilterAll, ModeInspect))
	case 1: // Forks
		return PushScreen(NewRepoList(m.app, m.repos, FilterForks, ModeDelete))
	case 2: // Public → make private
		return PushScreen(NewRepoList(m.app, m.repos, FilterPublic, ModePrivate))
	case 3: // Private → make public
		return PushScreen(NewRepoList(m.app, m.repos, FilterPrivate, ModePublic))
	case 4: // By age
		return PushScreen(NewRepoList(m.app, m.repos, FilterAll, ModeDeleteOldest))
	case 5: // Clone
		return PushScreen(NewCloneView(m.app, m.repos))
	case 6: // Refresh clones
		return PushScreen(NewRefreshView(m.app))
	case 7: // Test setup
		return PushScreen(NewTestSetup(m.app))
	case 8: // Migrate to GitLab
		return PushScreen(NewMigrateView(m.app, m.repos))
	}
	return nil
}

func (m *Menu) View() string {
	titleStyle := StyleTitle.Margin(1, 0, 0, 2)
	title := titleStyle.Render("gh-repo-inspector")

	var banner string
	if m.app.DryRun {
		banner = "\n" + StyleDryRunBanner.Render("⚠  DRY RUN — no changes will be made")
	}

	var status string
	if m.loading {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).PaddingLeft(2).Render("  Loading repos…")
	} else if m.repoErr != nil {
		status = StyleErrorBanner.Render(fmt.Sprintf("Error: %v", m.repoErr))
	} else {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).PaddingLeft(2).Render(fmt.Sprintf("  %d repos loaded", len(m.repos)))
	}

	itemStyle := lipgloss.NewStyle().Padding(0, 2)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).PaddingLeft(4)
	reloadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 2)

	reloadLabel := reloadStyle.Render("0. Reload repo list from GitHub")

	var items string
	for i, item := range menuItems {
		num := fmt.Sprintf("%d. ", i+1)
		cursor := "  "
		label := itemStyle.Render(num + item.label)
		if i == m.cursor {
			cursor = cursorStyle.Render("❯ ")
			label = cursorStyle.Render(num + item.label)
		}
		items += "\n" + cursor + label
		if i == m.cursor {
			items += "\n" + descStyle.Render(item.desc)
		}
	}

	help := "\n\n" + StyleHelp.Render("  ↑/k up  ↓/j down  0 reload  1-9 jump  enter select  q quit")

	return title + banner + "\n" + status + "\n" + reloadLabel + "\n" + items + help
}
