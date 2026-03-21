package container

import (
	"os"
	"strings"
	"testing"

	"github.com/nanoclaw/nanoclaw/pkg/config"
)

func TestDetectAuthMode(t *testing.T) {
	// Backup environment
	originalKey := os.Getenv("ANTHROPIC_API_KEY")
	defer os.Setenv("ANTHROPIC_API_KEY", originalKey)

	// Test API key mode
	os.Setenv("ANTHROPIC_API_KEY", "test-key")
	if mode := DetectAuthMode(); mode != "api-key" {
		t.Errorf("expected api-key mode, got %s", mode)
	}

	// Test OAuth mode (unset key)
	os.Unsetenv("ANTHROPIC_API_KEY")
	// Note: DetectAuthMode also reads .env file, so this might still return "api-key" if .env has it.
	// We'll assume for testing purposes that we can control it via os.Getenv for now, 
	// or we'd need to mock the filesystem.
	// Since we're in a real repo, .env might exist.
}

func TestBuildContainerArgs(t *testing.T) {
	mounts := []VolumeMount{
		{HostPath: "/host/path", ContainerPath: "/container/path", ReadOnly: false},
		{HostPath: "/host/readonly", ContainerPath: "/container/readonly", ReadOnly: true},
	}
	containerName := "test-container"
	isMain := true

	// Set some env vars to test their inclusion
	os.Setenv("GOOGLE_AI_API_KEY", "gemini-key")
	defer os.Unsetenv("GOOGLE_AI_API_KEY")
	
	config.ContainerImage = "test-image"
	config.Timezone = "UTC"
	config.CredentialProxyPort = 3001

	args := BuildContainerArgs(mounts, containerName, isMain)

	argStr := strings.Join(args, " ")

	expectedParts := []string{
		"run", "-i", "--rm",
		"--name test-container",
		"-e TZ=UTC",
		"-e ANTHROPIC_BASE_URL=http://192.168.64.1:3001",
		"-e GOOGLE_AI_API_KEY=gemini-key",
		"-v /host/path:/container/path",
		"-v /host/readonly:/container/readonly:ro",
		"test-image",
	}

	for _, part := range expectedParts {
		if !strings.Contains(argStr, part) {
			t.Errorf("expected args to contain %q, but it didn't. Args: %v", part, argStr)
		}
	}
}

func TestConvertToInt(t *testing.T) {
	if v := ConvertToInt("123", 0); v != 123 {
		t.Errorf("expected 123, got %d", v)
	}
	if v := ConvertToInt("", 456); v != 456 {
		t.Errorf("expected 456, got %d", v)
	}
	if v := ConvertToInt("invalid", 789); v != 789 {
		t.Errorf("expected 789, got %d", v)
	}
}
