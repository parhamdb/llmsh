package config

import (
	"os"
	"path/filepath"
)

// xdgConfigHome returns $XDG_CONFIG_HOME or ~/.config.
func xdgConfigHome() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config")
}

// UserConfigDir returns the llmsh config directory and whether XDG was used.
// It checks $XDG_CONFIG_HOME/llmsh/ first, then falls back to ~/.llmsh/.
func UserConfigDir() (string, bool) {
	xdg := xdgConfigHome()
	if xdg != "" {
		xdgDir := filepath.Join(xdg, "llmsh")
		if _, err := os.Stat(xdgDir); err == nil {
			return xdgDir, true
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, ".llmsh"), false
}

// UserConfigPath returns the path to the user's config.yaml.
func UserConfigPath() string {
	dir, _ := UserConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config.yaml")
}

// CredentialsPath returns the path to the user's credentials.yaml.
func CredentialsPath() string {
	dir, _ := UserConfigDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "credentials.yaml")
}
