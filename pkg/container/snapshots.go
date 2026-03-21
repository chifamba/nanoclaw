package container

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/types"
)

// AvailableGroup for snapshots
type AvailableGroup struct {
	JID          string `json:"jid"`
	Name         string `json:"name"`
	LastActivity string `json:"lastActivity"`
	IsRegistered bool   `json:"isRegistered"`
}

// WriteTasksSnapshot writes filtered tasks to the group's IPC directory
func WriteTasksSnapshot(groupFolder string, isMain bool, tasks []types.ScheduledTask) error {
	groupIpcDir := ResolveGroupIpcPath(groupFolder)
	os.MkdirAll(groupIpcDir, 0755)

	var filteredTasks []types.ScheduledTask
	if isMain {
		filteredTasks = tasks
	} else {
		for _, t := range tasks {
			if t.GroupFolder == groupFolder {
				filteredTasks = append(filteredTasks, t)
			}
		}
	}

	tasksFile := filepath.Join(groupIpcDir, "current_tasks.json")
	data, err := json.MarshalIndent(filteredTasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tasksFile, data, 0644)
}

// WriteGroupsSnapshot writes available groups snapshot for the container to read
func WriteGroupsSnapshot(groupFolder string, isMain bool, groups []AvailableGroup) error {
	groupIpcDir := ResolveGroupIpcPath(groupFolder)
	os.MkdirAll(groupIpcDir, 0755)

	visibleGroups := []AvailableGroup{}
	if isMain {
		visibleGroups = groups
	}

	snapshot := map[string]interface{}{
		"groups":   visibleGroups,
		"lastSync": time.Now().Format(time.RFC3339),
	}

	groupsFile := filepath.Join(groupIpcDir, "available_groups.json")
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(groupsFile, data, 0644)
}
