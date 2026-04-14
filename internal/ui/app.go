package ui

import (
	"github.com/al/gh-repo-inspector/internal/gh"
	tea "github.com/charmbracelet/bubbletea"
)

// fetchReposMsg carries the result of loading repos from gh.
type fetchReposMsg struct {
	repos []gh.Repo
	err   error
}

func fetchRepos() tea.Msg {
	repos, err := gh.ListRepos()
	return fetchReposMsg{repos: repos, err: err}
}

// App is the root Bubble Tea model. It owns the navigation stack and global state.
type App struct {
	stack  []tea.Model
	width  int
	height int
	DryRun bool
}

// NewApp creates the root app model. repos may be nil initially (loaded async).
func NewApp(dryRun bool) *App {
	a := &App{DryRun: dryRun}
	a.stack = []tea.Model{NewMenu(a)}
	return a
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(
		fetchRepos,
		a.top().Init(),
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

	case screenMsg:
		a.stack = append(a.stack, msg.model)
		return a, a.top().Init()

	case popMsg:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		} else {
			return a, tea.Quit
		}
		return a, nil

	case reloadMsg:
		return a, fetchRepos

	case fetchReposMsg:
		// Deliver repos to menu so phases can use them.
		if menu, ok := a.stack[0].(*Menu); ok {
			menu.onRepos(msg.repos, msg.err)
		}
		return a, nil
	}

	// Delegate to top of stack.
	updated, cmd := a.top().Update(msg)
	a.stack[len(a.stack)-1] = updated
	return a, cmd
}

func (a *App) View() string {
	return a.top().View()
}

func (a *App) top() tea.Model {
	return a.stack[len(a.stack)-1]
}
