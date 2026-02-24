package jobs

import (
	"testing"
	"time"
)

func TestJobStateTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	job := newJob("abc", "https://example.com/repo.git", "main", "tbeam", "/tmp/workspace", now)

	if job.snapshot().Status != StatusQueued {
		t.Fatalf("expected queued status")
	}

	job.markRunning(now.Add(1 * time.Minute))
	if job.snapshot().Status != StatusRunning {
		t.Fatalf("expected running status")
	}

	artifacts := []Artifact{{ID: "1", Name: "firmware.bin", RelativePath: "firmware.bin", Size: 100}}
	job.markSuccess(now.Add(2*time.Minute), artifacts)

	state := job.snapshot()
	if state.Status != StatusSuccess {
		t.Fatalf("expected success status")
	}
	if state.FinishedAt == nil {
		t.Fatalf("expected finishedAt to be set")
	}
	if len(state.Artifacts) != 1 {
		t.Fatalf("expected one artifact")
	}
}
