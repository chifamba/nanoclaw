package setup

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// GetPlatform returns "macOS", "Linux", or "Other".
func GetPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return "Other"
	}
}

// IsWSL returns boolean.
func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	release := strings.ToLower(string(data))
	return strings.Contains(release, "microsoft") || strings.Contains(release, "wsl")
}

// IsHeadless returns boolean.
func IsHeadless() bool {
	if runtime.GOOS == "linux" {
		return os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
	}
	return false
}

// IsRoot returns boolean.
func IsRoot() bool {
	return os.Getuid() == 0
}

// CommandExists returns boolean.
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// IsAppleSilicon returns boolean.
func IsAppleSilicon() bool {
	return runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
}

// GetArchitecture returns architecture string.
func GetArchitecture() string {
	return runtime.GOARCH
}

// IsArm64 returns boolean.
func IsArm64() bool {
	return runtime.GOARCH == "arm64"
}

// HasSystemd returns true if systemd is the init process.
func HasSystemd() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/1/comm")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "systemd"
}

// GetServiceManager returns "launchd", "systemd", or "none".
func GetServiceManager() string {
	if runtime.GOOS == "darwin" {
		return "launchd"
	}
	if runtime.GOOS == "linux" {
		if HasSystemd() {
			return "systemd"
		}
		return "none"
	}
	return "none"
}

// GetNodePath returns the path to the node executable.
func GetNodePath() string {
	path, err := exec.LookPath("node")
	if err == nil {
		return path
	}
	return "node"
}

// OpenBrowser opens a URL in the default browser.
func OpenBrowser(url string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		if IsWSL() {
			if CommandExists("wslview") {
				cmd = exec.Command("wslview", url)
			} else {
				cmd = exec.Command("cmd.exe", "/c", "start", "", url)
			}
		} else {
			cmd = exec.Command("xdg-open", url)
		}
	default:
		return false
	}

	err := cmd.Run()
	return err == nil
}

// GetNodeVersion returns the node version string.
func GetNodeVersion() string {
	out, err := exec.Command("node", "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
}

// GetNodeMajorVersion returns the node major version.
func GetNodeMajorVersion() int {
	version := GetNodeVersion()
	if version == "" {
		return 0
	}
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0
	}
	var major int
	fmt.Sscanf(parts[0], "%d", &major)
	return major
}
