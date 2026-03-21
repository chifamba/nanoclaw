package setup

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/nanoclaw/nanoclaw/pkg/logger"
)

type MountConfig struct {
	AllowedRoots    []string `json:"allowedRoots"`
	BlockedPatterns []string `json:"blockedPatterns"`
	NonMainReadOnly bool     `json:"nonMainReadOnly"`
}

func parseMountArgs(args []string) (bool, string) {
	empty := false
	jsonStr := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--empty" {
			empty = true
		}
		if args[i] == "--json" && i+1 < len(args) {
			jsonStr = args[i+1]
			i++
		}
	}
	return empty, jsonStr
}

func ConfigureMounts(args []string) {
	empty, jsonStr := parseMountArgs(args)
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "nanoclaw")
	configFile := filepath.Join(configDir, "mount-allowlist.json")

	if IsRoot() {
		logger.Warn("Running as root — mount allowlist will be written to root home directory")
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		logger.Error("Failed to create config directory", err)
		EmitStatus("CONFIGURE_MOUNTS", map[string]interface{}{
			"STATUS": "failed",
			"ERROR":  "mkdir_failed",
			"LOG":    "logs/setup.log",
		})
		os.Exit(1)
	}

	var config MountConfig
	var writeData []byte

	if empty {
		logger.Info("Writing empty mount allowlist")
		config = MountConfig{
			AllowedRoots:    []string{},
			BlockedPatterns: []string{},
			NonMainReadOnly: true,
		}
		writeData, _ = json.MarshalIndent(config, "", "  ")
	} else if jsonStr != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			logger.Error("Invalid JSON input", err)
			EmitStatus("CONFIGURE_MOUNTS", map[string]interface{}{
				"PATH":               configFile,
				"ALLOWED_ROOTS":      0,
				"NON_MAIN_READ_ONLY": "unknown",
				"STATUS":             "failed",
				"ERROR":              "invalid_json",
				"LOG":                "logs/setup.log",
			})
			os.Exit(4)
		}
		writeData, _ = json.MarshalIndent(parsed, "", "  ")
	} else {
		logger.Info("Reading mount allowlist from stdin")
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			logger.Error("Failed to read from stdin", err)
			EmitStatus("CONFIGURE_MOUNTS", map[string]interface{}{
				"STATUS": "failed",
				"ERROR":  "stdin_read_failed",
				"LOG":    "logs/setup.log",
			})
			os.Exit(1)
		}
		var parsed interface{}
		if err := json.Unmarshal(input, &parsed); err != nil {
			logger.Error("Invalid JSON from stdin", err)
			EmitStatus("CONFIGURE_MOUNTS", map[string]interface{}{
				"PATH":               configFile,
				"ALLOWED_ROOTS":      0,
				"NON_MAIN_READ_ONLY": "unknown",
				"STATUS":             "failed",
				"ERROR":              "invalid_json",
				"LOG":                "logs/setup.log",
			})
			os.Exit(4)
		}
		writeData, _ = json.MarshalIndent(parsed, "", "  ")
	}

	// For status report, we need some fields from the parsed data
	var parsedConfig map[string]interface{}
	json.Unmarshal(writeData, &parsedConfig)
	allowedRoots := 0
	if ar, ok := parsedConfig["allowedRoots"].([]interface{}); ok {
		allowedRoots = len(ar)
	}
	nonMainReadOnly := "true"
	if nmro, ok := parsedConfig["nonMainReadOnly"].(bool); ok && !nmro {
		nonMainReadOnly = "false"
	}

	if err := os.WriteFile(configFile, append(writeData, '\n'), 0644); err != nil {
		logger.Error("Failed to write config file", err)
		EmitStatus("CONFIGURE_MOUNTS", map[string]interface{}{
			"STATUS": "failed",
			"ERROR":  "write_failed",
			"LOG":    "logs/setup.log",
		})
		os.Exit(1)
	}

	logger.Info("Allowlist configured", map[string]interface{}{
		"configFile":      configFile,
		"allowedRoots":    allowedRoots,
		"nonMainReadOnly": nonMainReadOnly,
	})

	EmitStatus("CONFIGURE_MOUNTS", map[string]interface{}{
		"PATH":               configFile,
		"ALLOWED_ROOTS":      allowedRoots,
		"NON_MAIN_READ_ONLY": nonMainReadOnly,
		"STATUS":             "success",
		"LOG":                "logs/setup.log",
	})
}
