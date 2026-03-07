package buildlogs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type BuildLog struct {
	JobID      string     `json:"jobId"`
	RepoURL    string     `json:"repoUrl"`
	Ref        string     `json:"ref,omitempty"`
	Device     string     `json:"device"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
	Lines      []string   `json:"lines"`
}

type BuildLogEntry struct {
	JobID      string     `json:"jobId"`
	RepoURL    string     `json:"repoUrl"`
	Ref        string     `json:"ref,omitempty"`
	Device     string     `json:"device"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Error      string     `json:"error,omitempty"`
	LineCount  int        `json:"lineCount"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

func (s *Store) Save(log BuildLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create build-logs dir: %w", err)
	}

	data, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("marshal build log: %w", err)
	}

	path := filepath.Join(s.dir, log.JobID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write build log: %w", err)
	}
	return nil
}

func (s *Store) List(limit int) ([]BuildLogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BuildLogEntry{}, nil
		}
		return nil, fmt.Errorf("read build-logs dir: %w", err)
	}

	type fileEntry struct {
		name    string
		modTime time.Time
	}
	files := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{name: e.Name(), modTime: info.ModTime()})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}

	result := make([]BuildLogEntry, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(s.dir, f.name))
		if err != nil {
			continue
		}
		var bl BuildLog
		if err := json.Unmarshal(data, &bl); err != nil {
			continue
		}
		result = append(result, BuildLogEntry{
			JobID:      bl.JobID,
			RepoURL:    bl.RepoURL,
			Ref:        bl.Ref,
			Device:     bl.Device,
			Status:     bl.Status,
			CreatedAt:  bl.CreatedAt,
			StartedAt:  bl.StartedAt,
			FinishedAt: bl.FinishedAt,
			Error:      bl.Error,
			LineCount:  len(bl.Lines),
		})
	}
	return result, nil
}

func (s *Store) Get(jobID string) (*BuildLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	clean := filepath.Base(jobID)
	if clean != jobID || clean == "." || clean == ".." {
		return nil, fmt.Errorf("invalid job ID")
	}

	path := filepath.Join(s.dir, clean+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read build log: %w", err)
	}

	var bl BuildLog
	if err := json.Unmarshal(data, &bl); err != nil {
		return nil, fmt.Errorf("unmarshal build log: %w", err)
	}
	return &bl, nil
}
