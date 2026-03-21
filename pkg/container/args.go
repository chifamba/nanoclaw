package container

import (
	"fmt"
	"os"
	"strconv"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/env"
)

// DetectAuthMode determines the authentication mode based on environment variables
func DetectAuthMode() string {
	envVars := env.ReadEnvFile([]string{"ANTHROPIC_API_KEY"})
	if os.Getenv("ANTHROPIC_API_KEY") != "" || envVars["ANTHROPIC_API_KEY"] != "" {
		return "api-key"
	}
	return "oauth"
}

// BuildContainerArgs constructs the command-line arguments for the container runtime
func BuildContainerArgs(mounts []VolumeMount, containerName string, isMain bool) []string {
	args := []string{"run", "-i", "--rm", "--name", containerName}

	// Pass host timezone
	args = append(args, "-e", fmt.Sprintf("TZ=%s", config.Timezone))

	// Route API traffic through the credential proxy
	args = append(args, "-e", fmt.Sprintf("ANTHROPIC_BASE_URL=http://%s:%d", ContainerHostGateway, config.CredentialProxyPort))

	// Mirror host's auth method
	authMode := DetectAuthMode()
	if authMode == "api-key" {
		args = append(args, "-e", "ANTHROPIC_API_KEY=placeholder")
	} else {
		args = append(args, "-e", "CLAUDE_CODE_OAUTH_TOKEN=placeholder")
	}

	// Runtime-specific args
	args = append(args, HostGatewayArgs()...)

	// Gemini configuration
	envKeys := []string{
		"GOOGLE_AI_API_KEY",
		"GOOGLE_AI_MODEL",
		"OBSIDIAN_API_KEY",
		"OBSIDIAN_PORT",
		"OBSIDIAN_HOST",
	}
	envVars := env.ReadEnvFile(envKeys)

	geminiKey := os.Getenv("GOOGLE_AI_API_KEY")
	if geminiKey == "" {
		geminiKey = envVars["GOOGLE_AI_API_KEY"]
	}
	if geminiKey != "" {
		args = append(args, "-e", fmt.Sprintf("GOOGLE_AI_API_KEY=%s", geminiKey))
	}

	geminiModel := os.Getenv("GOOGLE_AI_MODEL")
	if geminiModel == "" {
		geminiModel = envVars["GOOGLE_AI_MODEL"]
	}
	if geminiModel != "" {
		args = append(args, "-e", fmt.Sprintf("GOOGLE_AI_MODEL=%s", geminiModel))
	}

	// Obsidian configuration
	obsidianKey := os.Getenv("OBSIDIAN_API_KEY")
	if obsidianKey == "" {
		obsidianKey = envVars["OBSIDIAN_API_KEY"]
	}
	if obsidianKey != "" {
		args = append(args, "-e", fmt.Sprintf("OBSIDIAN_API_KEY=%s", obsidianKey))
	}

	obsidianPort := os.Getenv("OBSIDIAN_PORT")
	if obsidianPort == "" {
		obsidianPort = envVars["OBSIDIAN_PORT"]
	}
	if obsidianPort == "" {
		obsidianPort = "27124"
	}
	args = append(args, "-e", fmt.Sprintf("OBSIDIAN_PORT=%s", obsidianPort))

	obsidianHost := os.Getenv("OBSIDIAN_HOST")
	if obsidianHost == "" {
		obsidianHost = envVars["OBSIDIAN_HOST"]
	}
	if obsidianHost == "" {
		obsidianHost = ContainerHostGateway
	}
	args = append(args, "-e", fmt.Sprintf("OBSIDIAN_HOST=%s", obsidianHost))

	// User UID/GID
	hostUid := os.Getuid()
	hostGid := os.Getgid()
	if hostUid != 0 && hostUid != 1000 {
		if isMain {
			args = append(args, "-e", fmt.Sprintf("RUN_UID=%d", hostUid))
			args = append(args, "-e", fmt.Sprintf("RUN_GID=%d", hostGid))
		} else {
			args = append(args, "--user", fmt.Sprintf("%d:%d", hostUid, hostGid))
		}
		args = append(args, "-e", "HOME=/home/node")
	}

	// Volume mounts
	for _, m := range mounts {
		if m.ReadOnly {
			args = append(args, ReadonlyMountArgs(m.HostPath, m.ContainerPath)...)
		} else {
			args = append(args, "-v", fmt.Sprintf("%s:%s", m.HostPath, m.ContainerPath))
		}
	}

	args = append(args, config.ContainerImage)

	return args
}

// ConvertToInt helper for optional values
func ConvertToInt(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return i
}
