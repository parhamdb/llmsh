package shell

import (
	"os"
	"path/filepath"
)

// HistoryPath returns the path to the shell history file.
func HistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".llmsh")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "history")
}
