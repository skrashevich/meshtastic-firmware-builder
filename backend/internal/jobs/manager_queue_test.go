package jobs

import (
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/config"
)

func TestQueuePositionForQueuedJobs(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	mgr := NewManager(config.Config{
		ConcurrentBuilds: 0,
		JobsRootPath:     filepath.Join(workDir, "jobs"),
		MaxLogLines:      200,
		CleanupInterval:  time.Hour,
	}, log.New(io.Discard, "", 0))
	defer mgr.Close()

	first, err := mgr.CreateJob("https://github.com/example/repo.git", "main", "tbeam", BuildOptions{})
	if err != nil {
		t.Fatalf("create first job: %v", err)
	}
	second, err := mgr.CreateJob("https://github.com/example/repo.git", "main", "tbeam", BuildOptions{})
	if err != nil {
		t.Fatalf("create second job: %v", err)
	}

	if first.Status != StatusQueued {
		t.Fatalf("first job status: got=%s want=%s", first.Status, StatusQueued)
	}
	if second.Status != StatusQueued {
		t.Fatalf("second job status: got=%s want=%s", second.Status, StatusQueued)
	}

	if first.QueuePosition == nil || *first.QueuePosition != 1 {
		t.Fatalf("first job queue position: got=%v want=1", first.QueuePosition)
	}
	if second.QueuePosition == nil || *second.QueuePosition != 2 {
		t.Fatalf("second job queue position: got=%v want=2", second.QueuePosition)
	}

	secondState, err := mgr.GetJob(second.ID)
	if err != nil {
		t.Fatalf("get second job: %v", err)
	}
	if secondState.QueuePosition == nil || *secondState.QueuePosition != 2 {
		t.Fatalf("second job queue position from GetJob: got=%v want=2", secondState.QueuePosition)
	}
}

func TestQueueETAForQueuedJobs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
	mgr := &Manager{
		cfg: config.Config{
			ConcurrentBuilds: 2,
			BuildTimeout:     20 * time.Minute,
		},
		jobs:       make(map[string]*Job),
		queueOrder: []string{"q1", "q2"},
	}

	runningA := newJob("run-a", "https://github.com/example/repo.git", "main", "tbeam", BuildOptions{}, "/tmp/run-a", now)
	runningA.markRunning(now.Add(-5 * time.Minute))

	runningB := newJob("run-b", "https://github.com/example/repo.git", "main", "tbeam", BuildOptions{}, "/tmp/run-b", now)
	runningB.markRunning(now.Add(-4 * time.Minute))

	completed := newJob("done", "https://github.com/example/repo.git", "main", "tbeam", BuildOptions{}, "/tmp/done", now)
	completed.markRunning(now.Add(-15 * time.Minute))
	completed.markSuccess(now.Add(-11*time.Minute), nil)

	queuedFirst := newJob("q1", "https://github.com/example/repo.git", "main", "tbeam", BuildOptions{}, "/tmp/q1", now)
	queuedSecond := newJob("q2", "https://github.com/example/repo.git", "main", "tbeam", BuildOptions{}, "/tmp/q2", now)

	mgr.jobs[runningA.ID] = runningA
	mgr.jobs[runningB.ID] = runningB
	mgr.jobs[completed.ID] = completed
	mgr.jobs[queuedFirst.ID] = queuedFirst
	mgr.jobs[queuedSecond.ID] = queuedSecond

	state := queuedSecond.snapshot()
	mgr.attachQueueMetadata(queuedSecond.ID, &state)

	if state.QueuePosition == nil || *state.QueuePosition != 2 {
		t.Fatalf("queue position: got=%v want=2", state.QueuePosition)
	}
	if state.QueueETASeconds == nil {
		t.Fatalf("queue ETA should be set")
	}
	if *state.QueueETASeconds != 240 {
		t.Fatalf("queue ETA seconds: got=%d want=240", *state.QueueETASeconds)
	}
}
