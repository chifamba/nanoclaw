package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/nanoclaw/nanoclaw/pkg/env"
)

// Constants from src/config.ts that are not overridden by environment variables
const (
	// PollInterval is the polling interval for various services in milliseconds
	PollInterval = 2000
	// SchedulerPollInterval is the interval for the task scheduler in milliseconds
	SchedulerPollInterval = 60000
	// IPCPollInterval is the interval for IPC polling in milliseconds
	IPCPollInterval = 1000
)

// Variables from src/config.ts that can be overridden by environment variables or .env
var (
	// AssistantName is the name of the assistant (default "Andy")
	AssistantName string
	// AssistantHasOwnNumber indicates if the assistant has its own phone number
	AssistantHasOwnNumber bool
	// MountAllowlistPath is the absolute path to the mount allowlist configuration
	MountAllowlistPath string
	// SenderAllowlistPath is the absolute path to the sender allowlist configuration
	SenderAllowlistPath string
	// StoreDir is the absolute path to the store directory
	StoreDir string
	// GroupsDir is the absolute path to the groups directory
	GroupsDir string
	// DataDir is the absolute path to the data directory
	DataDir string
	// ContainerImage is the docker image used for agent containers
	ContainerImage string
	// ContainerTimeout is the maximum execution time for a container in milliseconds
	ContainerTimeout int
	// ContainerMaxOutputSize is the maximum size of container output in bytes
	ContainerMaxOutputSize int
	// CredentialProxyPort is the port for the credential proxy service
	CredentialProxyPort int
	// IdleTimeout is the time to keep a container alive after last result in milliseconds
	IdleTimeout int
	// MaxConcurrentContainers is the maximum number of containers that can run simultaneously
	MaxConcurrentContainers int
	// TriggerPattern is the regex pattern used to trigger the assistant
	TriggerPattern *regexp.Regexp
	// Timezone is the timezone used for scheduled tasks
	Timezone string
	// RedisURL is the URL of the Redis cache, if configured
	RedisURL string
)

func init() {
	Load()
}

// Load reads configuration from environment variables and .env file.
// It is called automatically by init(), but can be called manually for testing.
func Load() {
	envKeys := []string{
		"ASSISTANT_NAME",
		"ASSISTANT_HAS_OWN_NUMBER",
		"CONTAINER_IMAGE",
		"CONTAINER_TIMEOUT",
		"CONTAINER_MAX_OUTPUT_SIZE",
		"CREDENTIAL_PROXY_PORT",
		"IDLE_TIMEOUT",
		"MAX_CONCURRENT_CONTAINERS",
		"TZ",
		"REDIS_URL",
	}

	envConfig := env.ReadEnvFile(envKeys)

	AssistantName = getEnv("ASSISTANT_NAME", envConfig, "Andy")
	AssistantHasOwnNumber = getEnv("ASSISTANT_HAS_OWN_NUMBER", envConfig, "false") == "true"

	projectRoot, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()
	if h := os.Getenv("HOME"); h != "" {
		homeDir = h
	}

	MountAllowlistPath = filepath.Join(homeDir, ".config", "nanoclaw", "mount-allowlist.json")
	SenderAllowlistPath = filepath.Join(homeDir, ".config", "nanoclaw", "sender-allowlist.json")
	StoreDir = filepath.Join(projectRoot, "store")
	GroupsDir = filepath.Join(projectRoot, "groups")
	DataDir = filepath.Join(projectRoot, "data")

	ContainerImage = getEnv("CONTAINER_IMAGE", envConfig, "nanoclaw-agent:latest")
	ContainerTimeout = getIntEnv("CONTAINER_TIMEOUT", envConfig, 1800000)
	ContainerMaxOutputSize = getIntEnv("CONTAINER_MAX_OUTPUT_SIZE", envConfig, 10485760)
	CredentialProxyPort = getIntEnv("CREDENTIAL_PROXY_PORT", envConfig, 3001)
	IdleTimeout = getIntEnv("IDLE_TIMEOUT", envConfig, 1800000)

	maxContainers := getIntEnv("MAX_CONCURRENT_CONTAINERS", envConfig, 5)
	if maxContainers < 1 {
		maxContainers = 1
	}
	MaxConcurrentContainers = maxContainers

	// TRIGGER_PATTERN matches ^@AssistantName\b case-insensitively
	escapedName := regexp.QuoteMeta(AssistantName)
	TriggerPattern = regexp.MustCompile("(?i)^@" + escapedName + "\\b")

	Timezone = os.Getenv("TZ")
	if Timezone == "" {
		Timezone = envConfig["TZ"]
	}
	if Timezone == "" {
		// In Go, we can't easily get the IANA timezone name without looking at /etc/localtime or similar.
		// For consistency with typical Node.js environments and lack of a robust built-in Go way,
		// we'll default to "UTC" if not set, though the system local is used in TS.
		// However, many systems don't have TZ set.
		// If we want to be closer to JS's Intl.DateTimeFormat().resolvedOptions().timeZone,
		// we could try to read /etc/timezone or similar, but let's keep it simple for now.
		Timezone = "UTC"
	}

	RedisURL = getEnv("REDIS_URL", envConfig, "")
}

func getEnv(key string, envConfig map[string]string, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if val, ok := envConfig[key]; ok {
		return val
	}
	return defaultValue
}

func getIntEnv(key string, envConfig map[string]string, defaultValue int) int {
	valStr := getEnv(key, envConfig, "")
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}
