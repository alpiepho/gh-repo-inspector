# gh-repo-inspector

A terminal UI for inspecting and managing all your GitHub repositories.  
Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

---

## Features

| Option | What it does |
|--------|-------------|
| **1 – Inspect** | Browse all repos: name, description, last commit, visibility, fork status, branch count, disk size |
| **2 – Forks** | Review forks and selectively delete them |
| **3 – Public → Private** | Review public repos and change visibility to private |
| **4 – Private → Public** | Review private repos and change visibility to public |
| **5 – By Age** | Browse repos oldest-first and selectively delete them |
| **6 – Clone** | Multi-select repos and clone locally; track what's already cloned |
| **7 – Refresh Clones** | Scan a local directory for git repos and run `git pull` on selected ones |
| **8 – Test Setup** | Step-by-step guide for creating test repos via the `gh` CLI |

### Clone extras
- Select by group: All / Forks only / Public (non-fork) / Private (non-fork) / Clear
- Toggle between cloning the default branch only (`--single-branch`) or all branches
- Live progress: shows the exact `git clone` command as each repo clones
- 10-minute per-repo timeout so a stalled clone never hangs the UI
- Remembers the last destination path across sessions
- Tracks clone history (date, path, branch mode) and highlights already-cloned repos

### Dry-run mode
All mutating operations (delete, make-private, make-public, clone) can be previewed without side effects:

```
gh-repo-inspector --dry-run
```

Press `D` at any time inside the app to toggle dry-run on/off.

---

## Prerequisites

- **Go** ≥ 1.21
- **gh CLI** authenticated (`gh auth login`)
- **git** on your PATH

---

## Install

```bash
git clone https://github.com/al/gh-repo-inspector
cd gh-repo-inspector
go build -o gh-repo-inspector .
```

Or run without installing:

```bash
go run . [--dry-run]
```

---

## Usage

```
gh-repo-inspector [--dry-run | -n]
```

### Main menu key bindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `0` | Reload repo list from GitHub |
| `1`–`8` | Jump directly to that option |
| `Enter` | Select |
| `q` | Quit |

### Global key bindings (inside any view)

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Esc` | Back |
| `D` | Toggle dry-run mode |

### Inspect (option 1)

| Key | Action |
|-----|--------|
| `s` | Cycle sort column |
| `f` | Toggle filter (all / forks / public / private) |

### Review Forks / By Age (options 2, 5)

| Key | Action |
|-----|--------|
| `d` | Delete selected repo |

### Review Public / Private repos (options 3, 4)

| Key | Action |
|-----|--------|
| `p` | Make selected repo private / public |

### Clone (option 6)

| Key | Action |
|-----|--------|
| `Space` | Toggle selection |
| `a` | Open group-select menu |
| `b` | Toggle branch mode (default / all) |
| `Enter` | Proceed to destination prompt |

### Refresh Clones (option 7)

| Key | Action |
|-----|--------|
| `Enter` | Confirm directory / start scan |
| `Space` | Toggle repo selection |
| `a` | Select all / deselect all |
| `p` | Start pull on selected repos |

---

## Architecture

```
internal/gh/client.go      — all gh CLI / git subprocess calls
internal/config/config.go  — persists last clone path & clone history
internal/oplog/oplog.go    — append-only operations log (./operations.log)
internal/ui/app.go         — Bubble Tea root model; navigation stack
internal/ui/menu.go        — main menu with number shortcuts
internal/ui/repolist.go    — shared sortable/filterable repo table (options 1–5)
internal/ui/confirm.go     — dry-run-aware confirmation dialog
internal/ui/clone.go       — multi-select clone picker with live progress
internal/ui/refresh.go     — directory-scan git pull view
internal/ui/testsetup.go   — paginated test-repo setup guide
internal/ui/styles.go      — shared Lip Gloss styles and navigation commands
main.go                    — CLI entry point
```

---

## Development

```bash
# Install dependencies
go mod tidy

# Build
go build -o gh-repo-inspector .

# Run tests
go test ./...
```

---

## Troubleshooting

### Deleting repos fails with HTTP 403 / "delete_repo scope"

```
error: gh repo delete: HTTP 403: Must have admin rights to Repository.
This API operation needs the "delete_repo" scope. To request it, run:
  gh auth refresh -h github.com -s delete_repo
```

The `gh` CLI token does not include the `delete_repo` permission by default. Fix it once:

```bash
gh auth refresh -h github.com -s delete_repo
```

A browser window will open to confirm. After that, deletes will work permanently.

> **Note:** You can only delete repos you own or where you have admin rights. Repos owned
> by another account or organisation will still fail with 403 even after refreshing the scope.

---

### Making a repo private/public fails with "accept-visibility-change-consequences"

Older versions of `gh` (< 2.4) do not support the
`--accept-visibility-change-consequences` flag. Upgrade `gh`:

```bash
brew upgrade gh   # macOS
```

---

### Operations log

All mutating operations (delete, make-private, make-public, clone, pull) are appended to
`operations.log` in the directory you run the binary from. Check it for full
error messages that were truncated in the TUI:

```bash
cat operations.log
```

---

MIT
