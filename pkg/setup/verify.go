package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/db"
	"github.com/nanoclaw/nanoclaw/pkg/env"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
)

// RunVerify performs an end-to-end health check of the full installation.
// Ports logic from setup/verify.ts.
func RunVerify(args []string) {
	projectRoot, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()

	logger.Info("Starting verification")

	// 1. Check service status
	service := "not_found"
	mgr := GetServiceManager()

	if mgr == "launchd" {
		out, err := exec.Command("launchctl", "list").Output()
		if err == nil {
			output := string(out)
			if strings.Contains(output, "com.nanoclaw") {
				lines := strings.Split(output, "\n")
				for _, line := range lines {
					if strings.Contains(line, "com.nanoclaw") {
						fields := strings.Fields(line)
						if len(fields) > 0 {
							pidField := fields[0]
							if pidField != "-" && pidField != "" {
								service = "running"
							} else {
								service = "stopped"
							}
						}
						break
					}
				}
			}
		}
	} else if mgr == "systemd" {
		prefix := "systemctl"
		if !IsRoot() {
			prefix = "systemctl --user"
		}
		prefixParts := strings.Fields(prefix)

		cmdArgs := append(prefixParts[1:], "is-active", "nanoclaw")
		if err := exec.Command(prefixParts[0], cmdArgs...).Run(); err == nil {
			service = "running"
		} else {
			cmdArgs = append(prefixParts[1:], "list-unit-files")
			out, err := exec.Command(prefixParts[0], cmdArgs...).Output()
			if err == nil && strings.Contains(string(out), "nanoclaw") {
				service = "stopped"
			}
		}
	} else {
		// Check for nohup PID file
		pidFile := filepath.Join(projectRoot, "nanoclaw.pid")
		if _, err := os.Stat(pidFile); err == nil {
			data, err := os.ReadFile(pidFile)
			if err == nil {
				var pid int
				fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
				if pid > 0 {
					process, err := os.FindProcess(pid)
					if err == nil {
						// On Unix, signaling with 0 checks if the process is alive.
						if err := process.Signal(os.Signal(nil)); err == nil {
							service = "running"
						} else {
							service = "stopped"
						}
					} else {
						service = "stopped"
					}
				}
			}
		}
	}
	logger.Info("Service status", map[string]string{"service": service})

	// 2. Check container runtime
	containerRuntime := "none"
	if CommandExists("container") {
		containerRuntime = "apple-container"
	} else if CommandExists("docker") {
		if err := exec.Command("docker", "info").Run(); err == nil {
			containerRuntime = "docker"
		}
	}

	// 3. Check credentials
	credentials := "missing"
	envFile := filepath.Join(projectRoot, ".env")
	if _, err := os.Stat(envFile); err == nil {
		content, err := os.ReadFile(envFile)
		if err == nil {
			re := regexp.MustCompile(`(?m)^(CLAUDE_CODE_OAUTH_TOKEN|ANTHROPIC_API_KEY)=`)
			if re.Match(content) {
				credentials = "configured"
			}
		}
	}

	// 4. Check channel auth
	envKeys := []string{
		"TELEGRAM_BOT_TOKEN",
		"SLACK_BOT_TOKEN",
		"SLACK_APP_TOKEN",
		"DISCORD_BOT_TOKEN",
	}
	envVars := env.ReadEnvFile(envKeys)

	channelAuth := make(map[string]string)

	// WhatsApp: check for auth credentials on disk
	authDir := filepath.Join(projectRoot, "store", "auth")
	if entries, err := os.ReadDir(authDir); err == nil && len(entries) > 0 {
		channelAuth["whatsapp"] = "authenticated"
	}

	// Token-based channels
	if os.Getenv("TELEGRAM_BOT_TOKEN") != "" || envVars["TELEGRAM_BOT_TOKEN"] != "" {
		channelAuth["telegram"] = "configured"
	}
	if (os.Getenv("SLACK_BOT_TOKEN") != "" || envVars["SLACK_BOT_TOKEN"] != "") &&
		(os.Getenv("SLACK_APP_TOKEN") != "" || envVars["SLACK_APP_TOKEN"] != "") {
		channelAuth["slack"] = "configured"
	}
	if os.Getenv("DISCORD_BOT_TOKEN") != "" || envVars["DISCORD_BOT_TOKEN"] != "" {
		channelAuth["discord"] = "configured"
	}

	var configuredChannels []string
	for k := range channelAuth {
		configuredChannels = append(configuredChannels, k)
	}
	anyChannelConfigured := len(configuredChannels) > 0

	// 5. Check registered groups
	registeredGroups := 0
	dbPath := filepath.Join(config.StoreDir, "messages.db")
	if _, err := os.Stat(dbPath); err == nil {
		storage, err := db.NewSQLiteStorage(dbPath)
		if err == nil {
			groups, err := storage.GetAllRegisteredGroups()
			if err == nil {
				registeredGroups = len(groups)
			}
			storage.Close()
		}
	}

	// 6. Check mount allowlist
	mountAllowlist := "missing"
	if _, err := os.Stat(filepath.Join(homeDir, ".config", "nanoclaw", "mount-allowlist.json")); err == nil {
		mountAllowlist = "configured"
	}

	// Determine overall status
	status := "failed"
	if service == "running" && credentials != "missing" && anyChannelConfigured && registeredGroups > 0 {
		status = "success"
	}

	logger.Info("Verification complete", map[string]interface{}{"status": status, "channelAuth": channelAuth})

	channelAuthJSON, _ := json.Marshal(channelAuth)

	EmitStatus("VERIFY", map[string]interface{}{
		"SERVICE":             service,
		"CONTAINER_RUNTIME":   containerRuntime,
		"CREDENTIALS":         credentials,
		"CONFIGURED_CHANNELS": strings.Join(configuredChannels, ","),
		"CHANNEL_AUTH":        string(channelAuthJSON),
		"REGISTERED_GROUPS":   registeredGroups,
		"MOUNT_ALLOWLIST":     mountAllowlist,
		"STATUS":              status,
		"LOG":                 "logs/setup.log",
	})

	if status == "failed" {
		os.Exit(1)
	}
}
