# gh-repo-inspector — TUI Plan

A terminal UI utility for inspecting and managing all of your GitHub repositories, built in Go using Bubble Tea.

---

## Goals

- Surface key metadata for every repo in one place (name, description, last commit, visibility, fork status, clone size)
- Provide focused review workflows for forks, public repos, and old repos — each with safe, one-at-a-time destructive actions
- Allow selective or bulk local cloning of repos
- Support a **dry-run mode** so all mutating actions can be previewed safely before execution
- Provide a **test setup guide** that walks the user through creating a small set of representative test repos to validate all workflows

---

## Stack

| Concern | Choice |
|---------|--------|
| Language | Go |
| GitHub access | Shell out to `gh` CLI (no token management needed) |
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Components | [Bubbles](https://github.com/charmbracelet/bubbles) (table, spinner, text input) |
| Confirmation UX | One action at a time — safer |
| Dry-run mode | CLI flag `--dry-run` / `-n` at launch + in-app toggle (`D` key) |

---

## Project Structure

```
gh-repo-inspector/
├── main.go
├── go.mod
├── go.sum
└── internal/
    ├── gh/
    │   └── client.go       # all gh CLI and git subprocess calls
    └── ui/
        ├── app.go          # root Bubble Tea model, navigation stack, dry-run state
        ├── menu.go         # main menu (5 phases + test setup option)
        ├── repolist.go     # reusable sortable/filterable repo table
        ├── confirm.go      # single-action confirmation dialog (dry-run aware)
        ├── clone.go        # multi-select clone picker + progress view
        └── testsetup.go    # step-by-step test repo creation guide
```

---

## Data Model

Repos are fetched **once on startup** and cached in memory for the session.

```go
// internal/gh/client.go

type Repo struct {
    Name        string
    Description string
    PushedAt    time.Time   // "last commit date"
    IsPrivate   bool
    IsFork      bool
    DiskUsage   int         // KB — displayed as human-readable size
    URL         string      // https://github.com/owner/name
}
```

Fetched via:
```bash
gh repo list --limit 1000 --json name,description,pushedAt,isPrivate,isFork,diskUsage,url
```

---

## Navigation Model

The root `app.go` model owns a **stack of `tea.Model` values**. Navigating to a phase pushes a new model; pressing `Esc`/`q` pops it (or quits from the main menu). Dry-run state is stored at the app level and passed down to all child models.

```
App (stack, dry-run flag)
 └── MainMenu
      ├── Phase 1: RepoList (read-only, all repos)
      ├── Phase 2: RepoList (forks only) → ConfirmDialog
      ├── Phase 3: RepoList (public only) → ConfirmDialog
      ├── Phase 4: RepoList (oldest first) → ConfirmDialog
      ├── Phase 5: CloneView → ConfirmDialog
      └── Test Setup: TestSetupView (step-by-step guide)
```

Window resize events are forwarded to the active model on the stack.

---

## Phases

### Phase 1 — Inspect All Repos
Full table of every repo. Read-only — no destructive actions.

**Columns:** Name · Description · Last Commit · Visibility · Fork? · Clone Size

### Phase 2 — Review Forks
Filtered to repos where `IsFork == true`.

**Action:** `d` → confirm dialog → `gh repo delete <name> --yes`

### Phase 3 — Review Public Repos
Filtered to repos where `IsPrivate == false`.

**Action:** `p` → confirm dialog → `gh repo edit <name> --visibility private`

### Phase 4 — Review by Age (Oldest First)
All repos sorted by `PushedAt` ascending (oldest activity first).

**Action:** `d` → confirm dialog → `gh repo delete <name> --yes`

### Phase 5 — Clone Repos
Multi-select list of all repos. On confirm: prompt for a local destination directory, then clone each selected repo sequentially with progress output.

**Actions:** `Space` toggle selection · `a` select all · `Enter` proceed to clone

---

## Dry-Run Mode

Applies to all mutating actions: **delete**, **make private**, and **clone**.

### Activation
- CLI flag: `--dry-run` or `-n` at launch
- In-app toggle: `D` key from any phase view (visible in the help bar)

### Behaviour
When dry-run is active:
- A **persistent banner** is shown at the top of every view: `⚠ DRY RUN — no changes will be made`
- The confirm dialog shows: `[DRY RUN] Would delete owner/repo-name` instead of the real prompt
- The `gh` / `git` calls are **skipped entirely**; a log entry is printed instead
- The repo list is **not refreshed** after a dry-run action (since nothing changed)

All UI flow (navigation, selection, confirmation steps) behaves identically to live mode so the full experience can be validated safely.

---

## Test Setup Guide

A dedicated menu option — **"Test Setup"** — that walks the user through creating a small, representative set of repos to use when testing all workflows.

Presented as a **paginated checklist** inside the TUI. Each step shows the exact `gh` command to run in a separate terminal, then the user presses `Enter` to mark it done and advance.

### Recommended test repos to create

| Step | What to create | Purpose |
|------|---------------|---------|
| 1 | A public repo `test-public-alpha` with a description | Tests Phase 1 display, Phase 3 (make private) |
| 2 | A public repo `test-public-beta` with no description | Tests Phase 3 with missing description |
| 3 | A private repo `test-private-one` | Tests Phase 1 visibility display |
| 4 | Fork any public repo (e.g., `gh repo fork cli/cli --clone=false`) | Tests Phase 2 (fork review + delete) |
| 5 | A repo with no commits for > 1 year (create with a past date or just an old one) | Tests Phase 4 age sort |
| 6 | Any repo to test cloning | Tests Phase 5 |

Each step includes:
- The `gh` CLI command to run
- A brief explanation of what it tests
- A `[✓ Done]` / `[ ] Pending` status the user toggles with `Space` or `Enter`

The guide does **not** create repos automatically — it instructs the user and tracks their progress through the checklist.

---

## Keybindings

Consistent across all views:

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `/` | Filter / search by name |
| `Enter` | Select / confirm |
| `Space` | Toggle selection (Phase 5) |
| `a` | Select all (Phase 5) |
| `d` | Delete repo (Phases 2, 4) |
| `p` | Make private (Phase 3) |
| `c` | Open clone view (Phase 5) |
| `D` | Toggle dry-run mode (all mutating phases) |
| `Esc` / `q` | Back / quit |

A **context-sensitive help bar** at the bottom of each view shows only the keys relevant to that phase.

---

## Visual Design

| Item | Style |
|------|-------|
| Fork repos | Yellow text |
| Public repos | Orange text |
| Private repos | Default/muted |
| Large repos (> 500 MB) | Red disk usage column |
| Selected row | Highlighted background |
| Loading state | Spinner with "Fetching repos…" message |
| Error state | Red banner at top of screen |
| Dry-run active | Persistent yellow banner: `⚠ DRY RUN — no changes will be made` |
| Test setup checklist | `[✓]` green / `[ ]` muted per step |

---

## `gh` CLI Calls

```go
// List all repos
gh repo list --limit 1000 --json name,description,pushedAt,isPrivate,isFork,diskUsage,url

// Delete a repo
gh repo delete <owner>/<name> --yes

// Change visibility to private
gh repo edit <owner>/<name> --visibility private

// Clone a repo
git clone <url> <destination>/<name>
```

All calls are wrapped in `internal/gh/client.go` and return typed results or errors.

---

## Implementation Order

The table below shows each task, what it depends on, and a brief description.

| # | ID | Title | Depends On |
|---|----|-------|-----------|
| 1 | `scaffold` | Scaffold Go module + deps | — |
| 2 | `gh-client` | `internal/gh/client.go` | scaffold |
| 3 | `ui-app` | Root Bubble Tea model + nav stack + dry-run state | scaffold |
| 4 | `ui-menu` | Main menu (5 phases + Test Setup option) | ui-app |
| 5 | `ui-repolist` | Reusable repo table component | gh-client |
| 6 | `ui-confirm` | Confirmation dialog (dry-run aware) | ui-app |
| 7 | `phase1` | Phase 1 wired end-to-end | ui-menu, ui-repolist |
| 8 | `phase2` | Phase 2: fork review + delete | phase1, ui-confirm |
| 9 | `phase3` | Phase 3: public → private | phase1, ui-confirm |
| 10 | `phase4` | Phase 4: oldest-first + delete | phase1, ui-confirm |
| 11 | `ui-clone` | Clone picker + progress view (dry-run aware) | gh-client |
| 12 | `phase5` | Phase 5: clone repos | ui-clone, ui-menu |
| 13 | `ui-testsetup` | Test setup guide (paginated checklist) | ui-app |
| 14 | `dryrun` | Dry-run mode: CLI flag, in-app toggle, banner, skip logic | ui-confirm, ui-clone |
| 15 | `polish` | Spinner, error banners, help bar | phase5, ui-testsetup, dryrun |
| 16 | `update-instructions` | Update `.github/copilot-instructions.md` | polish |

---

## Commands (to be filled in after scaffold)

```bash
# Install dependencies
go mod tidy

# Build
go build -o gh-repo-inspector .

# Run
./gh-repo-inspector

# Run tests
go test ./...

# Run a single package's tests
go test ./internal/gh/...
```
