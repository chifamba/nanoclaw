package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nanoclaw/nanoclaw/pkg/logger"
)

// RunService handles generating and loading service manager config.
// Ports logic from setup/service.ts.
func RunService(args []string) {
	projectRoot, _ := os.Getwd()
	platform := GetPlatform()
	nodePath := GetNodePath()
	homeDir, _ := os.UserHomeDir()

	logger.Info("Setting up service", map[string]interface{}{
		"platform":    platform,
		"nodePath":    nodePath,
		"projectRoot": projectRoot,
	})

	// Build first
	logger.Info("Building Go binary")
	buildCmd := exec.Command("make", "build")
	buildCmd.Dir = projectRoot
	if err := buildCmd.Run(); err != nil {
		logger.Error("Build failed", err)
		EmitStatus("SETUP_SERVICE", map[string]interface{}{
			"SERVICE_TYPE": "unknown",
			"NODE_PATH":    nodePath,
			"PROJECT_PATH": projectRoot,
			"STATUS":       "failed",
			"ERROR":        "build_failed",
			"LOG":          "logs/setup.log",
		})
		os.Exit(1)
	}
	logger.Info("Build succeeded")

	os.MkdirAll(filepath.Join(projectRoot, "logs"), 0755)

	if runtime.GOOS == "darwin" {
		setupLaunchd(projectRoot, homeDir)
	} else if runtime.GOOS == "linux" {
		setupLinux(projectRoot, homeDir)
	} else {
		EmitStatus("SETUP_SERVICE", map[string]interface{}{
			"SERVICE_TYPE": "unknown",
			"NODE_PATH":    nodePath,
			"PROJECT_PATH": projectRoot,
			"STATUS":       "failed",
			"ERROR":        "unsupported_platform",
			"LOG":          "logs/setup.log",
		})
		os.Exit(1)
	}
}

func setupLaunchd(projectRoot string, homeDir string) {
	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "com.nanoclaw.plist")
	os.MkdirAll(filepath.Dir(plistPath), 0755)

	binaryPath := filepath.Join(projectRoot, "bin", "nanoclaw")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.nanoclaw</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:%s/.local/bin</string>
        <key>HOME</key>
        <string>%s</string>
    </dict>
    <key>StandardOutPath</key>
    <string>%s/logs/nanoclaw.log</string>
    <key>StandardErrorPath</key>
    <string>%s/logs/nanoclaw.error.log</string>
</dict>
</plist>`, binaryPath, projectRoot, homeDir, homeDir, projectRoot, projectRoot)

	err := os.WriteFile(plistPath, []byte(plist), 0644)
	if err != nil {
		logger.Error("Failed to write launchd plist", err)
		return
	}
	logger.Info("Wrote launchd plist", map[string]string{"plistPath": plistPath})

	_ = exec.Command("launchctl", "load", plistPath).Run()
	logger.Info("launchctl load attempted")

	// Verify
	serviceLoaded := false
	out, err := exec.Command("launchctl", "list").Output()
	if err == nil && strings.Contains(string(out), "com.nanoclaw") {
		serviceLoaded = true
	}

	EmitStatus("SETUP_SERVICE", map[string]interface{}{
		"SERVICE_TYPE":   "launchd",
		"PROJECT_PATH":   projectRoot,
		"PLIST_PATH":     plistPath,
		"SERVICE_LOADED": serviceLoaded,
		"STATUS":         "success",
		"LOG":            "logs/setup.log",
	})
}

func setupLinux(projectRoot string, homeDir string) {
	mgr := GetServiceManager()
	if mgr == "systemd" {
		setupSystemd(projectRoot, homeDir)
	} else {
		setupNohupFallback(projectRoot, homeDir)
	}
}

func setupSystemd(projectRoot string, homeDir string) {
	runningAsRoot := IsRoot()
	binaryPath := filepath.Join(projectRoot, "bin", "nanoclaw")

	var unitPath string
	var systemctlPrefix []string

	if runningAsRoot {
		unitPath = "/etc/systemd/system/nanoclaw.service"
		systemctlPrefix = []string{"systemctl"}
		logger.Info("Running as root — installing system-level systemd unit")
	} else {
		// Check if user-level systemd session is available
		if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
			logger.Warn("systemd user session not available — falling back to nohup wrapper")
			setupNohupFallback(projectRoot, homeDir)
			return
		}
		unitDir := filepath.Join(homeDir, ".config", "systemd", "user")
		os.MkdirAll(unitDir, 0755)
		unitPath = filepath.Join(unitDir, "nanoclaw.service")
		systemctlPrefix = []string{"systemctl", "--user"}
	}

	wantedBy := "default.target"
	if runningAsRoot {
		wantedBy = "multi-user.target"
	}

	unit := fmt.Sprintf(`[Unit]
Description=NanoClaw Personal Assistant
After=network.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=always
RestartSec=5
Environment=HOME=%s
Environment=PATH=/usr/local/bin:/usr/bin:/bin:%s/.local/bin
StandardOutput=append:%s/logs/nanoclaw.log
StandardError=append:%s/logs/nanoclaw.error.log

[Install]
WantedBy=%s
`, binaryPath, projectRoot, homeDir, homeDir, projectRoot, projectRoot, wantedBy)

	err := os.WriteFile(unitPath, []byte(unit), 0644)
	if err != nil {
		logger.Error("Failed to write systemd unit", err)
		return
	}
	logger.Info("Wrote systemd unit", map[string]string{"unitPath": unitPath})

	dockerGroupStale := !runningAsRoot && checkDockerGroupStale()
	if dockerGroupStale {
		logger.Warn("Docker group not active in systemd session — user was likely added to docker group mid-session")
	}

	killOrphanedProcesses(projectRoot)

	// Enable and start
	_ = exec.Command(systemctlPrefix[0], append(systemctlPrefix[1:], "daemon-reload")...).Run()
	_ = exec.Command(systemctlPrefix[0], append(systemctlPrefix[1:], "enable", "nanoclaw")...).Run()
	_ = exec.Command(systemctlPrefix[0], append(systemctlPrefix[1:], "start", "nanoclaw")...).Run()

	// Verify
	serviceLoaded := false
	if err := exec.Command(systemctlPrefix[0], append(systemctlPrefix[1:], "is-active", "nanoclaw")...).Run(); err == nil {
		serviceLoaded = true
	}

	serviceType := "systemd-user"
	if runningAsRoot {
		serviceType = "systemd-system"
	}

	fields := map[string]interface{}{
		"SERVICE_TYPE":   serviceType,
		"PROJECT_PATH":   projectRoot,
		"UNIT_PATH":      unitPath,
		"SERVICE_LOADED": serviceLoaded,
		"STATUS":         "success",
		"LOG":            "logs/setup.log",
	}
	if dockerGroupStale {
		fields["DOCKER_GROUP_STALE"] = true
	}
	EmitStatus("SETUP_SERVICE", fields)
}

func checkDockerGroupStale() bool {
	cmd := exec.Command("systemd-run", "--user", "--pipe", "--wait", "docker", "info")
	if err := cmd.Run(); err != nil {
		if err := exec.Command("docker", "info").Run(); err == nil {
			return true
		}
	}
	return false
}

func killOrphanedProcesses(projectRoot string) {
	binaryPath := filepath.Join(projectRoot, "bin", "nanoclaw")
	_ = exec.Command("pkill", "-f", binaryPath).Run()
	nodeTarget := filepath.Join(projectRoot, "dist", "index.js")
	_ = exec.Command("pkill", "-f", nodeTarget).Run()
	logger.Info("Stopped any orphaned nanoclaw processes")
}

func setupNohupFallback(projectRoot string, homeDir string) {
	logger.Warn("No systemd detected — generating nohup wrapper script")

	wrapperPath := filepath.Join(projectRoot, "start-nanoclaw.sh")
	pidFile := filepath.Join(projectRoot, "nanoclaw.pid")
	binaryPath := filepath.Join(projectRoot, "bin", "nanoclaw")
	logPath := filepath.Join(projectRoot, "logs", "nanoclaw.log")
	errLogPath := filepath.Join(projectRoot, "logs", "nanoclaw.error.log")

	wrapper := fmt.Sprintf(`#!/bin/bash
# start-nanoclaw.sh — Start NanoClaw without systemd
# To stop: kill $(cat "%s")

set -euo pipefail

cd "%s"

# Stop existing instance if running
if [ -f "%s" ]; then
  OLD_PID=$(cat "%s" 2>/dev/null || echo "")
  if [ -n "$OLD_PID" ] && kill -0 "$OLD_PID" 2>/dev/null; then
    echo "Stopping existing NanoClaw (PID $OLD_PID)..."
    kill "$OLD_PID" 2>/dev/null || true
    sleep 2
  fi
fi

echo "Starting NanoClaw..."
nohup "%s" \
  >> "%s" \
  2>> "%s" &

echo $! > "%s"
echo "NanoClaw started (PID $!)"
echo "Logs: tail -f %s"
`, pidFile, projectRoot, pidFile, pidFile, binaryPath, logPath, errLogPath, pidFile, logPath)

	err := os.WriteFile(wrapperPath, []byte(wrapper), 0755)
	if err != nil {
		logger.Error("Failed to write nohup wrapper", err)
		return
	}
	logger.Info("Wrote nohup wrapper script", map[string]string{"wrapperPath": wrapperPath})

	EmitStatus("SETUP_SERVICE", map[string]interface{}{
		"SERVICE_TYPE":   "nohup",
		"PROJECT_PATH":   projectRoot,
		"WRAPPER_PATH":   wrapperPath,
		"SERVICE_LOADED": false,
		"FALLBACK":       "wsl_no_systemd",
		"STATUS":         "success",
		"LOG":            "logs/setup.log",
	})
}
