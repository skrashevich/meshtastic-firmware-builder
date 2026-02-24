package jobs

import (
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSuccess   Status = "success"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Artifact struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
	Size         int64  `json:"size"`

	absPath string
}

func (a Artifact) AbsolutePath() string {
	return a.absPath
}

type State struct {
	ID         string      `json:"id"`
	RepoURL    string      `json:"repoUrl"`
	Ref        string      `json:"ref,omitempty"`
	Device     string      `json:"device"`
	Status     Status      `json:"status"`
	CreatedAt  time.Time   `json:"createdAt"`
	StartedAt  *time.Time  `json:"startedAt,omitempty"`
	FinishedAt *time.Time  `json:"finishedAt,omitempty"`
	Error      string      `json:"error,omitempty"`
	Artifacts  []Artifact  `json:"artifacts"`
	LogLines   int         `json:"logLines"`
	Logs       []string    `json:"-"`
	Internal   interface{} `json:"-"`
}

type Job struct {
	mu          sync.RWMutex
	ID          string
	RepoURL     string
	Ref         string
	Device      string
	Status      Status
	CreatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	Error       string
	Artifacts   []Artifact
	Workspace   string
	logLines    []string
	subscribers map[chan string]struct{}
}

func newJob(id string, repoURL string, ref string, device string, workspace string, now time.Time) *Job {
	return &Job{
		ID:          id,
		RepoURL:     repoURL,
		Ref:         ref,
		Device:      device,
		Status:      StatusQueued,
		CreatedAt:   now,
		Workspace:   workspace,
		logLines:    make([]string, 0, 256),
		Artifacts:   make([]Artifact, 0),
		subscribers: make(map[chan string]struct{}),
	}
}

func (j *Job) snapshot() State {
	j.mu.RLock()
	defer j.mu.RUnlock()

	artifacts := make([]Artifact, len(j.Artifacts))
	copy(artifacts, j.Artifacts)

	return State{
		ID:         j.ID,
		RepoURL:    j.RepoURL,
		Ref:        j.Ref,
		Device:     j.Device,
		Status:     j.Status,
		CreatedAt:  j.CreatedAt,
		StartedAt:  copyTime(j.StartedAt),
		FinishedAt: copyTime(j.FinishedAt),
		Error:      j.Error,
		Artifacts:  artifacts,
		LogLines:   len(j.logLines),
	}
}

func (j *Job) getLogs() []string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	logs := make([]string, len(j.logLines))
	copy(logs, j.logLines)
	return logs
}

func (j *Job) subscribe() (<-chan string, []string, func()) {
	j.mu.Lock()
	defer j.mu.Unlock()

	snapshot := make([]string, len(j.logLines))
	copy(snapshot, j.logLines)

	stream := make(chan string, 256)
	if !isFinal(j.Status) {
		j.subscribers[stream] = struct{}{}
	} else {
		close(stream)
	}

	unsubscribe := func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		if _, ok := j.subscribers[stream]; !ok {
			return
		}
		delete(j.subscribers, stream)
		close(stream)
	}

	return stream, snapshot, unsubscribe
}

func (j *Job) appendLog(maxLines int, line string) {
	clean := strings.TrimRight(line, "\r\n")
	if clean == "" {
		return
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	j.logLines = append(j.logLines, clean)
	if len(j.logLines) > maxLines {
		j.logLines = append([]string(nil), j.logLines[len(j.logLines)-maxLines:]...)
	}

	for stream := range j.subscribers {
		select {
		case stream <- clean:
		default:
		}
	}
}

func (j *Job) markRunning(now time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	started := now
	j.StartedAt = &started
	j.Status = StatusRunning
}

func (j *Job) markFailed(now time.Time, reason string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	finished := now
	j.Status = StatusFailed
	j.FinishedAt = &finished
	j.Error = reason
	j.closeSubscribersLocked()
}

func (j *Job) markCancelled(now time.Time, reason string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	finished := now
	j.Status = StatusCancelled
	j.FinishedAt = &finished
	j.Error = reason
	j.closeSubscribersLocked()
}

func (j *Job) markSuccess(now time.Time, artifacts []Artifact) {
	j.mu.Lock()
	defer j.mu.Unlock()
	finished := now
	j.Status = StatusSuccess
	j.FinishedAt = &finished
	j.Artifacts = make([]Artifact, len(artifacts))
	copy(j.Artifacts, artifacts)
	j.closeSubscribersLocked()
}

func (j *Job) isExpired(now time.Time, ttl time.Duration) bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.FinishedAt == nil {
		return false
	}
	return now.Sub(*j.FinishedAt) >= ttl
}

func (j *Job) artifactByID(artifactID string) (Artifact, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	for _, artifact := range j.Artifacts {
		if artifact.ID == artifactID {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func (j *Job) closeSubscribersLocked() {
	for stream := range j.subscribers {
		close(stream)
	}
	clear(j.subscribers)
}

func isFinal(status Status) bool {
	return status == StatusSuccess || status == StatusFailed || status == StatusCancelled
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
