package env

import (
	"os"
	"path/filepath"
	"strings"
)

// readEnvFile reads specified keys from .env file, returns map of found values
// Does NOT load anything into os.Enviroment — callers decide what to do with the values.
// This keeps secrets out of the process environment so they don't leak to child processes.
func ReadEnvFile(keys []string) map[string]string {
	envFile := filepath.Join(".", ".env")
	content, err := os.ReadFile(envFile)
	if err != nil {
		// logger.Debug would go here in real implementation
		return make(map[string]string)
	}

	result := make(map[string]string)
	wanted := make(map[string]bool)
	for _, key := range keys {
		wanted[key] = true
	}

	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx == -1 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eqIdx])
		if !wanted[key] {
			continue
		}
		value := strings.TrimSpace(trimmed[eqIdx+1:])
		if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
			(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
			value = value[1 : len(value)-1]
		}
		if value != "" {
			result[key] = value
		}
	}

	return result
}