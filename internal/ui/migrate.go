package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/al/gh-repo-inspector/internal/config"
	"github.com/al/gh-repo-inspector/internal/gh"
	"github.com/al/gh-repo-inspector/internal/gitlab"
	"github.com/al/gh-repo-inspector/internal/oplog"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type migrateState int

const (
	migrateStateConfig    migrateState = iota // enter GitLab URL + PAT
	migrateStateDir                           // enter local directory to scan
	migrateStateScanning                      // scanning for git repos
	migrateStateSelect                        // multi-select repos to push
	migrateStateChecking                      // checking which repos already exist on GitLab
	migrateStateConflict                      // per-repo conflict resolution
	migrateStateRunning                       // streaming git push progress
	migrateStateDone                          // summary
)

// configField tracks which text field is active in the config screen.
type configField int

const (
	fieldURL   configField = iota
	fieldToken             // PAT
)

type conflictRepo struct {
	name      string
	localPath string
	isPrivate bool
	desc      string
}

// migrateCheckMsg is sent after batch-checking selected repos against GitLab.
type migrateCheckMsg struct {
	conflicts []conflictRepo // repos that already exist
	err       error
}

// migratePushMsg is sent when one push completes.
type migratePushMsg struct {
	result  string
	nextIdx int
}

// MigrateView pushes locally cloned repos to a local GitLab instance.
type MigrateView struct {
	app          *App
	cfg          *config.Config
	state        migrateState
	activeField  configField
	gitLabURL    string
	gitLabToken  string
	gitLabUser   string // fetched from /api/v4/user
	scanDir      string
	repos        []gh.LocalRepo
	selected     map[int]bool
	cursor       int
	offset       int
	height       int
	scanErr      error
	conflicts    []conflictRepo
	conflictIdx  int
	forceDecisions map[string]bool // repo name → force push?
	pendingRepos []gh.LocalRepo
	results      []string
	currentCmd   string
	configErr    string

	// ghRepos maps repo name → IsPrivate for visibility mirroring.
	ghRepos map[string]bool

	selectMenuOpen bool
	selectMenuCur  int
}

type migrateSelectOption struct {
	label  string
	filter func(r gh.LocalRepo) bool
}

var migrateSelectOptions = []migrateSelectOption{
	{"All", func(r gh.LocalRepo) bool { return true }},
	{"Clear selection", nil},
}

func NewMigrateView(app *App, ghRepoList []gh.Repo) *MigrateView {
	cfg, _ := config.Load()
	dir, _ := os.UserHomeDir()
	if cfg.LastMigratePath != "" {
		dir = cfg.LastMigratePath
	} else if cfg.LastClonePath != "" {
		dir = cfg.LastClonePath
	}
	// Build name → isPrivate lookup from GitHub repo list.
	ghMap := make(map[string]bool, len(ghRepoList))
	for _, r := range ghRepoList {
		ghMap[r.Name] = r.IsPrivate
	}
	return &MigrateView{
		app:            app,
		cfg:            cfg,
		state:          migrateStateConfig,
		activeField:    fieldURL,
		gitLabURL:      cfg.GitLabURL,
		gitLabToken:    cfg.GitLabToken,
		scanDir:        dir,
		selected:       make(map[int]bool),
		forceDecisions: make(map[string]bool),
		ghRepos:        ghMap,
		height:         app.height,
	}
}

func (mv *MigrateView) Init() tea.Cmd { return nil }

func (mv *MigrateView) viewportHeight() int {
	h := mv.height - 8
	if h < 1 {
		h = 18
	}
	return h
}

func (mv *MigrateView) clampOffset() {
	vh := mv.viewportHeight()
	if mv.cursor < mv.offset {
		mv.offset = mv.cursor
	}
	if mv.cursor >= mv.offset+vh {
		mv.offset = mv.cursor - vh + 1
	}
	if mv.offset < 0 {
		mv.offset = 0
	}
}

func (mv *MigrateView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		mv.height = msg.Height
		mv.clampOffset()

	case scanMsg:
		mv.scanErr = msg.err
		mv.repos = msg.repos
		mv.selected = make(map[int]bool)
		mv.cursor = 0
		mv.offset = 0
		mv.state = migrateStateSelect
		return mv, nil

	case migrateCheckMsg:
		if msg.err != nil {
			mv.configErr = msg.err.Error()
			mv.state = migrateStateSelect
			return mv, nil
		}
		mv.conflicts = msg.conflicts
		mv.conflictIdx = 0
		mv.forceDecisions = make(map[string]bool)
		if len(mv.conflicts) > 0 {
			mv.state = migrateStateConflict
		} else {
			mv.state = migrateStateRunning
			return mv, mv.startMigrating()
		}
		return mv, nil

	case migratePushMsg:
		if msg.result != "" {
			mv.results = append(mv.results, msg.result)
		}
		if msg.nextIdx >= len(mv.pendingRepos) {
			mv.currentCmd = ""
			mv.state = migrateStateDone
			return mv, nil
		}
		r := mv.pendingRepos[msg.nextIdx]
		mv.currentCmd = "git push --all gitlab  [" + r.Name + "]"
		return mv, mv.pushOne(msg.nextIdx)

	case tea.KeyMsg:
		switch mv.state {
		case migrateStateConfig:
			return mv.updateConfig(msg)
		case migrateStateDir:
			return mv.updateDir(msg)
		case migrateStateScanning, migrateStateChecking:
			// block input
		case migrateStateSelect:
			return mv.updateSelect(msg)
		case migrateStateConflict:
			return mv.updateConflict(msg)
		case migrateStateDone:
			if msg.String() == "esc" || msg.String() == "q" {
				return mv, PopScreen()
			}
		}
	}
	return mv, nil
}

// ---------- Config screen ----------

func (mv *MigrateView) updateConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return mv, PopScreen()
	case "tab", "shift+tab":
		if mv.activeField == fieldURL {
			mv.activeField = fieldToken
		} else {
			mv.activeField = fieldURL
		}
	case "enter":
		if mv.activeField == fieldURL {
			mv.activeField = fieldToken
			return mv, nil
		}
		// Validate and save.
		if strings.TrimSpace(mv.gitLabURL) == "" || strings.TrimSpace(mv.gitLabToken) == "" {
			mv.configErr = "Both GitLab URL and token are required."
			return mv, nil
		}
		mv.configErr = ""
		mv.cfg.GitLabURL = strings.TrimRight(strings.TrimSpace(mv.gitLabURL), "/")
		mv.cfg.GitLabToken = strings.TrimSpace(mv.gitLabToken)
		_ = mv.cfg.Save()
		mv.gitLabURL = mv.cfg.GitLabURL
		mv.gitLabToken = mv.cfg.GitLabToken
		mv.state = migrateStateDir
		return mv, nil
	case "backspace":
		if mv.activeField == fieldURL && len(mv.gitLabURL) > 0 {
			mv.gitLabURL = mv.gitLabURL[:len(mv.gitLabURL)-1]
		} else if mv.activeField == fieldToken && len(mv.gitLabToken) > 0 {
			mv.gitLabToken = mv.gitLabToken[:len(mv.gitLabToken)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			ch := string(msg.Runes)
			if mv.activeField == fieldURL {
				mv.gitLabURL += ch
			} else {
				mv.gitLabToken += ch
			}
		}
	}
	return mv, nil
}

// ---------- Directory screen ----------

func (mv *MigrateView) updateDir(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		mv.state = migrateStateConfig
	case "enter":
		mv.state = migrateStateScanning
		mv.cfg.LastMigratePath = strings.TrimSpace(mv.scanDir)
		_ = mv.cfg.Save()
		return mv, doScan(mv.scanDir)
	case "backspace":
		if len(mv.scanDir) > 0 {
			mv.scanDir = mv.scanDir[:len(mv.scanDir)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			mv.scanDir += string(msg.Runes)
		}
	}
	return mv, nil
}

// ---------- Select screen ----------

func (mv *MigrateView) updateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mv.selectMenuOpen {
		return mv.updateSelectMenu(msg)
	}
	switch msg.String() {
	case "esc", "q":
		mv.state = migrateStateDir
		mv.repos = nil
		mv.selected = make(map[int]bool)
	case "up", "k":
		if mv.cursor > 0 {
			mv.cursor--
			mv.clampOffset()
		}
	case "down", "j":
		if mv.cursor < len(mv.repos)-1 {
			mv.cursor++
			mv.clampOffset()
		}
	case " ":
		mv.selected[mv.cursor] = !mv.selected[mv.cursor]
	case "a":
		mv.selectMenuOpen = true
		mv.selectMenuCur = 0
	case "enter":
		if len(mv.selected) == 0 {
			return mv, nil
		}
		mv.configErr = ""
		mv.state = migrateStateChecking
		return mv, mv.checkConflicts()
	case "D":
		mv.app.DryRun = !mv.app.DryRun
	}
	return mv, nil
}

func (mv *MigrateView) updateSelectMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		mv.selectMenuOpen = false
	case "up", "k":
		if mv.selectMenuCur > 0 {
			mv.selectMenuCur--
		}
	case "down", "j":
		if mv.selectMenuCur < len(migrateSelectOptions)-1 {
			mv.selectMenuCur++
		}
	case "enter", " ":
		opt := migrateSelectOptions[mv.selectMenuCur]
		mv.selected = make(map[int]bool)
		if opt.filter != nil {
			for i, r := range mv.repos {
				if opt.filter(r) {
					mv.selected[i] = true
				}
			}
		}
		mv.selectMenuOpen = false
	}
	return mv, nil
}

// checkConflicts fires a tea.Cmd that queries GitLab for each selected repo.
func (mv *MigrateView) checkConflicts() tea.Cmd {
	selected := mv.selectedRepos()
	glURL := mv.gitLabURL
	token := mv.gitLabToken

	return func() tea.Msg {
		gl := gitlab.New(glURL, token)
		username, err := gl.GetCurrentUser()
		if err != nil {
			msg := fmt.Sprintf("Cannot reach GitLab (%s): %v", glURL, err)
			oplog.Write("GITLAB-CHECK", glURL, msg)
			return migrateCheckMsg{err: fmt.Errorf("%s", msg)}
		}

		var conflicts []conflictRepo
		for _, r := range selected {
			_, exists, err := gl.CheckRepo(username, r.Name)
			if err != nil {
				oplog.Write("GITLAB-CHECK", r.Name, fmt.Sprintf("check error: %v", err))
				continue
			}
			if exists {
				conflicts = append(conflicts, conflictRepo{
					name:      r.Name,
					localPath: r.Path,
				})
			}
		}
		return migrateCheckMsg{conflicts: conflicts}
	}
}

// ---------- Conflict screen ----------

func (mv *MigrateView) updateConflict(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		mv.state = migrateStateSelect
		return mv, nil
	case "s", "S", "n", "N":
		// Skip this repo
		mv.forceDecisions[mv.conflicts[mv.conflictIdx].name] = false
		mv.conflictIdx++
	case "f", "F", "y", "Y":
		// Force push
		mv.forceDecisions[mv.conflicts[mv.conflictIdx].name] = true
		mv.conflictIdx++
	}
	if mv.conflictIdx >= len(mv.conflicts) {
		mv.state = migrateStateRunning
		return mv, mv.startMigrating()
	}
	return mv, nil
}

// ---------- Push phase ----------

func (mv *MigrateView) selectedRepos() []gh.LocalRepo {
	var out []gh.LocalRepo
	for i, r := range mv.repos {
		if mv.selected[i] {
			out = append(out, r)
		}
	}
	return out
}

func (mv *MigrateView) startMigrating() tea.Cmd {
	selected := mv.selectedRepos()
	// Filter out repos the user chose to skip.
	var pending []gh.LocalRepo
	for _, r := range selected {
		if force, decided := mv.forceDecisions[r.Name]; decided && !force {
			continue // user said skip
		}
		pending = append(pending, r)
	}
	mv.pendingRepos = pending
	mv.results = nil
	mv.currentCmd = ""
	if len(mv.pendingRepos) == 0 {
		mv.state = migrateStateDone
		return nil
	}
	mv.currentCmd = "git push --all gitlab  [" + mv.pendingRepos[0].Name + "]"
	return mv.pushOne(0)
}

func (mv *MigrateView) pushOne(idx int) tea.Cmd {
	r := mv.pendingRepos[idx]
	force := mv.forceDecisions[r.Name]
	dryRun := mv.app.DryRun
	glURL := mv.gitLabURL
	glToken := mv.gitLabToken
	isPrivate := mv.ghRepos[r.Name] // false (public) if not found in GitHub list
	next := idx + 1

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		gl := gitlab.New(glURL, glToken)

		// Get (or re-use cached) username.
		username, err := gl.GetCurrentUser()
		if err != nil {
			return migratePushMsg{
				result:  fmt.Sprintf("✗ %s: cannot reach GitLab: %v", r.Name, err),
				nextIdx: next,
			}
		}

		// Ensure project exists on GitLab.
		_, exists, _ := gl.CheckRepo(username, r.Name)
		var remoteURL string
		if !exists {
			_, err := gl.CreateRepo(r.Name, "", isPrivate)
			if err != nil {
				oplog.Write("GITLAB-PUSH", r.Name, fmt.Sprintf("create failed: %v", err))
				return migratePushMsg{
					result:  fmt.Sprintf("✗ %s: create failed: %v", r.Name, err),
					nextIdx: next,
				}
			}
		}
		remoteURL = gl.RepoHTTPURL(username, r.Name)
		cleanURL := gl.CleanRepoURL(username, r.Name)

		cmdStr, err := gh.PushToRemote(ctx, r.Path, remoteURL, cleanURL, force, dryRun)
		if err != nil {
			oplog.Write("GITLAB-PUSH", r.Name, fmt.Sprintf("push error: %v", err))
			return migratePushMsg{
				result:  fmt.Sprintf("✗ %s: %v", r.Name, err),
				nextIdx: next,
			}
		}
		oplog.Write("GITLAB-PUSH", r.Name, "ok → "+cleanURL)
		visibility := "public"
		if isPrivate {
			visibility = "private"
		}
		label := fmt.Sprintf("✓ %s  →  %s/%s  (%s)", r.Name, glURL, username, visibility)
		if dryRun {
			label = "[DRY RUN] " + cmdStr
		}
		return migratePushMsg{result: label, nextIdx: next}
	}
}

// ---------- Views ----------

func (mv *MigrateView) View() string {
	title := StyleTitle.Render("Migrate to GitLab")
	var banner string
	if mv.app.DryRun {
		banner = "\n" + StyleDryRunBanner.Render("⚠  DRY RUN — no changes will be made")
	}

	switch mv.state {
	case migrateStateConfig:
		return title + banner + "\n" + mv.viewConfig()
	case migrateStateDir:
		return title + banner + "\n" + mv.viewDir()
	case migrateStateScanning:
		return title + banner + fmt.Sprintf("\n\n  Scanning %s…", mv.scanDir)
	case migrateStateChecking:
		return title + banner + "\n\n  Checking repos against GitLab…"
	case migrateStateSelect:
		base := title + banner + "\n" + mv.viewSelect()
		if mv.selectMenuOpen {
			return base + "\n" + mv.viewSelectMenu()
		}
		return base
	case migrateStateConflict:
		return title + banner + "\n" + mv.viewConflict()
	case migrateStateRunning:
		return title + banner + "\n" + mv.viewRunning()
	case migrateStateDone:
		return title + banner + "\n\n" + mv.viewDone()
	}
	return ""
}

func (mv *MigrateView) viewConfig() string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	urlLabel := labelStyle.Render("GitLab URL")
	tokenLabel := labelStyle.Render("Personal Access Token")
	if mv.activeField == fieldURL {
		urlLabel = activeStyle.Render("GitLab URL")
	} else {
		tokenLabel = activeStyle.Render("Personal Access Token")
	}

	urlCursor := ""
	tokenCursor := ""
	if mv.activeField == fieldURL {
		urlCursor = "█"
	} else {
		tokenCursor = "█"
	}

	// Mask the token except last 4 chars.
	tokenDisplay := maskToken(mv.gitLabToken) + tokenCursor

	var errLine string
	if mv.configErr != "" {
		errLine = "\n  " + errStyle.Render("✗ "+mv.configErr)
	}

	setup := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(
		"  How to get a token:\n" +
			"  1. Open " + mv.gitLabURL + " in a browser\n" +
			"  2. Avatar → Edit profile → Access Tokens\n" +
			"  3. Create token with scopes: api, write_repository\n" +
			"  4. Paste the token below")

	return fmt.Sprintf(
		"\n%s\n\n  %s\n  %s%s\n\n  %s\n  %s\n%s\n\n%s",
		setup,
		urlLabel, mv.gitLabURL, urlCursor,
		tokenLabel, tokenDisplay,
		errLine,
		StyleHelp.Render("  tab switch field  enter next/confirm  esc back"),
	)
}

func (mv *MigrateView) viewDir() string {
	prompt := fmt.Sprintf("\n  Directory containing cloned repos:\n\n  %s█\n\n", mv.scanDir)
	note := StyleHelp.Render("  Scans immediate subdirectories for .git folders.")
	help := "\n" + StyleHelp.Render("  enter scan  esc back")
	return prompt + note + help
}

func (mv *MigrateView) viewSelect() string {
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	if mv.scanErr != nil {
		return fmt.Sprintf("\n  Error scanning %s:\n  %v\n\n%s",
			mv.scanDir, mv.scanErr, StyleHelp.Render("  esc to try a different directory"))
	}
	if len(mv.repos) == 0 {
		return fmt.Sprintf("\n  No git repos found in %s\n\n%s",
			mv.scanDir, StyleHelp.Render("  esc to try a different directory"))
	}

	vh := mv.viewportHeight()
	end := mv.offset + vh
	if end > len(mv.repos) {
		end = len(mv.repos)
	}

	header := headerStyle.Render(fmt.Sprintf("     %-28s %-16s %s",
		"NAME", "BRANCH", "LAST COMMIT"))

	var rows strings.Builder
	for i := mv.offset; i < end; i++ {
		r := mv.repos[i]
		branch := r.Branch
		if branch == "" {
			branch = "?"
		}
		lastCommit := r.LastCommit
		if lastCommit == "" {
			lastCommit = "unknown"
		}
		checkbox := "[ ]"
		if mv.selected[i] {
			checkbox = selectedStyle.Render("[✓]")
		}
		cur := "  "
		if i == mv.cursor {
			cur = cursorStyle.Render("❯ ")
		}
		line := fmt.Sprintf("%s%s %-28s %-16s %s",
			cur, checkbox, truncate(r.Name, 28), branch, lastCommit)
		if i == mv.cursor {
			line = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("237")).Render(line)
		}
		rows.WriteString("\n" + line)
	}

	scrollInfo := fmt.Sprintf("%d-%d / %d  in %s", mv.offset+1, end, len(mv.repos), mv.scanDir)
	selInfo := fmt.Sprintf("%d selected", len(mv.selected))
	status := StyleTitle.Render(selInfo) + "  " + StyleHelp.Render(scrollInfo)

	var errLine string
	if mv.configErr != "" {
		errLine = "\n  " + errStyle.Render("✗ "+mv.configErr)
	}

	help := StyleHelp.Render("  space toggle  a select-all  enter push selected  D dry-run  esc rescan")
	return "\n" + header + "\n" + status + rows.String() + errLine + "\n\n" + help
}

func (mv *MigrateView) viewSelectMenu() string {
	menuStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(0, 2).
		Margin(0, 4)
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Select group:") + "\n")
	for i, opt := range migrateSelectOptions {
		cur := "  "
		label := opt.label
		if i == mv.selectMenuCur {
			cur = cursorStyle.Render("❯ ")
			label = cursorStyle.Render(opt.label)
		}
		b.WriteString("\n" + cur + label)
	}
	b.WriteString("\n\n" + StyleHelp.Render("↑/k up  ↓/j down  enter select  esc cancel"))
	return menuStyle.Render(b.String())
}

func (mv *MigrateView) viewConflict() string {
	if mv.conflictIdx >= len(mv.conflicts) {
		return ""
	}
	c := mv.conflicts[mv.conflictIdx]
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	remaining := len(mv.conflicts) - mv.conflictIdx

	return fmt.Sprintf(
		"\n  %s\n\n  Repo %q already exists on GitLab.\n  (%d repo(s) to resolve)\n\n"+
			"  What should we do?\n\n"+
			"  %s  skip this repo\n"+
			"  %s  force push (overwrites remote)\n\n%s",
		warnStyle.Render("⚠  Conflict"),
		c.name,
		remaining,
		StyleTitle.Render("s"),
		StyleTitle.Render("f"),
		StyleHelp.Render("  esc cancel migration"),
	)
}

func (mv *MigrateView) viewRunning() string {
	var b strings.Builder
	b.WriteString("\n")
	for _, line := range mv.results {
		b.WriteString("  " + line + "\n")
	}
	if mv.currentCmd != "" {
		b.WriteString("\n  ▶ " + StyleHelp.Render(mv.currentCmd) + "\n")
	}
	return b.String()
}

func (mv *MigrateView) viewDone() string {
	var ok, fail int
	for _, r := range mv.results {
		if strings.HasPrefix(r, "✓") {
			ok++
		} else {
			fail++
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  Migration complete: %d succeeded, %d failed\n\n", ok, fail))
	for _, r := range mv.results {
		b.WriteString("  " + r + "\n")
	}
	b.WriteString("\n" + StyleHelp.Render("  esc to go back"))
	return b.String()
}

// maskToken shows only the last 4 chars of a token for display.
func maskToken(t string) string {
	if len(t) <= 4 {
		return strings.Repeat("*", len(t))
	}
	return strings.Repeat("*", len(t)-4) + t[len(t)-4:]
}
