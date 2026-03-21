package taskqueue

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/config"
)

func TestGroupQueue_Concurrency(t *testing.T) {
	config.MaxConcurrentContainers = 2
	q := NewGroupQueue()

	var mu sync.Mutex
	active := 0
	maxActive := 0
	completed := 0

	processFn := func(groupJid string) (bool, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		active--
		completed++
		mu.Unlock()
		return true, nil
	}

	q.SetProcessMessagesFn(processFn)

	q.EnqueueMessageCheck("group1")
	q.EnqueueMessageCheck("group2")
	q.EnqueueMessageCheck("group3")

	// Wait for completion
	start := time.Now()
	for {
		mu.Lock()
		if completed == 3 || time.Since(start) > 2*time.Second {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	if completed != 3 {
		t.Errorf("Expected 3 completed, got %d", completed)
	}
	if maxActive > 2 {
		t.Errorf("Expected maxActive <= 2, got %d", maxActive)
	}
}

func TestGroupQueue_Retry(t *testing.T) {
	oldRetryMs := BaseRetryMs
	BaseRetryMs = 100
	defer func() { BaseRetryMs = oldRetryMs }()

	config.MaxConcurrentContainers = 1
	q := NewGroupQueue()

	var mu sync.Mutex
	calls := 0

	processFn := func(groupJid string) (bool, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if calls == 1 {
			return false, errors.New("fail first time")
		}
		return true, nil
	}

	q.SetProcessMessagesFn(processFn)
	q.EnqueueMessageCheck("group1")

	// Wait for second call
	start := time.Now()
	for {
		mu.Lock()
		if calls >= 2 || time.Since(start) > 10*time.Second { // Retry takes time
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}

	if calls < 2 {
		t.Errorf("Expected at least 2 calls (retry), got %d", calls)
	}
}

func TestGroupQueue_Tasks(t *testing.T) {
	config.MaxConcurrentContainers = 1
	q := NewGroupQueue()

	var mu sync.Mutex
	completedTasks := make([]string, 0)

	q.EnqueueTask("group1", "task1", func() error {
		mu.Lock()
		completedTasks = append(completedTasks, "task1")
		mu.Unlock()
		return nil
	})

	q.EnqueueTask("group1", "task2", func() error {
		mu.Lock()
		completedTasks = append(completedTasks, "task2")
		mu.Unlock()
		return nil
	})

	// Wait for completion
	start := time.Now()
	for {
		mu.Lock()
		if len(completedTasks) == 2 || time.Since(start) > 2*time.Second {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	if len(completedTasks) != 2 {
		t.Errorf("Expected 2 completed tasks, got %d", len(completedTasks))
	}
}
