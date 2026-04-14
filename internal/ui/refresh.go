package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/al/gh-repo-inspector/internal/config"
	"github.com/al/gh-repo-inspector/internal/gh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type refreshState int

const (
	refreshStateDir      refreshState = iota // typing scan directory
	refreshStateScanning                     // scanning in progress
	refreshStateSelect                       // choosing repos to pull
	refreshStateRunning                      // pulling
	refreshStateDone                         // finished
)

// RefreshView scans a directory for git repos and runs git pull on selected ones.
type RefreshView struct {
	app            *App
	scanDir        string
	repos          []gh.LocalRepo
	selected       map[int]bool
	cursor         int
	offset         int
	height         int
	state          refreshState
	scanErr        error
	results        []string
	pendingRepos   []gh.LocalRepo
	currentCmd     string
	cfg            *config.Config
	selectMenuOpen bool
	selectMenuCur  int
}

type refreshSelectOption struct {
	label  string
	filter func(r gh.LocalRepo) bool
}

var refreshSelectOptions = []refreshSelectOption{
	{"All", func(r gh.LocalRepo) bool { return true }},
	{"Clear selection", nil},
}

func NewRefreshView(app *App) *RefreshView {
	cfg, _ := config.Load()
	dir, _ := os.UserHomeDir()
	if cfg.LastClonePath != "" {
		dir = cfg.LastClonePath
	}
	return &RefreshView{
		app:      app,
		scanDir:  dir,
		selected: make(map[int]bool),
		height:   app.height,
		cfg:      cfg,
		state:    refreshStateDir,
	}
}

func (rv *RefreshView) Init() tea.Cmd { return nil }

func (rv *RefreshView) viewportHeight() int {
	h := rv.height - 7
	if h < 1 {
		h = 20
	}
	return h
}

func (rv *RefreshView) clampOffset() {
	vh := rv.viewportHeight()
	if rv.cursor < rv.offset {
		rv.offset = rv.cursor
	}
	if rv.cursor >= rv.offset+vh {
		rv.offset = rv.cursor - vh + 1
	}
	if rv.offset < 0 {
		rv.offset = 0
	}
}

// scanMsg carries the result of scanning a directory for git repos.
type scanMsg struct {
	repos []gh.LocalRepo
	err   error
}

// pullProgressMsg is sent when one git pull completes.
type pullProgressMsg struct {
	result  string
	nextIdx int
}

func doScan(dir string) tea.Cmd {
	return func() tea.Msg {
		repos, err := gh.ScanLocalRepos(dir)
		return scanMsg{repos: repos, err: err}
	}
}

func (rv *RefreshView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scanMsg:
		rv.scanErr = msg.err
		rv.repos = msg.repos
		rv.selected = make(map[int]bool)
		rv.cursor = 0
		rv.offset = 0
		rv.state = refreshStateSelect
		return rv, nil

	case pullProgressMsg:
		if msg.result != "" {
			rv.results = append(rv.results, msg.result)
		}
		if msg.nextIdx >= len(rv.pendingRepos) {
			rv.currentCmd = ""
			rv.state = refreshStateDone
			return rv, nil
		}
		r := rv.pendingRepos[msg.nextIdx]
		rv.currentCmd = "git -C " + r.Path + " pull"
		return rv, rv.pullOne(msg.nextIdx)

	case tea.WindowSizeMsg:
		rv.height = msg.Height
		rv.clampOffset()

	case tea.KeyMsg:
		switch rv.state {
		case refreshStateDir:
			return rv.updateDir(msg)
		case refreshStateScanning:
			// block input while scanning
		case refreshStateSelect:
			return rv.updateSelect(msg)
		case refreshStateDone:
			if msg.String() == "esc" || msg.String() == "q" {
				return rv, PopScreen()
			}
		}
	}
	return rv, nil
}

func (rv *RefreshView) updateDir(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return rv, PopScreen()
	case "enter":
		rv.state = refreshStateScanning
		return rv, doScan(rv.scanDir)
	case "backspace":
		if len(rv.scanDir) > 0 {
			rv.scanDir = rv.scanDir[:len(rv.scanDir)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			rv.scanDir += string(msg.Runes)
		}
	}
	return rv, nil
}

func (rv *RefreshView) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if rv.selectMenuOpen {
		return rv.updateSelectMenu(msg)
	}
	switch msg.String() {
	case "esc", "q":
		// Return to directory prompt.
		rv.state = refreshStateDir
		rv.repos = nil
		rv.selected = make(map[int]bool)
	case "up", "k":
		if rv.cursor > 0 {
			rv.cursor--
			rv.clampOffset()
		}
	case "down", "j":
		if rv.cursor < len(rv.repos)-1 {
			rv.cursor++
			rv.clampOffset()
		}
	case " ":
		rv.selected[rv.cursor] = !rv.selected[rv.cursor]
	case "a":
		rv.selectMenuOpen = true
		rv.selectMenuCur = 0
	case "enter":
		if len(rv.selected) > 0 {
			rv.state = refreshStateRunning
			return rv, rv.startPulling()
		}
	case "D":
		rv.app.DryRun = !rv.app.DryRun
	}
	return rv, nil
}

func (rv *RefreshView) updateSelectMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		rv.selectMenuOpen = false
	case "up", "k":
		if rv.selectMenuCur > 0 {
			rv.selectMenuCur--
		}
	case "down", "j":
		if rv.selectMenuCur < len(refreshSelectOptions)-1 {
			rv.selectMenuCur++
		}
	case "enter", " ":
		opt := refreshSelectOptions[rv.selectMenuCur]
		rv.selected = make(map[int]bool)
		if opt.filter != nil {
			for i, r := range rv.repos {
				if opt.filter(r) {
					rv.selected[i] = true
				}
			}
		}
		rv.selectMenuOpen = false
	}
	return rv, nil
}

func (rv *RefreshView) startPulling() tea.Cmd {
	rv.pendingRepos = rv.selectedRepos()
	rv.results = nil
	rv.currentCmd = ""
	if len(rv.pendingRepos) == 0 {
		rv.state = refreshStateDone
		return nil
	}
	rv.currentCmd = "git -C " + rv.pendingRepos[0].Path + " pull"
	return rv.pullOne(0)
}

func (rv *RefreshView) pullOne(idx int) tea.Cmd {
	r := rv.pendingRepos[idx]
	dryRun := rv.app.DryRun
	next := idx + 1

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		err := gh.PullRepo(ctx, r.Path, dryRun)
		var line string
		switch {
		case dryRun:
			line = fmt.Sprintf("[DRY RUN] git -C %s pull", r.Path)
		case err != nil:
			line = fmt.Sprintf("✗ %s: %v", r.Name, err)
		default:
			line = fmt.Sprintf("✓ %s  (%s)", r.Name, r.Path)
		}
		return pullProgressMsg{result: line, nextIdx: next}
	}
}

func (rv *RefreshView) selectedRepos() []gh.LocalRepo {
	var out []gh.LocalRepo
	for i, r := range rv.repos {
		if rv.selected[i] {
			out = append(out, r)
		}
	}
	return out
}

func (rv *RefreshView) View() string {
	title := StyleTitle.Render("Refresh Clones  (git pull)")
	var banner string
	if rv.app.DryRun {
		banner = "\n" + StyleDryRunBanner.Render("⚠  DRY RUN — no changes will be made")
	}

	switch rv.state {
	case refreshStateDir:
		return title + banner + "\n" + rv.viewDir()
	case refreshStateScanning:
		return title + banner + fmt.Sprintf("\n\n  Scanning %s…", rv.scanDir)
	case refreshStateSelect:
		base := title + banner + "\n" + rv.viewSelect()
		if rv.selectMenuOpen {
			return base + "\n" + rv.viewSelectMenu()
		}
		return base
	case refreshStateRunning:
		var b strings.Builder
		b.WriteString("\n")
		for _, line := range rv.results {
			b.WriteString("  " + line + "\n")
		}
		if rv.currentCmd != "" {
			b.WriteString("\n  ▶ " + StyleHelp.Render(rv.currentCmd) + "\n")
		}
		return title + banner + b.String()
	case refreshStateDone:
		return title + banner + "\n\n" + rv.viewDone()
	}
	return ""
}

func (rv *RefreshView) viewDir() string {
	prompt := fmt.Sprintf("\n  Directory to scan for git repos:\n\n  %s█\n\n", rv.scanDir)
	note := StyleHelp.Render("  Scans immediate subdirectories for .git folders.")
	help := "\n" + StyleHelp.Render("  enter scan  esc back")
	return prompt + note + help
}

func (rv *RefreshView) viewSelect() string {
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))

	if rv.scanErr != nil {
		return fmt.Sprintf("\n  Error scanning %s:\n  %v\n\n%s",
			rv.scanDir, rv.scanErr, StyleHelp.Render("  esc to try a different directory"))
	}
	if len(rv.repos) == 0 {
		return fmt.Sprintf("\n  No git repos found in %s\n\n%s",
			rv.scanDir, StyleHelp.Render("  esc to try a different directory"))
	}

	vh := rv.viewportHeight()
	end := rv.offset + vh
	if end > len(rv.repos) {
		end = len(rv.repos)
	}

	header := headerStyle.Render(fmt.Sprintf("     %-28s %-16s %s",
		"NAME", "BRANCH", "LAST COMMIT"))

	var rows strings.Builder
	for i := rv.offset; i < end; i++ {
		r := rv.repos[i]

		branch := r.Branch
		if branch == "" {
			branch = "?"
		}
		lastCommit := r.LastCommit
		if lastCommit == "" {
			lastCommit = "unknown"
		}

		checkbox := "[ ]"
		if rv.selected[i] {
			checkbox = selectedStyle.Render("[✓]")
		}
		cur := "  "
		if i == rv.cursor {
			cur = cursorStyle.Render("❯ ")
		}

		line := fmt.Sprintf("%s%s %-28s %-16s %s",
			cur, checkbox,
			truncate(r.Name, 28),
			branch,
			lastCommit,
		)
		if i == rv.cursor {
			line = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("237")).Render(line)
		}
		rows.WriteString("\n" + line)
	}

	scrollInfo := fmt.Sprintf("%d-%d / %d  in %s", rv.offset+1, end, len(rv.repos), rv.scanDir)
	selInfo := fmt.Sprintf("%d selected", len(rv.selected))
	status := StyleTitle.Render(selInfo) + "  " + StyleHelp.Render(scrollInfo)

	help := StyleHelp.Render("  space toggle  a select-all  enter pull selected  D dry-run  esc rescan")
	return "\n" + header + "\n" + status + rows.String() + "\n\n" + help
}

func (rv *RefreshView) viewSelectMenu() string {
	menuStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(0, 2).
		Margin(0, 4)

	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Select group:") + "\n")
	for i, opt := range refreshSelectOptions {
		cur := "  "
		label := opt.label
		if i == rv.selectMenuCur {
			cur = cursorStyle.Render("❯ ")
			label = cursorStyle.Render(opt.label)
		}
		b.WriteString("\n" + cur + label)
	}
	b.WriteString("\n\n" + StyleHelp.Render("↑/k up  ↓/j down  enter select  esc cancel"))
	return menuStyle.Render(b.String())
}

func (rv *RefreshView) viewDone() string {
	var b strings.Builder
	for _, r := range rv.results {
		b.WriteString("  " + r + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("  esc to go back"))
	return b.String()
}
