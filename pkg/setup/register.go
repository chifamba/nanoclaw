package setup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/db"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

type RegisterArgs struct {
	JID             string
	Name            string
	Trigger         string
	Folder          string
	Channel         string
	RequiresTrigger bool
	IsMain          bool
	AssistantName   string
}

func parseRegisterArgs(args []string) RegisterArgs {
	result := RegisterArgs{
		Channel:         "whatsapp",
		RequiresTrigger: true,
		AssistantName:   "Andy",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--jid":
			if i+1 < len(args) {
				result.JID = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				result.Name = args[i+1]
				i++
			}
		case "--trigger":
			if i+1 < len(args) {
				result.Trigger = args[i+1]
				i++
			}
		case "--folder":
			if i+1 < len(args) {
				result.Folder = args[i+1]
				i++
			}
		case "--channel":
			if i+1 < len(args) {
				result.Channel = strings.ToLower(args[i+1])
				i++
			}
		case "--no-trigger-required":
			result.RequiresTrigger = false
		case "--is-main":
			result.IsMain = true
		case "--assistant-name":
			if i+1 < len(args) {
				result.AssistantName = args[i+1]
				i++
			}
		}
	}

	return result
}

func Register(args []string) {
	projectRoot, _ := os.Getwd()
	parsed := parseRegisterArgs(args)

	if parsed.JID == "" || parsed.Name == "" || parsed.Trigger == "" || parsed.Folder == "" {
		EmitStatus("REGISTER_CHANNEL", map[string]interface{}{
			"STATUS": "failed",
			"ERROR":  "missing_required_args",
			"LOG":    "logs/setup.log",
		})
		os.Exit(4)
	}

	if !db.IsValidGroupFolder(parsed.Folder) {
		EmitStatus("REGISTER_CHANNEL", map[string]interface{}{
			"STATUS": "failed",
			"ERROR":  "invalid_folder",
			"LOG":    "logs/setup.log",
		})
		os.Exit(4)
	}

	logger.Info("Registering channel", parsed)

	// Ensure data and store directories exist
	os.MkdirAll(filepath.Join(projectRoot, "data"), 0755)
	os.MkdirAll(config.StoreDir, 0755)

	// Initialize database
	dbPath := filepath.Join(config.StoreDir, "messages.db")
	storage, err := db.NewSQLiteStorage(dbPath)
	if err != nil {
		logger.Error("Failed to initialize database", err)
		EmitStatus("REGISTER_CHANNEL", map[string]interface{}{
			"STATUS": "failed",
			"ERROR":  "db_init_failed",
			"LOG":    "logs/setup.log",
		})
		os.Exit(1)
	}
	defer storage.Close()

	err = storage.SetRegisteredGroup(parsed.JID, types.RegisteredGroup{
		Name:            parsed.Name,
		Folder:          parsed.Folder,
		Trigger:         parsed.Trigger,
		AddedAt:         time.Now().Format(time.RFC3339),
		RequiresTrigger: &parsed.RequiresTrigger,
		IsMain:          parsed.IsMain,
	})
	if err != nil {
		logger.Error("Failed to write registration to SQLite", err)
		EmitStatus("REGISTER_CHANNEL", map[string]interface{}{
			"STATUS": "failed",
			"ERROR":  "db_write_failed",
			"LOG":    "logs/setup.log",
		})
		os.Exit(1)
	}

	logger.Info("Wrote registration to SQLite")

	// Create group folders
	os.MkdirAll(filepath.Join(projectRoot, "groups", parsed.Folder, "logs"), 0755)

	nameUpdated := false
	if parsed.AssistantName != "Andy" {
		logger.Info("Updating assistant name", map[string]string{"from": "Andy", "to": parsed.AssistantName})

		mdFiles := []string{
			filepath.Join(projectRoot, "groups", "global", "CLAUDE.md"),
			filepath.Join(projectRoot, "groups", parsed.Folder, "CLAUDE.md"),
		}

		for _, mdFile := range mdFiles {
			if _, err := os.Stat(mdFile); err == nil {
				content, err := os.ReadFile(mdFile)
				if err == nil {
					newContent := bytes.ReplaceAll(content, []byte("# Andy"), []byte("# "+parsed.AssistantName))
					newContent = bytes.ReplaceAll(newContent, []byte("You are Andy"), []byte("You are "+parsed.AssistantName))
					os.WriteFile(mdFile, newContent, 0644)
					logger.Info("Updated CLAUDE.md", map[string]string{"file": mdFile})
				}
			}
		}

		// Update .env
		envFile := filepath.Join(projectRoot, ".env")
		assistantNameLine := fmt.Sprintf("ASSISTANT_NAME=\"%s\"", parsed.AssistantName)
		if _, err := os.Stat(envFile); err == nil {
			content, err := os.ReadFile(envFile)
			if err == nil {
				re := regexp.MustCompile(`(?m)^ASSISTANT_NAME=.*$`)
				var newContent []byte
				if re.Match(content) {
					newContent = re.ReplaceAll(content, []byte(assistantNameLine))
				} else {
					newContent = append(content, []byte("\n"+assistantNameLine)...)
				}
				os.WriteFile(envFile, newContent, 0644)
			}
		} else {
			os.WriteFile(envFile, []byte(assistantNameLine+"\n"), 0644)
		}
		logger.Info("Set ASSISTANT_NAME in .env")
		nameUpdated = true
	}

	EmitStatus("REGISTER_CHANNEL", map[string]interface{}{
		"JID":              parsed.JID,
		"NAME":             parsed.Name,
		"FOLDER":           parsed.Folder,
		"CHANNEL":          parsed.Channel,
		"TRIGGER":          parsed.Trigger,
		"REQUIRES_TRIGGER": parsed.RequiresTrigger,
		"ASSISTANT_NAME":   parsed.AssistantName,
		"NAME_UPDATED":     nameUpdated,
		"STATUS":           "success",
		"LOG":              "logs/setup.log",
	})
}
