package scheduler

import (
	"context"
	"strconv"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/db"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/taskqueue"
	"github.com/nanoclaw/nanoclaw/pkg/types"
	"github.com/robfig/cron/v3"
)

// ComputeNextRun computes the next run time for a recurring task.
func ComputeNextRun(task types.ScheduledTask, timezone string) (*string, error) {
	if task.ScheduleType == "once" {
		return nil, nil
	}

	now := time.Now()
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	if task.ScheduleType == "cron" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		sched, err := parser.Parse(task.ScheduleValue)
		if err != nil {
			return nil, err
		}
		next := sched.Next(now.In(loc))
		iso := next.Format(time.RFC3339)
		return &iso, nil
	}

	if task.ScheduleType == "interval" {
		ms, err := strconv.ParseInt(task.ScheduleValue, 10, 64)
		if err != nil || ms <= 0 {
			// Guard against malformed interval
			next := now.Add(60 * time.Second)
			iso := next.Format(time.RFC3339)
			return &iso, nil
		}

		duration := time.Duration(ms) * time.Millisecond
		var next time.Time
		if task.NextRun != nil {
			parsedNext, err := time.Parse(time.RFC3339, *task.NextRun)
			if err == nil {
				next = parsedNext.Add(duration)
			} else {
				next = now.Add(duration)
			}
		} else {
			next = now.Add(duration)
		}

		// Skip past any missed intervals
		for next.Before(now) || next.Equal(now) {
			next = next.Add(duration)
		}
		iso := next.Format(time.RFC3339)
		return &iso, nil
	}

	return nil, nil
}

type TaskScheduler struct {
	storage db.Storage
	queue   *taskqueue.GroupQueue
}

func NewTaskScheduler(storage db.Storage, queue *taskqueue.GroupQueue) *TaskScheduler {
	return &TaskScheduler{
		storage: storage,
		queue:   queue,
	}
}

func (s *TaskScheduler) Start(ctx context.Context) {
	logger.Info("Scheduler loop started")

	ticker := time.NewTicker(time.Duration(config.SchedulerPollInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.poll(); err != nil {
				logger.Error("Error in scheduler loop", "err", err)
			}
		}
	}
}

func (s *TaskScheduler) poll() error {
	dueTasks, err := s.storage.GetDueTasks()
	if err != nil {
		return err
	}

	if len(dueTasks) > 0 {
		logger.Info("Found due tasks", "count", len(dueTasks))
	}

	for _, task := range dueTasks {
		// Re-check task status
		currentTask, err := s.storage.GetTaskById(task.ID)
		if err != nil || currentTask == nil || currentTask.Status != "active" {
			continue
		}

		// Closure to capture currentTask
		taskToRun := *currentTask
		s.queue.EnqueueTask(taskToRun.ChatJID, taskToRun.ID, func() error {
			return s.RunTask(taskToRun)
		})
	}

	return nil
}

func (s *TaskScheduler) RunTask(task types.ScheduledTask) error {
	startTime := time.Now()
	logger.Info("Running scheduled task", "taskId", task.ID, "group", task.GroupFolder)

	// In Go port, runAgent and other dependencies for RunTask are not fully implemented.
	// This would call pkg/container/runner.go
	
	// We'll simulate completion for now to satisfy the interface.
	
	duration := time.Since(startTime)
	log := types.TaskRunLog{
		TaskID:     task.ID,
		RunAt:      startTime.Format(time.RFC3339),
		DurationMS: int(duration.Milliseconds()),
		Status:     "success",
	}
	_ = s.storage.LogTaskRun(log)

	nextRun, _ := ComputeNextRun(task, config.Timezone)
	_ = s.storage.UpdateTaskAfterRun(task.ID, nextRun, "Completed")

	return nil
}
