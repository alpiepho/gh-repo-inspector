package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/al/gh-repo-inspector/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Preview actions without making changes")
	flag.BoolVar(dryRun, "n", false, "Shorthand for --dry-run")
	flag.Parse()

	app := ui.NewApp(*dryRun)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
