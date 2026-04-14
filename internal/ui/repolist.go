package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/al/gh-repo-inspector/internal/gh"
	"github.com/al/gh-repo-inspector/internal/oplog"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Filter controls which repos are shown.
type Filter int

const (
	FilterAll    Filter = iota
	FilterForks
	FilterPublic
)

// Mode controls which actions are available.
type Mode int

const (
	ModeInspect      Mode = iota // read-only
	ModeDelete                   // d = delete
	ModePrivate                  // p = make private
	ModeDeleteOldest             // d = delete, sorted oldest first
)

// RepoList is the shared sortable/filterable repo table.
type RepoList struct {
	app       *App
	all       []gh.Repo // full unfiltered list
	visible   []gh.Repo // after filter applied
	filter    Filter
	mode      Mode
	cursor    int
	offset    int // first rendered row (for scrolling)
	query     string
	searching bool
	err       string
	width     int
	height    int
}

func NewRepoList(app *App, repos []gh.Repo, filter Filter, mode Mode) *RepoList {
	rl := &RepoList{
		app:    app,
		all:    repos,
		filter: filter,
		mode:   mode,
		width:  app.width,
		height: app.height,
	}
	rl.applyFilter()
	return rl
}

func (rl *RepoList) Init() tea.Cmd { return nil }

func (rl *RepoList) applyFilter() {
	var filtered []gh.Repo
	q := strings.ToLower(rl.query)
	for _, r := range rl.all {
		switch rl.filter {
		case FilterForks:
			if !r.IsFork {
				continue
			}
		case FilterPublic:
			if r.IsPrivate {
				continue
			}
		}
		if q != "" && !strings.Contains(strings.ToLower(r.Name), q) &&
			!strings.Contains(strings.ToLower(r.Description), q) {
			continue
		}
		filtered = append(filtered, r)
	}
	if rl.mode == ModeDeleteOldest {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].PushedAt.Before(filtered[j].PushedAt)
		})
	}
	rl.visible = filtered
	if rl.cursor >= len(rl.visible) {
		rl.cursor = max(0, len(rl.visible)-1)
	}
	rl.clampOffset()
}

// viewportHeight returns how many list rows fit on screen.
func (rl *RepoList) viewportHeight() int {
	// chrome: title(1) + optional banner(1) + blank(1) + header(1) + blank(1) + help(1) = ~6-8 lines
	chrome := 8
	h := rl.height - chrome
	if h < 1 {
		h = 20 // fallback before first WindowSizeMsg
	}
	return h
}

func (rl *RepoList) clampOffset() {
	vh := rl.viewportHeight()
	if rl.cursor < rl.offset {
		rl.offset = rl.cursor
	}
	if rl.cursor >= rl.offset+vh {
		rl.offset = rl.cursor - vh + 1
	}
	if rl.offset < 0 {
		rl.offset = 0
	}
}

func (rl *RepoList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		rl.width = msg.Width
		rl.height = msg.Height
		rl.clampOffset()

	case tea.KeyMsg:
		if rl.searching {
			return rl.updateSearch(msg)
		}
		return rl.updateNav(msg)
	}
	return rl, nil
}

func (rl *RepoList) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		rl.searching = false
	case "backspace":
		if len(rl.query) > 0 {
			rl.query = rl.query[:len(rl.query)-1]
			rl.applyFilter()
		}
	default:
		if len(msg.Runes) > 0 {
			rl.query += string(msg.Runes)
			rl.applyFilter()
		}
	}
	return rl, nil
}

func (rl *RepoList) updateNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rl.err = ""
	switch msg.String() {
	case "q", "esc":
		return rl, PopScreen()
	case "up", "k":
		if rl.cursor > 0 {
			rl.cursor--
			rl.clampOffset()
		}
	case "down", "j":
		if rl.cursor < len(rl.visible)-1 {
			rl.cursor++
			rl.clampOffset()
		}
	case "/":
		rl.searching = true
	case "d":
		if rl.mode == ModeDelete || rl.mode == ModeDeleteOldest {
			return rl, rl.confirmAction("delete")
		}
	case "p":
		if rl.mode == ModePrivate {
			return rl, rl.confirmAction("private")
		}
	case "D":
		rl.app.DryRun = !rl.app.DryRun
	}
	return rl, nil
}

func (rl *RepoList) confirmAction(action string) tea.Cmd {
	if len(rl.visible) == 0 {
		return nil
	}
	repo := rl.visible[rl.cursor]
	return PushScreen(NewConfirm(rl.app, repo, action, func(confirmed bool) tea.Cmd {
		if !confirmed {
			return PopScreen()
		}
		var err error
		switch action {
		case "delete":
			err = gh.DeleteRepo(repo.URL, rl.app.DryRun)
		case "private":
			err = gh.SetPrivate(repo.URL, rl.app.DryRun)
		}
		// Log the operation.
		opName := "DELETE"
		if action == "private" {
			opName = "MAKE-PRIVATE"
		}
		logResult := "success"
		switch {
		case rl.app.DryRun:
			logResult = "[DRY-RUN]"
		case err != nil:
			logResult = "error: " + err.Error()
		}
		oplog.Write(opName, repo.Name, logResult)
		if err != nil {
			rl.err = err.Error()
			return PopScreen()
		}
		if !rl.app.DryRun {
			// Remove from local lists.
			rl.all = removeRepo(rl.all, repo.Name)
			rl.applyFilter()
		}
		return PopScreen()
	}))
}

func removeRepo(repos []gh.Repo, name string) []gh.Repo {
	out := repos[:0]
	for _, r := range repos {
		if r.Name != name {
			out = append(out, r)
		}
	}
	return out
}

func (rl *RepoList) View() string {
	title := rl.modeTitle()

	var banner string
	if rl.app.DryRun {
		banner = "\n" + StyleDryRunBanner.Render("⚠  DRY RUN — no changes will be made")
	}
	var errBanner string
	if rl.err != "" {
		errBanner = "\n" + StyleErrorBanner.Render("Error: "+rl.err)
	}

	if len(rl.visible) == 0 {
		empty := "\n\n  " + StyleHelp.Render("No repos match.")
		return StyleTitle.Render(title) + banner + errBanner + empty + "\n\n" + rl.helpBar()
	}

	header := fmt.Sprintf("  %-30s %-12s %-8s %-6s %-5s %-10s  %s",
		"NAME", "LAST COMMIT", "VIS", "FORK", "BR", "SIZE", "DESCRIPTION")
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))

	vh := rl.viewportHeight()
	end := rl.offset + vh
	if end > len(rl.visible) {
		end = len(rl.visible)
	}

	var rows strings.Builder
	for i := rl.offset; i < end; i++ {
		r := rl.visible[i]
		vis := "public"
		if r.IsPrivate {
			vis = "private"
		}
		fork := "no"
		if r.IsFork {
			fork = "yes"
		}
		age := formatAge(r.PushedAt)
		size := gh.FormatSize(r.DiskUsage)
		desc := r.Description
		if len(desc) > 40 {
			desc = desc[:37] + "…"
		}

		line := fmt.Sprintf("  %-30s %-12s %-8s %-6s %-5d %-10s  %s",
			truncate(r.Name, 30), age, vis, fork, r.BranchCount, size, desc)

		// Apply colour before highlight so highlight can override background.
		switch {
		case r.IsFork:
			line = StyleFork.Render(line)
		case !r.IsPrivate:
			line = StylePublic.Render(line)
		}

		if i == rl.cursor {
			line = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("237")).
				Render(line)
		}

		rows.WriteString("\n" + line)
	}

	// Scroll indicator
	scrollInfo := fmt.Sprintf("%d-%d / %d", rl.offset+1, end, len(rl.visible))
	scrollStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var searchLine string
	if rl.searching || rl.query != "" {
		searchLine = "\n  Filter: " + rl.query
		if rl.searching {
			searchLine += "█"
		}
	}

	return StyleTitle.Render(title) + "  " + scrollStyle.Render(scrollInfo) +
		banner + errBanner + "\n" +
		headerStyle.Render(header) + rows.String() + searchLine + "\n\n" + rl.helpBar()
}

func (rl *RepoList) modeTitle() string {
	switch rl.mode {
	case ModeDelete:
		return "Review Forks"
	case ModePrivate:
		return "Review Public Repos"
	case ModeDeleteOldest:
		return "Review by Age (Oldest First)"
	default:
		return "Inspect All Repos"
	}
}

func (rl *RepoList) helpBar() string {
	base := "↑/k up  ↓/j down  / search  D toggle-dry-run  esc back"
	switch rl.mode {
	case ModeDelete, ModeDeleteOldest:
		base = "d delete  " + base
	case ModePrivate:
		base = "p make-private  " + base
	}
	return StyleHelp.Render("  " + base)
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/24/7))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/24/365))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
