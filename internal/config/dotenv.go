package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ParseDotenv reads a .env file and returns key-value pairs.
// Supports KEY=VALUE, # comments, quoted values, and `export` prefix.
func ParseDotenv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip optional "export " prefix
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		// Split on first '='
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip matching quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		vars[key] = val
	}
	return vars, scanner.Err()
}

// LoadDotenvFiles loads .env then .env.local from dir, merged (later wins).
func LoadDotenvFiles(dir string) map[string]string {
	merged := make(map[string]string)

	for _, name := range []string{".env", ".env.local"} {
		path := filepath.Join(dir, name)
		vars, err := ParseDotenv(path)
		if err != nil {
			continue // file doesn't exist or unreadable, skip
		}
		for k, v := range vars {
			merged[k] = v
		}
	}

	return merged
}
