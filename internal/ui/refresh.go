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
	refreshStateSelect  refreshState = iota // choosing repos
	refreshStateRunning                     // pulling in progress
	refreshStateDone                        // finished
)

// RefreshView lets the user select previously cloned repos and run git pull on them.
type RefreshView struct {
	app            *App
	records        []config.CloneRecord
	selected       map[int]bool
	cursor         int
	offset         int
	height         int
	state          refreshState
	results        []string
	pendingRecords []config.CloneRecord
	currentCmd     string
	cfg            *config.Config
	selectMenuOpen bool
	selectMenuCur  int
}

type refreshMenuOption struct {
	label  string
	filter func(r config.CloneRecord) bool
}

var refreshMenuOptions = []refreshMenuOption{
	{"All", func(r config.CloneRecord) bool { return true }},
	{"Dir exists (pullable)", func(r config.CloneRecord) bool {
		_, err := os.Stat(r.Path)
		return err == nil
	}},
	{"Dir missing", func(r config.CloneRecord) bool {
		_, err := os.Stat(r.Path)
		return err != nil
	}},
	{"Clear selection", nil},
}

func NewRefreshView(app *App) *RefreshView {
	cfg, _ := config.Load()
	// Show most-recently-cloned first.
	records := make([]config.CloneRecord, len(cfg.CloneHistory))
	for i, r := range cfg.CloneHistory {
		records[len(cfg.CloneHistory)-1-i] = r
	}
	return &RefreshView{
		app:      app,
		records:  records,
		selected: make(map[int]bool),
		height:   app.height,
		cfg:      cfg,
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

// pullProgressMsg is sent when one git pull completes.
type pullProgressMsg struct {
	result  string
	nextIdx int
}

func (rv *RefreshView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pullProgressMsg:
		if msg.result != "" {
			rv.results = append(rv.results, msg.result)
		}
		if msg.nextIdx >= len(rv.pendingRecords) {
			rv.currentCmd = ""
			rv.state = refreshStateDone
			return rv, nil
		}
		r := rv.pendingRecords[msg.nextIdx]
		rv.currentCmd = "git -C " + r.Path + " pull"
		return rv, rv.pullOne(msg.nextIdx)

	case tea.WindowSizeMsg:
		rv.height = msg.Height
		rv.clampOffset()

	case tea.KeyMsg:
		switch rv.state {
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

func (rv *RefreshView) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if rv.selectMenuOpen {
		return rv.updateSelectMenu(msg)
	}
	switch msg.String() {
	case "esc", "q":
		return rv, PopScreen()
	case "up", "k":
		if rv.cursor > 0 {
			rv.cursor--
			rv.clampOffset()
		}
	case "down", "j":
		if rv.cursor < len(rv.records)-1 {
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
		if rv.selectMenuCur < len(refreshMenuOptions)-1 {
			rv.selectMenuCur++
		}
	case "enter", " ":
		opt := refreshMenuOptions[rv.selectMenuCur]
		rv.selected = make(map[int]bool)
		if opt.filter != nil {
			for i, r := range rv.records {
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
	rv.pendingRecords = rv.selectedRecords()
	rv.results = nil
	rv.currentCmd = ""
	if len(rv.pendingRecords) == 0 {
		rv.state = refreshStateDone
		return nil
	}
	rv.currentCmd = "git -C " + rv.pendingRecords[0].Path + " pull"
	return rv.pullOne(0)
}

func (rv *RefreshView) pullOne(idx int) tea.Cmd {
	r := rv.pendingRecords[idx]
	dryRun := rv.app.DryRun
	cfg := rv.cfg
	next := idx + 1

	return func() tea.Msg {
		if _, err := os.Stat(r.Path); err != nil {
			line := fmt.Sprintf("⚠  %s — directory not found at %s, skipping", r.Name, r.Path)
			return pullProgressMsg{result: line, nextIdx: next}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		err := gh.PullRepo(ctx, r.Path, dryRun)
		var line string
		if dryRun {
			line = fmt.Sprintf("[DRY RUN] git -C %s pull", r.Path)
		} else if err != nil {
			line = fmt.Sprintf("✗ %s: %v", r.Name, err)
		} else {
			line = fmt.Sprintf("✓ %s  (%s)", r.Name, r.Path)
			cfg.RecordPull(r.Name)
			_ = cfg.Save()
		}
		return pullProgressMsg{result: line, nextIdx: next}
	}
}

func (rv *RefreshView) selectedRecords() []config.CloneRecord {
	var out []config.CloneRecord
	for i, r := range rv.records {
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

func (rv *RefreshView) viewSelect() string {
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	missingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	pulledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	if len(rv.records) == 0 {
		return "\n  No clone history found. Use Clone Repos first.\n\n" +
			StyleHelp.Render("  esc back")
	}

	vh := rv.viewportHeight()
	end := rv.offset + vh
	if end > len(rv.records) {
		end = len(rv.records)
	}

	header := headerStyle.Render(fmt.Sprintf("     %-28s %-38s %-8s %-10s %s",
		"NAME", "PATH", "BRANCHES", "CLONED", "LAST PULL"))

	var rows strings.Builder
	for i := rv.offset; i < end; i++ {
		r := rv.records[i]

		branches := "default"
		if r.AllBranches {
			branches = "all"
		}

		_, statErr := os.Stat(r.Path)
		dirMissing := statErr != nil

		var pulledInfo string
		if r.LastPulledAt != nil {
			pulledInfo = pulledStyle.Render(formatAge(*r.LastPulledAt))
		} else {
			pulledInfo = StyleHelp.Render("never")
		}

		checkbox := "[ ]"
		if rv.selected[i] {
			checkbox = selectedStyle.Render("[✓]")
		}
		cur := "  "
		if i == rv.cursor {
			cur = cursorStyle.Render("❯ ")
		}

		namePart := truncate(r.Name, 28)
		if dirMissing {
			namePart = missingStyle.Render(namePart + " ✗")
		}

		line := fmt.Sprintf("%s%s %-28s %-38s %-8s %-10s %s",
			cur, checkbox,
			namePart,
			truncate(r.Path, 38),
			branches,
			formatAge(r.ClonedAt),
			pulledInfo,
		)

		if i == rv.cursor {
			line = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("237")).Render(line)
		}
		rows.WriteString("\n" + line)
	}

	scrollInfo := fmt.Sprintf("%d-%d / %d", rv.offset+1, end, len(rv.records))
	selInfo := fmt.Sprintf("%d selected", len(rv.selected))
	status := StyleTitle.Render(selInfo) + "  " + StyleHelp.Render(scrollInfo)

	help := StyleHelp.Render("  space toggle  a select-group  enter pull selected  D dry-run  esc back")
	note := StyleHelp.Render("  ✗ = directory missing on disk")
	return "\n" + header + "\n" + status + rows.String() + "\n\n" + note + "\n" + help
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
	for i, opt := range refreshMenuOptions {
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
