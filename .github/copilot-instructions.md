# Copilot Instructions

Terminal UI utility for inspecting and managing GitHub repositories. Built in Go using Bubble Tea.

## Commands

```bash
# Install dependencies
go mod tidy

# Build
go build -o gh-repo-inspector .

# Run (requires gh CLI authenticated)
./gh-repo-inspector

# Run in dry-run mode (no changes made)
./gh-repo-inspector --dry-run

# Run tests
go test ./...

# Run a single package's tests
go test ./internal/gh/...
```

## Architecture

```
internal/gh/client.go     — all gh CLI and git subprocess calls (ListRepos, DeleteRepo, SetPrivate, CloneRepo)
internal/ui/app.go        — root Bubble Tea model; owns navigation stack ([]tea.Model) and global DryRun bool
internal/ui/menu.go       — main menu; receives repos via onRepos() after async fetch
internal/ui/repolist.go   — reusable sortable/filterable repo table used by phases 1–4
internal/ui/confirm.go    — single-action confirmation dialog; dry-run aware
internal/ui/clone.go      — multi-select clone picker with dest dir prompt and progress
internal/ui/testsetup.go  — paginated checklist guide for creating test repos
internal/ui/styles.go     — shared Lip Gloss styles and navigation message types (screenMsg, popMsg)
main.go                   — parses --dry-run flag, starts tea.Program with alt screen
```

**Navigation:** `app.go` holds a stack of `tea.Model`. Child models push new screens via `PushScreen()` cmd and pop via `PopScreen()` cmd (both defined in `styles.go`). `popMsg` on the last item quits.

**Repo fetch:** `fetchRepos` runs once on `Init()` as a `tea.Cmd`. Result (`fetchReposMsg`) is handled in `app.Update` and delivered to the menu via `menu.onRepos()`.

**Dry-run:** Stored as `app.DryRun` bool. `gh.DeleteRepo`, `gh.SetPrivate`, and `gh.CloneRepo` all accept a `dryRun bool` and no-op when true. Any phase view can toggle it with the `D` key.

## Conventions

- All `gh`/`git` subprocess calls live in `internal/gh/client.go` — UI files never call `exec.Command` directly.
- Shared Lip Gloss styles are in `internal/ui/styles.go`; don't define styles inline in view files.
- Navigation commands (`PushScreen`, `PopScreen`, `Reload`) are also in `styles.go`.
- `RepoList` is configured via `Filter` + `Mode` enums — add new phases by adding new enum values, not new structs.
- The `gh repo list` call uses `--limit 1000`; repos are cached in memory for the session (no re-fetch unless `reloadMsg` is sent).
