package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/config"
	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/jobs"
)

type Server struct {
	cfg            config.Config
	manager        *jobs.Manager
	logger         *log.Logger
	allowedOrigins map[string]struct{}
	rateMu         sync.Mutex
	buildRequests  map[string][]time.Time
}

func NewServer(cfg config.Config, manager *jobs.Manager, logger *log.Logger) *Server {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowed[origin] = struct{}{}
	}

	return &Server{
		cfg:            cfg,
		manager:        manager,
		logger:         logger,
		allowedOrigins: allowed,
		buildRequests:  make(map[string][]time.Time),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()
	w.Header().Set("X-Request-ID", requestID)

	if !s.handleCORS(w, r, requestID) {
		return
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/api/healthz" {
		s.writeSuccess(w, http.StatusOK, requestID, map[string]string{"status": "ok"})
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/api/repos/discover" {
		s.handleDiscover(w, r, requestID)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/api/jobs" {
		s.handleCreateJob(w, r, requestID)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/api/jobs/") {
		s.handleJobRoutes(w, r, requestID)
		return
	}

	s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "route not found", nil)
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request, requestID string) {
	var req discoverRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	devices, err := s.manager.Discover(r.Context(), req.RepoURL, req.Ref)
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, requestID, "DISCOVERY_FAILED", err.Error(), nil)
		return
	}

	data := discoverResponse{
		RepoURL: req.RepoURL,
		Ref:     req.Ref,
		Devices: devices,
	}
	s.writeSuccess(w, http.StatusOK, requestID, data)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request, requestID string) {
	var req createJobRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	if !s.allowBuildRequest(r.RemoteAddr) {
		s.writeError(w, http.StatusTooManyRequests, requestID, "RATE_LIMITED", "too many build requests from this client", nil)
		return
	}

	state, err := s.manager.CreateJob(req.RepoURL, req.Ref, req.Device)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_JOB", err.Error(), nil)
		return
	}

	s.writeSuccess(w, http.StatusCreated, requestID, s.presentState(state))
}

func (s *Server) handleJobRoutes(w http.ResponseWriter, r *http.Request, requestID string) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "route not found", nil)
		return
	}

	parts := strings.Split(trimmed, "/")
	jobID := parts[0]

	if len(parts) == 1 && r.Method == http.MethodGet {
		s.handleGetJob(w, requestID, jobID)
		return
	}

	if len(parts) == 2 && parts[1] == "logs" && r.Method == http.MethodGet {
		s.handleGetLogs(w, requestID, jobID)
		return
	}

	if len(parts) == 3 && parts[1] == "logs" && parts[2] == "stream" && r.Method == http.MethodGet {
		s.handleLogStream(w, r, requestID, jobID)
		return
	}

	if len(parts) == 2 && parts[1] == "artifacts" && r.Method == http.MethodGet {
		s.handleGetArtifacts(w, requestID, jobID)
		return
	}

	if len(parts) == 3 && parts[1] == "artifacts" && r.Method == http.MethodGet {
		s.handleDownloadArtifact(w, r, requestID, jobID, parts[2])
		return
	}

	s.writeError(w, http.StatusNotFound, requestID, "NOT_FOUND", "route not found", nil)
}

func (s *Server) handleGetJob(w http.ResponseWriter, requestID string, jobID string) {
	state, err := s.manager.GetJob(jobID)
	if err != nil {
		s.handleJobError(w, requestID, err)
		return
	}

	s.writeSuccess(w, http.StatusOK, requestID, s.presentState(state))
}

func (s *Server) handleGetLogs(w http.ResponseWriter, requestID string, jobID string) {
	logs, err := s.manager.GetLogs(jobID)
	if err != nil {
		s.handleJobError(w, requestID, err)
		return
	}

	s.writeSuccess(w, http.StatusOK, requestID, logsResponse{Lines: logs})
}

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request, requestID string, jobID string) {
	stream, snapshot, unsubscribe, err := s.manager.SubscribeLogs(jobID)
	if err != nil {
		s.handleJobError(w, requestID, err)
		return
	}
	defer unsubscribe()

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, requestID, "STREAM_UNSUPPORTED", "streaming is not supported", nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for _, line := range snapshot {
		writeSSE(w, "log", line)
	}
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			writeSSE(w, "ping", time.Now().UTC().Format(time.RFC3339))
			flusher.Flush()
		case line, open := <-stream:
			if !open {
				return
			}
			writeSSE(w, "log", line)
			flusher.Flush()
		}
	}
}

func (s *Server) handleGetArtifacts(w http.ResponseWriter, requestID string, jobID string) {
	state, err := s.manager.GetJob(jobID)
	if err != nil {
		s.handleJobError(w, requestID, err)
		return
	}

	s.writeSuccess(w, http.StatusOK, requestID, artifactsResponse{Artifacts: toArtifactViews(jobID, state.Artifacts)})
}

func (s *Server) handleDownloadArtifact(w http.ResponseWriter, r *http.Request, requestID string, jobID string, artifactID string) {
	artifact, err := s.manager.GetArtifact(jobID, artifactID)
	if err != nil {
		s.handleJobError(w, requestID, err)
		return
	}

	fileName := filepath.Base(artifact.Name)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	http.ServeFile(w, r, artifact.AbsolutePath())
}

func (s *Server) handleJobError(w http.ResponseWriter, requestID string, err error) {
	if errors.Is(err, jobs.ErrJobNotFound) {
		s.writeError(w, http.StatusNotFound, requestID, "JOB_NOT_FOUND", err.Error(), nil)
		return
	}
	if errors.Is(err, jobs.ErrArtifactNotFound) {
		s.writeError(w, http.StatusNotFound, requestID, "ARTIFACT_NOT_FOUND", err.Error(), nil)
		return
	}
	s.writeError(w, http.StatusInternalServerError, requestID, "INTERNAL_ERROR", err.Error(), nil)
}

func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request, requestID string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	if _, ok := s.allowedOrigins[origin]; !ok {
		s.writeError(w, http.StatusForbidden, requestID, "ORIGIN_NOT_ALLOWED", "origin is not allowed", nil)
		return false
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "600")
	return true
}

func (s *Server) writeSuccess(w http.ResponseWriter, status int, requestID string, data any) {
	envelope := map[string]any{
		"data": data,
		"meta": map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"requestId": requestID,
		},
	}
	s.writeJSON(w, status, envelope)
}

func (s *Server) writeError(w http.ResponseWriter, status int, requestID string, code string, message string, details any) {
	envelope := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": details,
		},
		"meta": map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"requestId": requestID,
		},
	}
	s.writeJSON(w, status, envelope)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Printf("write response: %v", err)
	}
}

func (s *Server) presentState(state jobs.State) stateResponse {
	return stateResponse{
		ID:         state.ID,
		RepoURL:    state.RepoURL,
		Ref:        state.Ref,
		Device:     state.Device,
		Status:     state.Status,
		CreatedAt:  state.CreatedAt,
		StartedAt:  state.StartedAt,
		FinishedAt: state.FinishedAt,
		Error:      state.Error,
		LogLines:   state.LogLines,
		Artifacts:  toArtifactViews(state.ID, state.Artifacts),
	}
}

func toArtifactViews(jobID string, artifacts []jobs.Artifact) []artifactView {
	views := make([]artifactView, len(artifacts))
	for index, artifact := range artifacts {
		views[index] = artifactView{
			ID:           artifact.ID,
			Name:         artifact.Name,
			RelativePath: artifact.RelativePath,
			Size:         artifact.Size,
			DownloadURL:  fmt.Sprintf("/api/jobs/%s/artifacts/%s", jobID, artifact.ID),
		}
	}
	return views
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func writeSSE(w http.ResponseWriter, event string, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	for _, line := range strings.Split(data, "\n") {
		_, _ = fmt.Fprintf(w, "data: %s\n", line)
	}
	_, _ = fmt.Fprint(w, "\n")
}

func generateRequestID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func (s *Server) allowBuildRequest(remoteAddr string) bool {
	host := remoteAddr
	if parsedHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = parsedHost
	}

	now := time.Now().UTC()
	threshold := now.Add(-1 * time.Minute)

	s.rateMu.Lock()
	defer s.rateMu.Unlock()

	items := s.buildRequests[host]
	filtered := items[:0]
	for _, stamp := range items {
		if stamp.After(threshold) {
			filtered = append(filtered, stamp)
		}
	}

	if len(filtered) >= s.cfg.BuildRateLimit {
		s.buildRequests[host] = append([]time.Time(nil), filtered...)
		return false
	}

	filtered = append(filtered, now)
	s.buildRequests[host] = append([]time.Time(nil), filtered...)
	return true
}

type discoverRequest struct {
	RepoURL string `json:"repoUrl"`
	Ref     string `json:"ref"`
}

type discoverResponse struct {
	RepoURL string   `json:"repoUrl"`
	Ref     string   `json:"ref,omitempty"`
	Devices []string `json:"devices"`
}

type createJobRequest struct {
	RepoURL string `json:"repoUrl"`
	Ref     string `json:"ref"`
	Device  string `json:"device"`
}

type logsResponse struct {
	Lines []string `json:"lines"`
}

type stateResponse struct {
	ID         string         `json:"id"`
	RepoURL    string         `json:"repoUrl"`
	Ref        string         `json:"ref,omitempty"`
	Device     string         `json:"device"`
	Status     jobs.Status    `json:"status"`
	CreatedAt  time.Time      `json:"createdAt"`
	StartedAt  *time.Time     `json:"startedAt,omitempty"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
	Error      string         `json:"error,omitempty"`
	LogLines   int            `json:"logLines"`
	Artifacts  []artifactView `json:"artifacts"`
}

type artifactsResponse struct {
	Artifacts []artifactView `json:"artifacts"`
}

type artifactView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
	Size         int64  `json:"size"`
	DownloadURL  string `json:"downloadUrl"`
}
