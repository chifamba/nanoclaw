package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/robfig/cron/v3"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/container"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

// IpcDeps defines the dependencies required by the IPC watcher.
type IpcDeps interface {
	SendMessage(ctx context.Context, jid string, text string) error
	RegisteredGroups() map[string]types.RegisteredGroup
	RegisterGroup(jid string, group types.RegisteredGroup) error
	SyncGroups(force bool) error
	GetAvailableGroups() []AvailableGroup
	WriteGroupsSnapshot(groupFolder string, isMain bool, availableGroups []AvailableGroup, registeredJids []string) error
	GetTaskByID(id string) (*types.ScheduledTask, error)
	CreateTask(task types.ScheduledTask) error
	UpdateTask(id string, updates map[string]interface{}) error
	DeleteTask(id string) error
}

type Watcher struct {
	deps    IpcDeps
	baseDir string
	watcher *fsnotify.Watcher
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewWatcher(deps IpcDeps) *Watcher {
	return &Watcher{
		deps:    deps,
		baseDir: filepath.Join(config.DataDir, "ipc"),
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	if err := os.MkdirAll(w.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create IPC base directory: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}
	w.watcher = watcher

	// Initial scan and setup watches
	if err := w.setupInitialWatches(); err != nil {
		watcher.Close()
		return err
	}

	go w.watchLoop()

	// Also run periodic polling as a fallback and to catch new directories if fsnotify misses them
	go w.pollLoop()

	logger.Info("IPC watcher started (per-group namespaces)")
	return nil
}

func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.watcher != nil {
		w.watcher.Close()
	}
}

func (w *Watcher) setupInitialWatches() error {
	// Watch the base directory for new group folders
	if err := w.watcher.Add(w.baseDir); err != nil {
		return fmt.Errorf("failed to watch base dir: %w", err)
	}

	entries, err := ioutil.ReadDir(w.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read IPC base directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "errors" {
			w.watchGroupDir(entry.Name())
		}
	}

	return nil
}

func (w *Watcher) watchGroupDir(groupFolder string) {
	groupPath := filepath.Join(w.baseDir, groupFolder)
	messagesDir := filepath.Join(groupPath, "messages")
	tasksDir := filepath.Join(groupPath, "tasks")

	os.MkdirAll(messagesDir, 0755)
	os.MkdirAll(tasksDir, 0755)

	if err := w.watcher.Add(messagesDir); err != nil {
		logger.Error("Failed to watch messages directory", err, "group", groupFolder)
	}
	if err := w.watcher.Add(tasksDir); err != nil {
		logger.Error("Failed to watch tasks directory", err, "group", groupFolder)
	}

	// Immediate process existing files
	w.processGroupFiles(groupFolder)
}

func (w *Watcher) watchLoop() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					// Check if it's a new group directory or a messages/tasks dir
					parent := filepath.Dir(event.Name)
					if parent == w.baseDir {
						if filepath.Base(event.Name) != "errors" {
							w.watchGroupDir(filepath.Base(event.Name))
						}
					}
				} else if strings.HasSuffix(event.Name, ".json") {
					// It's a file, find out which group it belongs to
					rel, err := filepath.Rel(w.baseDir, event.Name)
					if err == nil {
						parts := strings.Split(rel, string(os.PathSeparator))
						if len(parts) >= 3 {
							w.processGroupFiles(parts[0])
						}
					}
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("IPC watcher error", err)
		}
	}
}

func (w *Watcher) pollLoop() {
	ticker := time.NewTicker(time.Duration(config.IPCPollInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processAllGroups()
		}
	}
}

func (w *Watcher) processAllGroups() {
	entries, err := ioutil.ReadDir(w.baseDir)
	if err != nil {
		logger.Error("Error reading IPC base directory", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "errors" {
			w.processGroupFiles(entry.Name())
		}
	}
}

func (w *Watcher) processGroupFiles(sourceGroup string) {
	registeredGroups := w.deps.RegisteredGroups()
	
	// Determine if this group is main
	isMain := false
	for _, group := range registeredGroups {
		if group.Folder == sourceGroup && group.IsMain {
			isMain = true
			break
		}
	}

	messagesDir := filepath.Join(w.baseDir, sourceGroup, "messages")
	tasksDir := filepath.Join(w.baseDir, sourceGroup, "tasks")

	w.processDir(messagesDir, sourceGroup, isMain, "message")
	w.processDir(tasksDir, sourceGroup, isMain, "task")
}

func (w *Watcher) processDir(dirPath string, sourceGroup string, isMain bool, dirType string) {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return
	}

	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		logger.Error("Error reading directory", err, "path", dirPath)
		return
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			filePath := filepath.Join(dirPath, file.Name())
			w.processFile(filePath, sourceGroup, isMain, dirType)
		}
	}
}

func (w *Watcher) processFile(filePath string, sourceGroup string, isMain bool, dirType string) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		logger.Error("Error reading IPC file", err, "path", filePath)
		return
	}

	var processed bool
	if dirType == "message" {
		processed = w.handleMessage(data, sourceGroup, isMain)
	} else {
		processed = w.handleTask(data, sourceGroup, isMain)
	}

	if processed {
		if err := os.Remove(filePath); err != nil {
			logger.Error("Error removing IPC file", err, "path", filePath)
		}
	} else {
		// If not processed due to error (not authorization), move to errors
		errorDir := filepath.Join(w.baseDir, "errors")
		os.MkdirAll(errorDir, 0755)
		newName := fmt.Sprintf("%s-%s", sourceGroup, filepath.Base(filePath))
		if err := os.Rename(filePath, filepath.Join(errorDir, newName)); err != nil {
			logger.Error("Error moving file to error directory", err, "path", filePath)
		}
	}
}

func (w *Watcher) handleMessage(data []byte, sourceGroup string, isMain bool) bool {
	var msg MessageEvent
	if err := json.Unmarshal(data, &msg); err != nil {
		logger.Error("Error parsing IPC message", err, "sourceGroup", sourceGroup)
		return false
	}

	if msg.Type != "message" || msg.ChatJID == "" || msg.Text == "" {
		return false
	}

	registeredGroups := w.deps.RegisteredGroups()
	targetGroup, exists := registeredGroups[msg.ChatJID]

	// Authorization: verify this group can send to this chatJID
	if isMain || (exists && targetGroup.Folder == sourceGroup) {
		err := w.deps.SendMessage(w.ctx, msg.ChatJID, msg.Text)
		if err != nil {
			logger.Error("Error sending IPC message", err, "chatJid", msg.ChatJID, "sourceGroup", sourceGroup)
			return true
		}
		logger.Info("IPC message sent", "chatJid", msg.ChatJID, "sourceGroup", sourceGroup)
		return true
	} else {
		logger.Warn("Unauthorized IPC message attempt blocked", "chatJid", msg.ChatJID, "sourceGroup", sourceGroup)
		return true // TS version unlinks it even if unauthorized
	}
}

func (w *Watcher) handleTask(data []byte, sourceGroup string, isMain bool) bool {
	var event TaskEvent
	if err := json.Unmarshal(data, &event); err != nil {
		logger.Error("Error parsing IPC task", err, "sourceGroup", sourceGroup)
		return false
	}

	err := w.ProcessTaskIpc(event, sourceGroup, isMain)
	if err != nil {
		logger.Error("Error processing IPC task", err, "sourceGroup", sourceGroup)
		return false
	}
	return true
}

func (w *Watcher) ProcessTaskIpc(event TaskEvent, sourceGroup string, isMain bool) error {
	registeredGroups := w.deps.RegisteredGroups()

	switch event.Type {
	case "schedule_task":
		if event.Prompt != "" && event.ScheduleType != "" && event.ScheduleValue != "" && event.TargetJID != "" {
			targetGroupEntry, exists := registeredGroups[event.TargetJID]
			if !exists {
				logger.Warn("Cannot schedule task: target group not registered", "targetJid", event.TargetJID)
				return nil
			}

			targetFolder := targetGroupEntry.Folder
			if !isMain && targetFolder != sourceGroup {
				logger.Warn("Unauthorized schedule_task attempt blocked", "sourceGroup", sourceGroup, "targetFolder", targetFolder)
				return nil
			}

			var nextRun *string
			loc, _ := time.LoadLocation(config.Timezone)
			if loc == nil {
				loc = time.UTC
			}

			switch event.ScheduleType {
			case "cron":
				p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
				sched, err := p.Parse(event.ScheduleValue)
				if err != nil {
					logger.Warn("Invalid cron expression", "scheduleValue", event.ScheduleValue)
					return nil
				}
				nextRunTime := sched.Next(time.Now().In(loc))
				nextRunStr := nextRunTime.Format(time.RFC3339)
				nextRun = &nextRunStr
			case "interval":
				ms, err := strconv.ParseInt(event.ScheduleValue, 10, 64)
				if err != nil || ms <= 0 {
					logger.Warn("Invalid interval", "scheduleValue", event.ScheduleValue)
					return nil
				}
				nextRunTime := time.Now().In(loc).Add(time.Duration(ms) * time.Millisecond)
				nextRunStr := nextRunTime.Format(time.RFC3339)
				nextRun = &nextRunStr
			case "once":
				t, err := time.Parse(time.RFC3339, event.ScheduleValue)
				if err != nil {
					logger.Warn("Invalid timestamp", "scheduleValue", event.ScheduleValue)
					return nil
				}
				nextRunStr := t.Format(time.RFC3339)
				nextRun = &nextRunStr
			}

			taskID := event.TaskID
			if taskID == "" {
				taskID = fmt.Sprintf("task-%d-%s", time.Now().Unix(), strings.ToLower(event.Type[0:4]))
			}

			contextMode := event.ContextMode
			if contextMode != "group" && contextMode != "isolated" {
				contextMode = "isolated"
			}

			err := w.deps.CreateTask(types.ScheduledTask{
				ID:            taskID,
				GroupFolder:   targetFolder,
				ChatJID:       event.TargetJID,
				Prompt:        event.Prompt,
				ScheduleType:  event.ScheduleType,
				ScheduleValue: event.ScheduleValue,
				ContextMode:   contextMode,
				NextRun:       nextRun,
				Status:        "active",
				CreatedAt:     time.Now().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			logger.Info("Task created via IPC", "taskId", taskID, "sourceGroup", sourceGroup, "targetFolder", targetFolder, "contextMode", contextMode)
		}

	case "pause_task":
		if event.TaskID != "" {
			task, err := w.deps.GetTaskByID(event.TaskID)
			if err == nil && task != nil && (isMain || task.GroupFolder == sourceGroup) {
				w.deps.UpdateTask(event.TaskID, map[string]interface{}{"status": "paused"})
				logger.Info("Task paused via IPC", "taskId", event.TaskID, "sourceGroup", sourceGroup)
			} else {
				logger.Warn("Unauthorized task pause attempt", "taskId", event.TaskID, "sourceGroup", sourceGroup)
			}
		}

	case "resume_task":
		if event.TaskID != "" {
			task, err := w.deps.GetTaskByID(event.TaskID)
			if err == nil && task != nil && (isMain || task.GroupFolder == sourceGroup) {
				w.deps.UpdateTask(event.TaskID, map[string]interface{}{"status": "active"})
				logger.Info("Task resumed via IPC", "taskId", event.TaskID, "sourceGroup", sourceGroup)
			} else {
				logger.Warn("Unauthorized task resume attempt", "taskId", event.TaskID, "sourceGroup", sourceGroup)
			}
		}

	case "cancel_task":
		if event.TaskID != "" {
			task, err := w.deps.GetTaskByID(event.TaskID)
			if err == nil && task != nil && (isMain || task.GroupFolder == sourceGroup) {
				w.deps.DeleteTask(event.TaskID)
				logger.Info("Task cancelled via IPC", "taskId", event.TaskID, "sourceGroup", sourceGroup)
			} else {
				logger.Warn("Unauthorized task cancel attempt", "taskId", event.TaskID, "sourceGroup", sourceGroup)
			}
		}

	case "update_task":
		if event.TaskID != "" {
			task, err := w.deps.GetTaskByID(event.TaskID)
			if err != nil || task == nil {
				logger.Warn("Task not found for update", "taskId", event.TaskID, "sourceGroup", sourceGroup)
				return nil
			}
			if !isMain && task.GroupFolder != sourceGroup {
				logger.Warn("Unauthorized task update attempt", "taskId", event.TaskID, "sourceGroup", sourceGroup)
				return nil
			}

			updates := make(map[string]interface{})
			if event.Prompt != "" {
				updates["prompt"] = event.Prompt
			}
			if event.ScheduleType != "" {
				updates["schedule_type"] = event.ScheduleType
			}
			if event.ScheduleValue != "" {
				updates["schedule_value"] = event.ScheduleValue
			}

			if event.ScheduleType != "" || event.ScheduleValue != "" {
				schedType := task.ScheduleType
				if event.ScheduleType != "" {
					schedType = event.ScheduleType
				}
				schedValue := task.ScheduleValue
				if event.ScheduleValue != "" {
					schedValue = event.ScheduleValue
				}

				loc, _ := time.LoadLocation(config.Timezone)
				if loc == nil {
					loc = time.UTC
				}

				if schedType == "cron" {
					p := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
					sched, err := p.Parse(schedValue)
					if err == nil {
						nextRunTime := sched.Next(time.Now().In(loc))
						nextRunStr := nextRunTime.Format(time.RFC3339)
						updates["next_run"] = nextRunStr
					}
				} else if schedType == "interval" {
					ms, err := strconv.ParseInt(schedValue, 10, 64)
					if err == nil && ms > 0 {
						nextRunTime := time.Now().In(loc).Add(time.Duration(ms) * time.Millisecond)
						nextRunStr := nextRunTime.Format(time.RFC3339)
						updates["next_run"] = nextRunStr
					}
				}
			}

			w.deps.UpdateTask(event.TaskID, updates)
			logger.Info("Task updated via IPC", "taskId", event.TaskID, "sourceGroup", sourceGroup)
		}

	case "refresh_groups":
		if isMain {
			logger.Info("Group metadata refresh requested via IPC", "sourceGroup", sourceGroup)
			w.deps.SyncGroups(true)
			availableGroups := w.deps.GetAvailableGroups()
			registeredJids := []string{}
			for jid := range registeredGroups {
				registeredJids = append(registeredJids, jid)
			}
			w.deps.WriteGroupsSnapshot(sourceGroup, true, availableGroups, registeredJids)
		} else {
			logger.Warn("Unauthorized refresh_groups attempt blocked", "sourceGroup", sourceGroup)
		}

	case "register_group":
		if !isMain {
			logger.Warn("Unauthorized register_group attempt blocked", "sourceGroup", sourceGroup)
			return nil
		}
		if event.JID != "" && event.Name != "" && event.Folder != "" && event.Trigger != "" {
			if !container.IsValidGroupFolder(event.Folder) {
				logger.Warn("Invalid register_group request - unsafe folder name", "sourceGroup", sourceGroup, "folder", event.Folder)
				return nil
			}

			var config *types.ContainerConfig
			if event.ContainerConfig != nil {
				json.Unmarshal(event.ContainerConfig, &config)
			}

			w.deps.RegisterGroup(event.JID, types.RegisteredGroup{
				Name:            event.Name,
				Folder:          event.Folder,
				Trigger:         event.Trigger,
				AddedAt:         time.Now().Format(time.RFC3339),
				ContainerConfig: config,
				RequiresTrigger: event.RequiresTrigger,
			})
		} else {
			logger.Warn("Invalid register_group request - missing required fields", "sourceGroup", sourceGroup)
		}

	default:
		logger.Warn("Unknown IPC task type", "type", event.Type)
	}

	return nil
}

func getSortedKeys(m map[string]types.RegisteredGroup) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
