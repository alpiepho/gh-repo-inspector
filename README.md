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
| **9 – Migrate to GitLab** | Push locally cloned repos to a local GitLab instance |

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
| `1`–`9` | Jump directly to that option |
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

### Migrate to GitLab (option 9)

| Key | Action |
|-----|--------|
| `Tab` | Switch between URL and token fields |
| `Enter` | Confirm field / advance to next step |
| `Space` | Toggle repo selection |
| `a` | Select all / deselect all |
| `s` | Skip conflicting repo (already exists on GitLab) |
| `f` | Force push conflicting repo |

---

## GitLab Migration Setup

Option 9 pushes locally cloned repos to a self-hosted GitLab.

### Generating a Personal Access Token (PAT)

1. Open your GitLab instance in a browser (e.g. `http://10.0.0.60:8929`)
2. Click your **avatar** (top-right corner)
3. Go to **Edit profile** → **Access Tokens** (left sidebar)
4. Click **Add new token**
5. Give it a name (e.g. `gh-repo-inspector`) and an expiry date
6. Check these two scopes:
   - ✅ `api` — needed to create repos and look up your username
   - ✅ `write_repository` — needed to push code
7. Click **Create personal access token**
8. **Copy the token immediately** — GitLab only shows it once

### First-time configuration in the app

1. Press **9** on the main menu
2. Enter your GitLab URL (e.g. `http://10.0.0.60:8929`) and press `Enter`
3. Press `Tab` to move to the token field, paste your PAT, and press `Enter`
4. The settings are saved to `state.json` next to the binary — you won't be prompted again

### What is stored locally

Both `state.json` and `operations.log` are written to the **same directory you run the binary from**. Neither file is tracked by git (both are in `.gitignore`).

| File | Contents |
|------|----------|
| `state.json` | Last clone/migrate path, clone history, GitLab URL and PAT |
| `operations.log` | Timestamped log of all mutating operations |

> **Security note:** The PAT is stored in plaintext in `state.json`. Keep this file private
> and do not commit it. For a personal local tool on a trusted machine this is acceptable,
> but be aware of the risk if others have access to your filesystem.

### What happens during migration

- The app scans a local directory you choose for git repos (`.git` folders); your last-used directory is remembered
- You multi-select which repos to push
- If any already exist on GitLab, you are prompted per-repo to **skip** (`s`) or **force push** (`f`)
- Each new repo is created on GitLab automatically, a `gitlab` remote is added to the local clone, and `git push --all` is run
- The PAT is used only for the push and is **stripped from `.git/config`** afterwards — the stored remote URL contains no credentials
- You can enter the GitLab URL with or without the `http://` scheme (e.g. `10.0.0.60:8929` works)
- All results are logged to `operations.log`

---

## Architecture

```
internal/gh/client.go      — all gh CLI / git subprocess calls
internal/gitlab/client.go  — GitLab REST API client (check/create repo, push auth)
internal/config/config.go  — persists last clone path, clone history, GitLab settings
internal/oplog/oplog.go    — append-only operations log (./operations.log)
internal/ui/app.go         — Bubble Tea root model; navigation stack
internal/ui/menu.go        — main menu with number shortcuts
internal/ui/repolist.go    — shared sortable/filterable repo table (options 1–5)
internal/ui/confirm.go     — dry-run-aware confirmation dialog
internal/ui/clone.go       — multi-select clone picker with live progress
internal/ui/refresh.go     — directory-scan git pull view
internal/ui/migrate.go     — GitLab migration view
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
