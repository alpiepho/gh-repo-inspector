# gh-repo-inspector

A terminal UI for inspecting and managing all your GitHub repositories.  
Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

---

## Features

| Phase | What it does |
|-------|-------------|
| **1 – Inspect** | Browse all repos: name, description, last commit, visibility, fork status, branch count, disk size |
| **2 – Forks** | Review forks and selectively delete them |
| **3 – Public** | Review public repos and change visibility to private |
| **4 – By Age** | Browse repos oldest-first and selectively delete them |
| **5 – Clone** | Multi-select repos and clone locally; track what's already cloned |
| **Test Setup** | Step-by-step guide for creating test repos via the `gh` CLI |

### Clone extras
- Select by group: All / Forks only / Public (non-fork) / Private (non-fork) / Clear
- Toggle between cloning the default branch only (`--single-branch`) or all branches
- Live progress: shows the exact `git clone` command as each repo clones
- 10-minute per-repo timeout so a stalled clone never hangs the UI
- Remembers the last destination path across sessions
- Tracks clone history (date, path, branch mode) and highlights already-cloned repos

### Dry-run mode
All mutating operations (delete, make-private, clone) can be previewed without side effects:

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

### Global key bindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` | Select / confirm |
| `Esc` / `q` | Back / quit |
| `D` | Toggle dry-run mode |

### Inspect (phase 1)

| Key | Action |
|-----|--------|
| `↑↓` | Scroll |
| `s` | Cycle sort column |
| `f` | Toggle filter (all / forks / public / private) |

### Clone (phase 5)

| Key | Action |
|-----|--------|
| `Space` | Toggle selection |
| `a` | Open group-select menu |
| `b` | Toggle branch mode (default / all) |
| `Enter` | Proceed to destination prompt |

---

## Architecture

```
internal/gh/client.go      — all gh CLI / git subprocess calls
internal/config/config.go  — persists last clone path & clone history
internal/ui/app.go         — Bubble Tea root model; navigation stack
internal/ui/menu.go        — main menu
internal/ui/repolist.go    — shared sortable/filterable repo table (phases 1–4)
internal/ui/confirm.go     — dry-run-aware confirmation dialog
internal/ui/clone.go       — multi-select clone picker with live progress
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

## License

MIT
