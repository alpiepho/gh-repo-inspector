package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/al/gh-repo-inspector/internal/config"
	"github.com/al/gh-repo-inspector/internal/gh"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type cloneState int

const (
	cloneStateSelect  cloneState = iota // choosing repos
	cloneStateDestDir                   // typing destination
	cloneStateRunning                   // cloning in progress
	cloneStateDone                      // finished
)

// CloneView handles multi-select + cloning.
type CloneView struct {
	app            *App
	repos          []gh.Repo
	selected       map[int]bool
	cursor         int
	offset         int
	height         int
	state          cloneState
	destDir        string
	allBranches    bool
	results        []string   // completed lines (✓/✗/⚠/dry-run)
	pendingRepos   []gh.Repo  // repos queued for this clone run
	currentCmd     string     // command currently executing (for display)
	cfg            *config.Config
	selectMenuOpen bool
	selectMenuCur  int
}

type selectMenuOption struct {
	label  string
	filter func(r gh.Repo) bool
}

var selectMenuOptions = []selectMenuOption{
	{"All repos", func(r gh.Repo) bool { return true }},
	{"Forks only", func(r gh.Repo) bool { return r.IsFork }},
	{"Public (non-fork)", func(r gh.Repo) bool { return !r.IsPrivate && !r.IsFork }},
	{"Private (non-fork)", func(r gh.Repo) bool { return r.IsPrivate && !r.IsFork }},
	{"Clear selection", nil},
}

func NewCloneView(app *App, repos []gh.Repo) *CloneView {
	cfg, _ := config.Load()
	destDir, _ := os.UserHomeDir()
	if cfg.LastClonePath != "" {
		destDir = cfg.LastClonePath
	}
	return &CloneView{
		app:      app,
		repos:    repos,
		selected: make(map[int]bool),
		height:   app.height,
		destDir:  destDir,
		cfg:      cfg,
	}
}

func (cv *CloneView) Init() tea.Cmd { return nil }

func (cv *CloneView) viewportHeight() int {
	h := cv.height - 7
	if h < 1 {
		h = 20
	}
	return h
}

func (cv *CloneView) clampOffset() {
	vh := cv.viewportHeight()
	if cv.cursor < cv.offset {
		cv.offset = cv.cursor
	}
	if cv.cursor >= cv.offset+vh {
		cv.offset = cv.cursor - vh + 1
	}
	if cv.offset < 0 {
		cv.offset = 0
	}
}

// cloneProgressMsg is sent when one repo finishes cloning.
// nextIdx is the index of the next repo to start, or len(pendingRepos) when done.
type cloneProgressMsg struct {
	result  string // completed line (may be empty for first kick-off)
	nextIdx int
}

func (cv *CloneView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cloneProgressMsg:
		if msg.result != "" {
			cv.results = append(cv.results, msg.result)
		}
		if msg.nextIdx >= len(cv.pendingRepos) {
			cv.currentCmd = ""
			cv.state = cloneStateDone
			return cv, nil
		}
		// Advance to next repo.
		r := cv.pendingRepos[msg.nextIdx]
		cv.currentCmd = cv.buildCmdStr(r)
		return cv, cv.cloneOne(msg.nextIdx)

	case tea.WindowSizeMsg:
		cv.height = msg.Height
		cv.clampOffset()

	case tea.KeyMsg:
		switch cv.state {
		case cloneStateSelect:
			return cv.updateSelect(msg)
		case cloneStateDestDir:
			return cv.updateDestDir(msg)
		case cloneStateDone:
			if msg.String() == "esc" || msg.String() == "q" {
				return cv, PopScreen()
			}
		}
	}
	return cv, nil
}

func (cv *CloneView) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if cv.selectMenuOpen {
		return cv.updateSelectMenu(msg)
	}
	switch msg.String() {
	case "esc", "q":
		return cv, PopScreen()
	case "up", "k":
		if cv.cursor > 0 {
			cv.cursor--
			cv.clampOffset()
		}
	case "down", "j":
		if cv.cursor < len(cv.repos)-1 {
			cv.cursor++
			cv.clampOffset()
		}
	case " ":
		cv.selected[cv.cursor] = !cv.selected[cv.cursor]
	case "a":
		cv.selectMenuOpen = true
		cv.selectMenuCur = 0
	case "b":
		cv.allBranches = !cv.allBranches
	case "enter":
		if len(cv.selected) > 0 {
			cv.state = cloneStateDestDir
		}
	case "D":
		cv.app.DryRun = !cv.app.DryRun
	}
	return cv, nil
}

func (cv *CloneView) updateSelectMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		cv.selectMenuOpen = false
	case "up", "k":
		if cv.selectMenuCur > 0 {
			cv.selectMenuCur--
		}
	case "down", "j":
		if cv.selectMenuCur < len(selectMenuOptions)-1 {
			cv.selectMenuCur++
		}
	case "enter", " ":
		opt := selectMenuOptions[cv.selectMenuCur]
		cv.selected = make(map[int]bool)
		if opt.filter != nil {
			for i, r := range cv.repos {
				if opt.filter(r) {
					cv.selected[i] = true
				}
			}
		}
		cv.selectMenuOpen = false
	}
	return cv, nil
}

func (cv *CloneView) updateDestDir(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		cv.state = cloneStateSelect
	case "enter":
		// Save path and start cloning.
		cv.cfg.LastClonePath = cv.destDir
		_ = cv.cfg.Save()
		cv.state = cloneStateRunning
		return cv, cv.startCloning()
	case "backspace":
		if len(cv.destDir) > 0 {
			cv.destDir = cv.destDir[:len(cv.destDir)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			cv.destDir += string(msg.Runes)
		}
	}
	return cv, nil
}

// startCloning initialises the pending queue and kicks off the first clone.
func (cv *CloneView) startCloning() tea.Cmd {
	cv.pendingRepos = cv.selectedRepos()
	cv.results = nil
	cv.currentCmd = ""
	if len(cv.pendingRepos) == 0 {
		cv.state = cloneStateDone
		return nil
	}
	cv.currentCmd = cv.buildCmdStr(cv.pendingRepos[0])
	return cv.cloneOne(0)
}

// cloneOne runs a single clone and returns a cloneProgressMsg when done.
func (cv *CloneView) cloneOne(idx int) tea.Cmd {
	r := cv.pendingRepos[idx]
	dest := filepath.Join(cv.destDir, r.Name)
	allBranches := cv.allBranches
	dryRun := cv.app.DryRun
	cfg := cv.cfg
	next := idx + 1

	return func() tea.Msg {
		if _, err := os.Stat(dest); err == nil && !dryRun {
			line := fmt.Sprintf("⚠  %s — directory exists at %s, skipping", r.Name, dest)
			return cloneProgressMsg{result: line, nextIdx: next}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		err := gh.CloneRepo(ctx, r.URL, dest, allBranches, dryRun)
		var line string
		if dryRun {
			mode := "default branch"
			if allBranches {
				mode = "all branches"
			}
			line = fmt.Sprintf("[DRY RUN] git clone %s → %s  (%s)", r.URL, dest, mode)
		} else if err != nil {
			line = fmt.Sprintf("✗ %s: %v", r.Name, err)
		} else {
			line = fmt.Sprintf("✓ %s → %s", r.Name, dest)
			cfg.RecordClone(r.Name, dest, allBranches)
			_ = cfg.Save()
		}
		return cloneProgressMsg{result: line, nextIdx: next}
	}
}

// buildCmdStr returns a human-readable clone command for display.
func (cv *CloneView) buildCmdStr(r gh.Repo) string {
	dest := filepath.Join(cv.destDir, r.Name)
	cmd := "git clone"
	if !cv.allBranches {
		cmd += " --single-branch"
	}
	return cmd + " " + r.URL + "  →  " + dest
}

func (cv *CloneView) selectedRepos() []gh.Repo {
	var out []gh.Repo
	for i, r := range cv.repos {
		if cv.selected[i] {
			out = append(out, r)
		}
	}
	return out
}

func (cv *CloneView) View() string {
	title := StyleTitle.Render("Clone Repos")
	var banner string
	if cv.app.DryRun {
		banner = "\n" + StyleDryRunBanner.Render("⚠  DRY RUN — no changes will be made")
	}

	switch cv.state {
	case cloneStateSelect:
		base := title + banner + "\n" + cv.viewSelect()
		if cv.selectMenuOpen {
			return base + "\n" + cv.viewSelectMenu()
		}
		return base
	case cloneStateDestDir:
		return title + banner + "\n" + cv.viewDestDir()
	case cloneStateRunning:
		var b strings.Builder
		b.WriteString("\n")
		for _, line := range cv.results {
			b.WriteString("  " + line + "\n")
		}
		if cv.currentCmd != "" {
			b.WriteString("\n  ▶ " + StyleHelp.Render(cv.currentCmd) + "\n")
		}
		return title + banner + b.String()
	case cloneStateDone:
		return title + banner + "\n\n" + cv.viewDone()
	}
	return ""
}

func (cv *CloneView) viewSelect() string {
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	clonedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	vh := cv.viewportHeight()
	end := cv.offset + vh
	if end > len(cv.repos) {
		end = len(cv.repos)
	}

	// Compute total selected size.
	var totalKB int
	for i, r := range cv.repos {
		if cv.selected[i] {
			totalKB += r.DiskUsage
		}
	}

	header := headerStyle.Render(fmt.Sprintf("     %-30s %-12s %-8s %-5s %-8s %-6s %s",
		"NAME", "LAST COMMIT", "VIS", "BR", "SIZE", "FORK", "HISTORY"))

	var rows strings.Builder
	for i := cv.offset; i < end; i++ {
		r := cv.repos[i]

		vis := "public"
		if r.IsPrivate {
			vis = "private"
		}
		fork := "no"
		if r.IsFork {
			fork = "yes"
		}

		// Clone history badge.
		history := ""
		if rec, ok := cv.cfg.FindClone(r.Name); ok {
			since := formatAge(rec.ClonedAt)
			mode := "default"
			if rec.AllBranches {
				mode = "all"
			}
			history = clonedStyle.Render(fmt.Sprintf("[C %s %s]", mode, since))
		}

		// Dir-exists warning (only if destDir is set).
		dest := filepath.Join(cv.destDir, r.Name)
		if _, err := os.Stat(dest); err == nil && history == "" {
			history = warnStyle.Render("[dir exists]")
		}

		checkbox := "[ ]"
		if cv.selected[i] {
			checkbox = selectedStyle.Render("[✓]")
		}
		cur := "  "
		if i == cv.cursor {
			cur = cursorStyle.Render("❯ ")
		}

		line := fmt.Sprintf("%s%s %-30s %-12s %-8s %-5d %-8s %-6s %s",
			cur, checkbox,
			truncate(r.Name, 30),
			formatAge(r.PushedAt),
			vis,
			r.BranchCount,
			gh.FormatSize(r.DiskUsage),
			fork,
			history,
		)

		switch {
		case r.IsFork:
			line = StyleFork.Render(line)
		case !r.IsPrivate:
			line = StylePublic.Render(line)
		}
		if i == cv.cursor {
			line = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("237")).Render(line)
		}

		rows.WriteString("\n" + line)
	}

	scrollInfo := fmt.Sprintf("%d-%d / %d", cv.offset+1, end, len(cv.repos))

	branchMode := "default branch"
	if cv.allBranches {
		branchMode = "all branches"
	}
	branchStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))

	var selInfo string
	if len(cv.selected) > 0 {
		selInfo = fmt.Sprintf("%d selected  total: %s", len(cv.selected), gh.FormatSize(totalKB))
	} else {
		selInfo = "0 selected"
	}

	status := StyleTitle.Render(selInfo) +
		"  " + StyleHelp.Render(scrollInfo) +
		"  clone: " + branchStyle.Render(branchMode)

	help := StyleHelp.Render("  space toggle  a select-group  b branch-mode  enter proceed  D dry-run  esc back")
	return "\n" + header + "\n" + status + rows.String() + "\n\n" + help
}

func (cv *CloneView) viewSelectMenu() string {
	menuStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(0, 2).
		Margin(0, 4)

	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Select group:") + "\n")
	for i, opt := range selectMenuOptions {
		cur := "  "
		label := opt.label
		if i == cv.selectMenuCur {
			cur = cursorStyle.Render("❯ ")
			label = cursorStyle.Render(opt.label)
		}
		b.WriteString("\n" + cur + label)
	}
	b.WriteString("\n\n" + StyleHelp.Render("↑/k up  ↓/j down  enter select  esc cancel"))
	return menuStyle.Render(b.String())
}

func (cv *CloneView) viewDestDir() string {
	prompt := fmt.Sprintf("\n  Destination directory:\n\n  %s█\n\n", cv.destDir)
	note := StyleHelp.Render("  Repos will be cloned into subdirectories of this path.")
	help := "\n" + StyleHelp.Render("  enter confirm  esc back")
	return prompt + note + help
}

func (cv *CloneView) viewDone() string {
	var b strings.Builder
	for _, r := range cv.results {
		b.WriteString("  " + r + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("  esc to go back"))
	return b.String()
}

