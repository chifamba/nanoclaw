package setup

import (
	"os"
	"runtime"
	"testing"
)

func TestGetPlatform(t *testing.T) {
	platform := GetPlatform()
	switch runtime.GOOS {
	case "darwin":
		if platform != "macOS" {
			t.Errorf("expected macOS, got %s", platform)
		}
	case "linux":
		if platform != "Linux" {
			t.Errorf("expected Linux, got %s", platform)
		}
	default:
		if platform != "Other" {
			t.Errorf("expected Other, got %s", platform)
		}
	}
}

func TestIsWSL(t *testing.T) {
	// Just ensure it doesn't panic
	_ = IsWSL()
}

func TestIsHeadless(t *testing.T) {
	if runtime.GOOS == "linux" {
		origDisplay := os.Getenv("DISPLAY")
		origWayland := os.Getenv("WAYLAND_DISPLAY")
		defer func() {
			os.Setenv("DISPLAY", origDisplay)
			os.Setenv("WAYLAND_DISPLAY", origWayland)
		}()

		os.Setenv("DISPLAY", "")
		os.Setenv("WAYLAND_DISPLAY", "")
		if !IsHeadless() {
			t.Errorf("expected headless to be true when DISPLAY and WAYLAND_DISPLAY are empty")
		}

		os.Setenv("DISPLAY", ":0")
		if IsHeadless() {
			t.Errorf("expected headless to be false when DISPLAY is set")
		}
	} else {
		if IsHeadless() {
			t.Errorf("expected headless to be false on non-linux")
		}
	}
}

func TestCommandExists(t *testing.T) {
	// "ls" should exist on macOS/Linux
	if runtime.GOOS != "windows" {
		if !CommandExists("ls") {
			t.Errorf("expected ls to exist")
		}
	}
	if CommandExists("this_command_does_not_exist_xyz_123") {
		t.Errorf("expected nonexistent command to not exist")
	}
}

func TestIsAppleSilicon(t *testing.T) {
	expected := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	if IsAppleSilicon() != expected {
		t.Errorf("expected %v, got %v", expected, IsAppleSilicon())
	}
}

func TestGetArchitecture(t *testing.T) {
	if GetArchitecture() != runtime.GOARCH {
		t.Errorf("expected %s, got %s", runtime.GOARCH, GetArchitecture())
	}
}

func TestIsArm64(t *testing.T) {
	expected := runtime.GOARCH == "arm64"
	if IsArm64() != expected {
		t.Errorf("expected %v, got %v", expected, IsArm64())
	}
}

func TestHasSystemd(t *testing.T) {
	// Just ensure it doesn't panic
	_ = HasSystemd()
}

func TestGetServiceManager(t *testing.T) {
	mgr := GetServiceManager()
	if runtime.GOOS == "darwin" && mgr != "launchd" {
		t.Errorf("expected launchd on macOS, got %s", mgr)
	}
	if runtime.GOOS == "linux" {
		if mgr != "systemd" && mgr != "none" {
			t.Errorf("expected systemd or none on Linux, got %s", mgr)
		}
	}
}

func TestGetNodePath(t *testing.T) {
	path := GetNodePath()
	if path == "" {
		t.Errorf("expected non-empty node path")
	}
}

func TestGetNodeVersion(t *testing.T) {
	// Node might not be installed in CI, just ensure it doesn't panic
	_ = GetNodeVersion()
}

func TestGetNodeMajorVersion(t *testing.T) {
	_ = GetNodeMajorVersion()
}
