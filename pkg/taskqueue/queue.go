package taskqueue

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
)

var (
	MaxRetries   = 5
	BaseRetryMs  = 5000
)

type QueuedTask struct {
	ID       string
	GroupJID string
	Fn       func() error
}

type groupState struct {
	active          bool
	idleWaiting     bool
	isTaskContainer bool
	runningTaskID   string
	pendingMessages bool
	pendingTasks    []QueuedTask
	containerName   string
	groupFolder     string
	retryCount      int
}

type GroupQueue struct {
	mu                sync.Mutex
	groups            map[string]*groupState
	activeCount       int
	waitingGroups     []string
	processMessagesFn func(groupJid string) (bool, error)
	shuttingDown      bool
}

func NewGroupQueue() *GroupQueue {
	return &GroupQueue{
		groups: make(map[string]*groupState),
	}
}

func (q *GroupQueue) getGroup(groupJid string) *groupState {
	state, ok := q.groups[groupJid]
	if !ok {
		state = &groupState{
			pendingTasks: make([]QueuedTask, 0),
		}
		q.groups[groupJid] = state
	}
	return state
}

func (q *GroupQueue) SetProcessMessagesFn(fn func(groupJid string) (bool, error)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.processMessagesFn = fn
}

func (q *GroupQueue) EnqueueMessageCheck(groupJid string) {
	q.mu.Lock()
	if q.shuttingDown {
		q.mu.Unlock()
		return
	}

	state := q.getGroup(groupJid)

	if state.active {
		state.pendingMessages = true
		logger.Debug("Container active, message queued", "groupJid", groupJid)
		q.mu.Unlock()
		return
	}

	if q.activeCount >= config.MaxConcurrentContainers {
		state.pendingMessages = true
		found := false
		for _, g := range q.waitingGroups {
			if g == groupJid {
				found = true
				break
			}
		}
		if !found {
			q.waitingGroups = append(q.waitingGroups, groupJid)
		}
		logger.Debug("At concurrency limit, message queued", "groupJid", groupJid, "activeCount", q.activeCount)
		q.mu.Unlock()
		return
	}

	state.active = true
	state.idleWaiting = false
	state.isTaskContainer = false
	state.pendingMessages = false
	q.activeCount++

	q.mu.Unlock()
	go func() {
		if err := q.runForGroup(groupJid, "messages"); err != nil {
			logger.Error("Unhandled error in runForGroup", "groupJid", groupJid, "err", err)
		}
	}()
}

func (q *GroupQueue) EnqueueTask(groupJid string, taskID string, fn func() error) {
	q.mu.Lock()
	if q.shuttingDown {
		q.mu.Unlock()
		return
	}

	state := q.getGroup(groupJid)

	if state.runningTaskID == taskID {
		logger.Debug("Task already running, skipping", "groupJid", groupJid, "taskID", taskID)
		q.mu.Unlock()
		return
	}
	for _, t := range state.pendingTasks {
		if t.ID == taskID {
			logger.Debug("Task already queued, skipping", "groupJid", groupJid, "taskID", taskID)
			q.mu.Unlock()
			return
		}
	}

	if state.active {
		state.pendingTasks = append(state.pendingTasks, QueuedTask{ID: taskID, GroupJID: groupJid, Fn: fn})
		if state.idleWaiting {
			q.mu.Unlock()
			q.CloseStdin(groupJid)
			q.mu.Lock()
		}
		logger.Debug("Container active, task queued", "groupJid", groupJid, "taskID", taskID)
		q.mu.Unlock()
		return
	}

	if q.activeCount >= config.MaxConcurrentContainers {
		state.pendingTasks = append(state.pendingTasks, QueuedTask{ID: taskID, GroupJID: groupJid, Fn: fn})
		found := false
		for _, g := range q.waitingGroups {
			if g == groupJid {
				found = true
				break
			}
		}
		if !found {
			q.waitingGroups = append(q.waitingGroups, groupJid)
		}
		logger.Debug("At concurrency limit, task queued", "groupJid", groupJid, "taskID", taskID, "activeCount", q.activeCount)
		q.mu.Unlock()
		return
	}

	state.active = true
	state.idleWaiting = false
	state.isTaskContainer = true
	state.runningTaskID = taskID
	q.activeCount++

	q.mu.Unlock()
	go func() {
		task := QueuedTask{ID: taskID, GroupJID: groupJid, Fn: fn}
		if err := q.runTask(groupJid, task); err != nil {
			logger.Error("Unhandled error in runTask", "groupJid", groupJid, "taskID", taskID, "err", err)
		}
	}()
}

func (q *GroupQueue) RegisterProcess(groupJid string, containerName string, groupFolder string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	state := q.getGroup(groupJid)
	state.containerName = containerName
	state.groupFolder = groupFolder
}

func (q *GroupQueue) NotifyIdle(groupJid string) {
	q.mu.Lock()
	state := q.getGroup(groupJid)
	state.idleWaiting = true
	hasPendingTasks := len(state.pendingTasks) > 0
	q.mu.Unlock()

	if hasPendingTasks {
		q.CloseStdin(groupJid)
	}
}

func (q *GroupQueue) SendMessage(groupJid string, text string) bool {
	q.mu.Lock()
	state := q.getGroup(groupJid)
	if !state.active || state.groupFolder == "" || state.isTaskContainer {
		q.mu.Unlock()
		return false
	}
	state.idleWaiting = false
	groupFolder := state.groupFolder
	q.mu.Unlock()

	inputDir := filepath.Join(config.DataDir, "ipc", groupFolder, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		return false
	}

	filename := fmt.Sprintf("%d-%s.json", time.Now().UnixNano(), "rand") // simplified rand
	filepath := filepath.Join(inputDir, filename)
	tempPath := filepath + ".tmp"

	data, _ := json.Marshal(map[string]string{"type": "message", "text": text})
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return false
	}
	if err := os.Rename(tempPath, filepath); err != nil {
		return false
	}
	return true
}

func (q *GroupQueue) CloseStdin(groupJid string) {
	q.mu.Lock()
	state := q.getGroup(groupJid)
	if !state.active || state.groupFolder == "" {
		q.mu.Unlock()
		return
	}
	groupFolder := state.groupFolder
	q.mu.Unlock()

	inputDir := filepath.Join(config.DataDir, "ipc", groupFolder, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(inputDir, "_close"), []byte(""), 0644)
}

func (q *GroupQueue) runForGroup(groupJid string, reason string) error {
	q.mu.Lock()
	state := q.getGroup(groupJid)
	processMessagesFn := q.processMessagesFn
	logger.Debug("Starting container for group", "groupJid", groupJid, "reason", reason, "activeCount", q.activeCount)
	q.mu.Unlock()

	var success bool
	var err error
	if processMessagesFn != nil {
		success, err = processMessagesFn(groupJid)
	} else {
		success = true
	}

	q.mu.Lock()
	if err != nil {
		logger.Error("Error processing messages for group", "groupJid", groupJid, "err", err)
		q.scheduleRetry(groupJid, state)
	} else if !success {
		q.scheduleRetry(groupJid, state)
	} else {
		state.retryCount = 0
	}

	state.active = false
	state.containerName = ""
	state.groupFolder = ""
	q.activeCount--
	q.mu.Unlock()

	q.drainGroup(groupJid)
	return nil
}

func (q *GroupQueue) runTask(groupJid string, task QueuedTask) error {
	q.mu.Lock()
	state := q.getGroup(groupJid)
	logger.Debug("Running queued task", "groupJid", groupJid, "taskID", task.ID, "activeCount", q.activeCount)
	q.mu.Unlock()

	err := task.Fn()

	q.mu.Lock()
	if err != nil {
		logger.Error("Error running task", "groupJid", groupJid, "taskID", task.ID, "err", err)
	}
	state.active = false
	state.isTaskContainer = false
	state.runningTaskID = ""
	state.containerName = ""
	state.groupFolder = ""
	q.activeCount--
	q.mu.Unlock()

	q.drainGroup(groupJid)
	return nil
}

func (q *GroupQueue) scheduleRetry(groupJid string, state *groupState) {
	state.retryCount++
	if state.retryCount > MaxRetries {
		logger.Error("Max retries exceeded, dropping messages", "groupJid", groupJid, "retryCount", state.retryCount)
		state.retryCount = 0
		return
	}

	delayMs := float64(BaseRetryMs) * math.Pow(2, float64(state.retryCount-1))
	delay := time.Duration(delayMs) * time.Millisecond
	logger.Info("Scheduling retry with backoff", "groupJid", groupJid, "retryCount", state.retryCount, "delay", delay)

	time.AfterFunc(delay, func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if !q.shuttingDown {
			q.mu.Unlock()
			q.EnqueueMessageCheck(groupJid)
			q.mu.Lock()
		}
	})
}

func (q *GroupQueue) drainGroup(groupJid string) {
	q.mu.Lock()
	if q.shuttingDown {
		q.mu.Unlock()
		return
	}

	state := q.getGroup(groupJid)

	if len(state.pendingTasks) > 0 {
		task := state.pendingTasks[0]
		state.pendingTasks = state.pendingTasks[1:]
		state.active = true
		state.idleWaiting = false
		state.isTaskContainer = true
		state.runningTaskID = task.ID
		q.activeCount++

		q.mu.Unlock()
		go func() {
			if err := q.runTask(groupJid, task); err != nil {
				logger.Error("Unhandled error in runTask (drain)", "groupJid", groupJid, "taskID", task.ID, "err", err)
			}
		}()
		return
	}

	if state.pendingMessages {
		state.active = true
		state.idleWaiting = false
		state.isTaskContainer = false
		state.pendingMessages = false
		q.activeCount++

		q.mu.Unlock()
		go func() {
			if err := q.runForGroup(groupJid, "drain"); err != nil {
				logger.Error("Unhandled error in runForGroup (drain)", "groupJid", groupJid, "err", err)
			}
		}()
		return
	}
	q.mu.Unlock()

	q.drainWaiting()
}

func (q *GroupQueue) drainWaiting() {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.waitingGroups) > 0 && q.activeCount < config.MaxConcurrentContainers {
		nextJid := q.waitingGroups[0]
		q.waitingGroups = q.waitingGroups[1:]
		state := q.getGroup(nextJid)

		if len(state.pendingTasks) > 0 {
			task := state.pendingTasks[0]
			state.pendingTasks = state.pendingTasks[1:]
			state.active = true
			state.idleWaiting = false
			state.isTaskContainer = true
			state.runningTaskID = task.ID
			q.activeCount++

			q.mu.Unlock()
			go func() {
				if err := q.runTask(nextJid, task); err != nil {
					logger.Error("Unhandled error in runTask (waiting)", "groupJid", nextJid, "taskID", task.ID, "err", err)
				}
			}()
			q.mu.Lock()
		} else if state.pendingMessages {
			state.active = true
			state.idleWaiting = false
			state.isTaskContainer = false
			state.pendingMessages = false
			q.activeCount++

			q.mu.Unlock()
			go func() {
				if err := q.runForGroup(nextJid, "drain"); err != nil {
					logger.Error("Unhandled error in runForGroup (waiting)", "groupJid", nextJid, "err", err)
				}
			}()
			q.mu.Lock()
		}
	}
}

func (q *GroupQueue) Shutdown(gracePeriod time.Duration) {
	q.mu.Lock()
	q.shuttingDown = true
	// In Go, we don't have easy access to child process objects unless we store them.
	// TS stores ChildProcess, but we only store containerName.
	// We'll just log that we are shutting down.
	logger.Info("GroupQueue shutting down", "activeCount", q.activeCount)
	q.mu.Unlock()

	// Wait for active count to reach zero or grace period
	start := time.Now()
	for {
		q.mu.Lock()
		count := q.activeCount
		q.mu.Unlock()
		if count == 0 || time.Since(start) > gracePeriod {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
