// Package oplog writes an append-only human-readable log of all GitHub operations.
// The log file lives at: $XDG_CONFIG_HOME/gh-repo-inspector/operations.log
// (on macOS: ~/Library/Application Support/gh-repo-inspector/operations.log)
// Errors are silently swallowed — logging must never crash the app.
package oplog

import (
	"fmt"
	"os"
	"time"
)

func logPath() string {
	return "operations.log"
}

// Write appends one log line:
//
//	2006-01-02T15:04:05Z  OPERATION       target                    result
func Write(operation, target, result string) {
	path := logPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	line := fmt.Sprintf("%s  %-14s %-44s %s\n",
		time.Now().UTC().Format(time.RFC3339),
		operation, target, result)
	_, _ = f.WriteString(line)
}

// Path returns the path to the log file (for display purposes).
func Path() string {
	return logPath()
}
