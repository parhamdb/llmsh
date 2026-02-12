package shell

import (
	"os"

	"github.com/parhamdb/llmsh/internal/config"
)

// HistoryPath returns the path to the shell history file.
func HistoryPath() string {
	dir, _ := config.UserConfigDir()
	if dir == "" {
		return ""
	}
	os.MkdirAll(dir, 0755)
	return dir + "/history"
}
