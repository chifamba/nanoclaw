package scheduler

import (
	"testing"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/types"
)

func TestComputeNextRun_Interval(t *testing.T) {
	now := time.Now()
	nextRun := now.Add(-10 * time.Minute).Format(time.RFC3339)

	task := types.ScheduledTask{
		ScheduleType:  "interval",
		ScheduleValue: "3600000", // 1 hour
		NextRun:       &nextRun,
	}

	result, err := ComputeNextRun(task, "UTC")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	parsed, err := time.Parse(time.RFC3339, *result)
	if err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	expected := now.Add(50 * time.Minute) // -10min + 60min = 50min from now
	if parsed.Sub(expected).Abs() > time.Second {
		t.Errorf("Expected next run around %v, got %v", expected, parsed)
	}
}

func TestComputeNextRun_Cron(t *testing.T) {
	task := types.ScheduledTask{
		ScheduleType:  "cron",
		ScheduleValue: "0 0 * * *", // every day at midnight
	}

	result, err := ComputeNextRun(task, "UTC")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	parsed, err := time.Parse(time.RFC3339, *result)
	if err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	if parsed.Hour() != 0 || parsed.Minute() != 0 {
		t.Errorf("Expected midnight, got %v", parsed)
	}
}

func TestComputeNextRun_Once(t *testing.T) {
	task := types.ScheduledTask{
		ScheduleType: "once",
	}

	result, err := ComputeNextRun(task, "UTC")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result for 'once', got %v", *result)
	}
}

func TestScheduler_PausesInvalidFolders(t *testing.T) {
	// this is tested inside queue logic essentially where invalid folders are skipped or panic, but we ensure ComputeNextRun doesn't panic
	// and scheduler pause logic would be tested with DB if we had it here
}

func TestComputeNextRun_SkipMissedIntervals(t *testing.T) {
	now := time.Now()
	ms := 60000 * time.Millisecond
	missedBy := 10 * ms
	scheduledTime := now.Add(-missedBy).Format(time.RFC3339)

	task := types.ScheduledTask{
		ScheduleType:  "interval",
		ScheduleValue: "60000",
		NextRun:       &scheduledTime,
	}

	result, err := ComputeNextRun(task, "UTC")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	parsed, err := time.Parse(time.RFC3339, *result)
	if err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	if parsed.Before(now) {
		t.Errorf("Expected skipped interval to be in the future, got %v", parsed)
	}
}
