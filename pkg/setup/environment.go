package setup

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	_ "github.com/mattn/go-sqlite3"
)

// CheckEnvironment detects OS, container runtimes, and existing configuration.
// It ports the logic from setup/environment.ts.
func CheckEnvironment() {
	logger.Info("Starting environment check")

	platform := GetPlatform()
	wsl := IsWSL()
	headless := IsHeadless()

	// Check Apple Container
	appleContainer := "not_found"
	if CommandExists("container") {
		appleContainer = "installed"
	}

	// Check Docker
	docker := "not_found"
	if CommandExists("docker") {
		// exec.Command("docker", "info").Run() will return non-nil error if docker is not running
		if err := exec.Command("docker", "info").Run(); err == nil {
			docker = "running"
		} else {
			docker = "installed_not_running"
		}
	}

	// Check existing config
	projectRoot, _ := os.Getwd()
	hasEnv := false
	if _, err := os.Stat(filepath.Join(projectRoot, ".env")); err == nil {
		hasEnv = true
	}

	authDir := filepath.Join(projectRoot, "store", "auth")
	hasAuth := false
	if info, err := os.Stat(authDir); err == nil && info.IsDir() {
		files, err := os.ReadDir(authDir)
		if err == nil && len(files) > 0 {
			hasAuth = true
		}
	}

	hasRegisteredGroups := false
	// Check JSON file first (pre-migration)
	if _, err := os.Stat(filepath.Join(config.DataDir, "registered_groups.json")); err == nil {
		hasRegisteredGroups = true
	} else {
		// Check SQLite directly
		dbPath := filepath.Join(config.StoreDir, "messages.db")
		if _, err := os.Stat(dbPath); err == nil {
			db, err := sql.Open("sqlite3", dbPath)
			if err == nil {
				defer db.Close()
				var count int
				// We don't want to log error here as table might not exist yet
				err = db.QueryRow("SELECT COUNT(*) FROM registered_groups").Scan(&count)
				if err == nil && count > 0 {
					hasRegisteredGroups = true
				}
			}
		}
	}

	logger.Info("Environment check complete", map[string]interface{}{
		"platform":              platform,
		"wsl":                   wsl,
		"appleContainer":        appleContainer,
		"docker":                docker,
		"hasEnv":                hasEnv,
		"hasAuth":               hasAuth,
		"hasRegisteredGroups":   hasRegisteredGroups,
	})

	EmitStatus("CHECK_ENVIRONMENT", map[string]interface{}{
		"PLATFORM":              platform,
		"IS_WSL":                wsl,
		"IS_HEADLESS":           headless,
		"APPLE_CONTAINER":       appleContainer,
		"DOCKER":                docker,
		"HAS_ENV":               hasEnv,
		"HAS_AUTH":              hasAuth,
		"HAS_REGISTERED_GROUPS": hasRegisteredGroups,
		"STATUS":                "success",
		"LOG":                   "logs/setup.log",
	})
}
