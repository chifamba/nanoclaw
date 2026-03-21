package ipc

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

type mockIpcDeps struct {
	sentMessages     []struct{ jid, text string }
	registeredGroups map[string]types.RegisteredGroup
	tasks           map[string]*types.ScheduledTask
	syncCount       int
	snapshots       []struct {
		folder          string
		isMain          bool
		availableGroups []AvailableGroup
		registeredJids  []string
	}
}

func (m *mockIpcDeps) SendMessage(ctx context.Context, jid string, text string) error {
	m.sentMessages = append(m.sentMessages, struct{ jid, text string }{jid, text})
	return nil
}

func (m *mockIpcDeps) RegisteredGroups() map[string]types.RegisteredGroup {
	return m.registeredGroups
}

func (m *mockIpcDeps) RegisterGroup(jid string, group types.RegisteredGroup) error {
	m.registeredGroups[jid] = group
	return nil
}

func (m *mockIpcDeps) SyncGroups(force bool) error {
	m.syncCount++
	return nil
}

func (m *mockIpcDeps) GetAvailableGroups() []AvailableGroup {
	return []AvailableGroup{
		{JID: "group1@g.us", Name: "Group 1"},
	}
}

func (m *mockIpcDeps) WriteGroupsSnapshot(groupFolder string, isMain bool, availableGroups []AvailableGroup, registeredJids []string) error {
	m.snapshots = append(m.snapshots, struct {
		folder          string
		isMain          bool
		availableGroups []AvailableGroup
		registeredJids  []string
	}{groupFolder, isMain, availableGroups, registeredJids})
	return nil
}

func (m *mockIpcDeps) GetTaskByID(id string) (*types.ScheduledTask, error) {
	return m.tasks[id], nil
}

func (m *mockIpcDeps) CreateTask(task types.ScheduledTask) error {
	m.tasks[task.ID] = &task
	return nil
}

func (m *mockIpcDeps) UpdateTask(id string, updates map[string]interface{}) error {
	task := m.tasks[id]
	if task == nil {
		return nil
	}
	if v, ok := updates["status"].(string); ok {
		task.Status = v
	}
	if v, ok := updates["prompt"].(string); ok {
		task.Prompt = v
	}
	if v, ok := updates["next_run"].(string); ok {
		task.NextRun = &v
	}
	return nil
}

func (m *mockIpcDeps) DeleteTask(id string) error {
	delete(m.tasks, id)
	return nil
}

func TestWatcher_MessageProcessing(t *testing.T) {
	tempDir := t.TempDir()
	config.DataDir = tempDir
	// p.IPCPollInterval = 100

	deps := &mockIpcDeps{
		registeredGroups: map[string]types.RegisteredGroup{
			"main@g.us": {Folder: "main", IsMain: true},
			"sub@g.us":  {Folder: "sub", IsMain: false},
		},
		tasks: make(map[string]*types.ScheduledTask),
	}

	watcher := NewWatcher(deps)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	ipcBaseDir := filepath.Join(tempDir, "ipc")
	mainMsgDir := filepath.Join(ipcBaseDir, "main", "messages")
	subMsgDir := filepath.Join(ipcBaseDir, "sub", "messages")

	os.MkdirAll(mainMsgDir, 0755)
	os.MkdirAll(subMsgDir, 0755)

	// 1. Main group sending to any JID (authorized)
	msg1 := MessageEvent{Type: "message", ChatJID: "any@g.us", Text: "hello from main"}
	data1, _ := json.Marshal(msg1)
	ioutil.WriteFile(filepath.Join(mainMsgDir, "msg1.json"), data1, 0644)

	// 2. Sub group sending to itself (authorized)
	msg2 := MessageEvent{Type: "message", ChatJID: "sub@g.us", Text: "hello from sub to sub"}
	data2, _ := json.Marshal(msg2)
	ioutil.WriteFile(filepath.Join(subMsgDir, "msg2.json"), data2, 0644)

	// 3. Sub group sending to another JID (unauthorized)
	msg3 := MessageEvent{Type: "message", ChatJID: "other@g.us", Text: "hello from sub to other"}
	data3, _ := json.Marshal(msg3)
	ioutil.WriteFile(filepath.Join(subMsgDir, "msg3.json"), data3, 0644)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	if len(deps.sentMessages) != 2 {
		t.Errorf("expected 2 sent messages, got %d", len(deps.sentMessages))
	}

	// Verify cleanup
	if _, err := os.Stat(filepath.Join(mainMsgDir, "msg1.json")); !os.IsNotExist(err) {
		t.Errorf("msg1.json should have been deleted")
	}
	if _, err := os.Stat(filepath.Join(subMsgDir, "msg2.json")); !os.IsNotExist(err) {
		t.Errorf("msg2.json should have been deleted")
	}
	if _, err := os.Stat(filepath.Join(subMsgDir, "msg3.json")); !os.IsNotExist(err) {
		t.Errorf("msg3.json should have been deleted (even if unauthorized)")
	}
}

func TestWatcher_TaskScheduling(t *testing.T) {
	tempDir := t.TempDir()
	config.DataDir = tempDir
	// p.IPCPollInterval = 100
	config.Timezone = "UTC"

	deps := &mockIpcDeps{
		registeredGroups: map[string]types.RegisteredGroup{
			"main@g.us": {Folder: "main", IsMain: true},
			"sub@g.us":  {Folder: "sub", IsMain: false},
		},
		tasks: make(map[string]*types.ScheduledTask),
	}

	watcher := NewWatcher(deps)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	ipcBaseDir := filepath.Join(tempDir, "ipc")
	mainTaskDir := filepath.Join(ipcBaseDir, "main", "tasks")
	subTaskDir := filepath.Join(ipcBaseDir, "sub", "tasks")

	os.MkdirAll(mainTaskDir, 0755)
	os.MkdirAll(subTaskDir, 0755)

	// 1. Schedule interval task (authorized)
	task1 := TaskEvent{
		Type:          "schedule_task",
		TaskID:        "task1",
		Prompt:        "test prompt",
		ScheduleType:  "interval",
		ScheduleValue: "60000",
		TargetJID:     "sub@g.us",
	}
	data1, _ := json.Marshal(task1)
	ioutil.WriteFile(filepath.Join(mainTaskDir, "task1.json"), data1, 0644)

	// 2. Schedule cron task (authorized)
	task2 := TaskEvent{
		Type:          "schedule_task",
		TaskID:        "task2",
		Prompt:        "cron prompt",
		ScheduleType:  "cron",
		ScheduleValue: "*/5 * * * *",
		TargetJID:     "sub@g.us",
	}
	data2, _ := json.Marshal(task2)
	ioutil.WriteFile(filepath.Join(subTaskDir, "task2.json"), data2, 0644)

	// 3. Unauthorized schedule (sub trying to schedule for main)
	task3 := TaskEvent{
		Type:          "schedule_task",
		TaskID:        "task3",
		Prompt:        "unauthorized",
		ScheduleType:  "interval",
		ScheduleValue: "60000",
		TargetJID:     "main@g.us",
	}
	data3, _ := json.Marshal(task3)
	ioutil.WriteFile(filepath.Join(subTaskDir, "task3.json"), data3, 0644)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	if len(deps.tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(deps.tasks))
	}

	if deps.tasks["task1"] == nil || deps.tasks["task1"].ScheduleType != "interval" {
		t.Errorf("task1 not created correctly")
	}
	if deps.tasks["task2"] == nil || deps.tasks["task2"].ScheduleType != "cron" {
		t.Errorf("task2 not created correctly")
	}
	if deps.tasks["task3"] != nil {
		t.Errorf("task3 should not have been created")
	}
}

func TestWatcher_TaskManagement(t *testing.T) {
	tempDir := t.TempDir()
	config.DataDir = tempDir
	// p.IPCPollInterval = 100

	deps := &mockIpcDeps{
		registeredGroups: map[string]types.RegisteredGroup{
			"main@g.us": {Folder: "main", IsMain: true},
			"sub@g.us":  {Folder: "sub", IsMain: false},
		},
		tasks: map[string]*types.ScheduledTask{
			"t1": {ID: "t1", GroupFolder: "sub", Status: "active"},
			"t2": {ID: "t2", GroupFolder: "main", Status: "active"},
		},
	}

	watcher := NewWatcher(deps)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	subTaskDir := filepath.Join(tempDir, "ipc", "sub", "tasks")
	os.MkdirAll(subTaskDir, 0755)

	// 1. Pause authorized task
	p1 := TaskEvent{Type: "pause_task", TaskID: "t1"}
	d1, _ := json.Marshal(p1)
	ioutil.WriteFile(filepath.Join(subTaskDir, "p1.json"), d1, 0644)

	// 2. Pause unauthorized task
	p2 := TaskEvent{Type: "pause_task", TaskID: "t2"}
	d2, _ := json.Marshal(p2)
	ioutil.WriteFile(filepath.Join(subTaskDir, "p2.json"), d2, 0644)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	if deps.tasks["t1"].Status != "paused" {
		t.Errorf("t1 should be paused")
	}
	if deps.tasks["t2"].Status != "active" {
		t.Errorf("t2 should still be active")
	}
}

func TestWatcher_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	config.DataDir = tempDir
	// p.IPCPollInterval = 100

	deps := &mockIpcDeps{
		registeredGroups: map[string]types.RegisteredGroup{
			"main@g.us": {Folder: "main", IsMain: true},
		},
		tasks: make(map[string]*types.ScheduledTask),
	}

	watcher := NewWatcher(deps)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	mainMsgDir := filepath.Join(tempDir, "ipc", "main", "messages")
	os.MkdirAll(mainMsgDir, 0755)

	// Invalid JSON
	ioutil.WriteFile(filepath.Join(mainMsgDir, "bad.json"), []byte("{invalid}"), 0644)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	errorDir := filepath.Join(tempDir, "ipc", "errors")
	if _, err := os.Stat(filepath.Join(errorDir, "main-bad.json")); os.IsNotExist(err) {
		t.Errorf("bad.json should have been moved to errors")
	}
}
