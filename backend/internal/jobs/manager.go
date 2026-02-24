package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"meshtastic-firmware-builder/backend/internal/config"
)

var (
	ErrJobNotFound      = errors.New("job not found")
	ErrArtifactNotFound = errors.New("artifact not found")
)

type Manager struct {
	cfg    config.Config
	logger *log.Logger

	mu   sync.RWMutex
	jobs map[string]*Job

	queue  chan *Job
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	now    func() time.Time
}

func NewManager(cfg config.Config, logger *log.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	mgr := &Manager{
		cfg:    cfg,
		logger: logger,
		jobs:   make(map[string]*Job),
		queue:  make(chan *Job, 128),
		ctx:    ctx,
		cancel: cancel,
		now:    func() time.Time { return time.Now().UTC() },
	}

	for index := 0; index < cfg.ConcurrentBuilds; index++ {
		mgr.wg.Add(1)
		go mgr.workerLoop(index + 1)
	}

	mgr.wg.Add(1)
	go mgr.cleanupLoop()

	return mgr
}

func (m *Manager) Close() {
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) Discover(ctx context.Context, repoURL string, ref string) ([]string, error) {
	if err := ValidateRepoURL(repoURL); err != nil {
		return nil, err
	}
	if err := ValidateRef(ref); err != nil {
		return nil, err
	}

	devices, err := discoverDevices(ctx, m.cfg.DiscoveryRootPath, repoURL, ref)
	if err != nil {
		return nil, err
	}
	return devices, nil
}

func (m *Manager) CreateJob(repoURL string, ref string, device string) (State, error) {
	if err := ValidateRepoURL(repoURL); err != nil {
		return State{}, err
	}
	if err := ValidateRef(ref); err != nil {
		return State{}, err
	}
	if err := ValidateDevice(device); err != nil {
		return State{}, err
	}

	jobID, err := generateJobID()
	if err != nil {
		return State{}, err
	}

	workspace := filepath.Join(m.cfg.JobsRootPath, jobID)
	job := newJob(jobID, repoURL, ref, device, workspace, m.now())

	m.mu.Lock()
	m.jobs[jobID] = job
	m.mu.Unlock()

	select {
	case m.queue <- job:
	case <-m.ctx.Done():
		return State{}, errors.New("service is shutting down")
	}

	return job.snapshot(), nil
}

func (m *Manager) GetJob(jobID string) (State, error) {
	job, err := m.getJob(jobID)
	if err != nil {
		return State{}, err
	}
	return job.snapshot(), nil
}

func (m *Manager) GetLogs(jobID string) ([]string, error) {
	job, err := m.getJob(jobID)
	if err != nil {
		return nil, err
	}
	return job.getLogs(), nil
}

func (m *Manager) SubscribeLogs(jobID string) (<-chan string, []string, func(), error) {
	job, err := m.getJob(jobID)
	if err != nil {
		return nil, nil, nil, err
	}
	stream, snapshot, unsubscribe := job.subscribe()
	return stream, snapshot, unsubscribe, nil
}

func (m *Manager) GetArtifact(jobID string, artifactID string) (Artifact, error) {
	job, err := m.getJob(jobID)
	if err != nil {
		return Artifact{}, err
	}

	artifact, ok := job.artifactByID(artifactID)
	if !ok {
		return Artifact{}, ErrArtifactNotFound
	}
	return artifact, nil
}

func (m *Manager) workerLoop(workerID int) {
	defer m.wg.Done()

	m.logger.Printf("worker-%d started", workerID)
	for {
		select {
		case <-m.ctx.Done():
			m.logger.Printf("worker-%d stopped", workerID)
			return
		case job := <-m.queue:
			if job == nil {
				continue
			}
			m.executeJob(job)
		}
	}
}

func (m *Manager) executeJob(job *Job) {
	job.markRunning(m.now())
	job.appendLog(m.cfg.MaxLogLines, fmt.Sprintf("build started for device %s", job.Device))

	if err := os.MkdirAll(job.Workspace, 0o755); err != nil {
		m.failJob(job, fmt.Errorf("create workspace: %w", err))
		return
	}

	repoPath := filepath.Join(job.Workspace, "repo")
	ctx, cancel := context.WithTimeout(m.ctx, m.cfg.BuildTimeout)
	defer cancel()

	onLog := func(line string) {
		job.appendLog(m.cfg.MaxLogLines, line)
	}

	if err := cloneRepository(ctx, job.RepoURL, job.Ref, repoPath, onLog); err != nil {
		m.failJob(job, err)
		return
	}

	exists, err := variantExists(repoPath, job.Device)
	if err != nil {
		m.failJob(job, err)
		return
	}
	if !exists {
		m.failJob(job, fmt.Errorf("device %q was not found in variants directory", job.Device))
		return
	}

	projectPath, err := findVariantProjectPath(repoPath, job.Device)
	if err != nil {
		m.failJob(job, err)
		return
	}
	if projectPath == "" {
		m.failJob(job, fmt.Errorf("device %q has no platformio.ini in variants", job.Device))
		return
	}

	if err := runBuildInContainer(ctx, m.cfg, repoPath, projectPath, job.Device, onLog); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			m.failJob(job, fmt.Errorf("build timeout reached after %s", m.cfg.BuildTimeout))
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			job.markCancelled(m.now(), "build cancelled")
			return
		}
		m.failJob(job, err)
		return
	}

	artifacts, err := collectArtifacts(projectPath, job.Device)
	if err != nil {
		m.failJob(job, err)
		return
	}

	job.appendLog(m.cfg.MaxLogLines, fmt.Sprintf("build completed, artifacts: %d", len(artifacts)))
	job.markSuccess(m.now(), artifacts)
}

func (m *Manager) failJob(job *Job, err error) {
	job.appendLog(m.cfg.MaxLogLines, "ERROR: "+err.Error())
	job.markFailed(m.now(), err.Error())
}

func (m *Manager) cleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupExpiredJobs()
		}
	}
}

func (m *Manager) cleanupExpiredJobs() {
	now := m.now()
	removePaths := make([]string, 0)
	removed := 0

	m.mu.Lock()
	for jobID, job := range m.jobs {
		if !job.isExpired(now, m.cfg.Retention) {
			continue
		}
		delete(m.jobs, jobID)
		removePaths = append(removePaths, job.Workspace)
		removed++
	}
	m.mu.Unlock()

	for _, path := range removePaths {
		if err := os.RemoveAll(path); err != nil {
			m.logger.Printf("cleanup workspace %s: %v", path, err)
		}
	}

	if removed > 0 {
		m.logger.Printf("cleanup removed %d expired jobs", removed)
	}
}

func (m *Manager) getJob(jobID string) (*Job, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return nil, ErrJobNotFound
	}
	return job, nil
}

func generateJobID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate job id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
