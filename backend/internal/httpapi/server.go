package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/config"
	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/jobs"
)

const (
	targetBackendHeader     = "X-MFB-Target-Backend"
	servedByHeader          = "X-MFB-Served-By"
	proxiedViaHeader        = "X-MFB-Proxied-Via"
	targetBackendQueryParam = "__mfb_target_backend"
)

type Server struct {
	cfg             config.Config
	manager         *jobs.Manager
	logger          *log.Logger
	allowedOrigins  map[string]struct{}
	rateMu          sync.Mutex
	buildRequests   map[string][]time.Time
	captchaMu       sync.Mutex
	captchas        map[string]captchaChallenge
	captchaSessions map[string]captchaSession
	nodeBaseURL     string
	proxyBackends   map[string]struct{}
	proxyClient     *http.Client
}

func NewServer(cfg config.Config, manager *jobs.Manager, logger *log.Logger) *Server {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowed[origin] = struct{}{}
	}

	proxyBackends := make(map[string]struct{}, len(cfg.ProxyBackendURLs))
	for _, backendBaseURL := range cfg.ProxyBackendURLs {
		proxyBackends[backendBaseURL] = struct{}{}
	}

	return &Server{
		cfg:             cfg,
		manager:         manager,
		logger:          logger,
		allowedOrigins:  allowed,
		buildRequests:   make(map[string][]time.Time),
		captchas:        make(map[string]captchaChallenge),
		captchaSessions: make(map[string]captchaSession),
		nodeBaseURL:     cfg.NodeBaseURL,
		proxyBackends:   proxyBackends,
		proxyClient: &http.Client{
			Timeout: cfg.ProxyTimeout,
		},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := generateRequestID()
	w.Header().Set("X-Request-ID", requestID)

	localBackendBaseURL := s.localBackendBaseURL(r)
	if localBackendBaseURL != "" {
		w.Header().Set(servedByHeader, localBackendBaseURL)
	}

	if !s.handleCORS(w, r, requestID) {
		return
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if s.tryProxyRequest(w, r, requestID, localBackendBaseURL) {
		return
	}

	s.serveLocalRoutes(w, r, requestID)
}

func (s *Server) serveLocalRoutes(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method == http.MethodGet && r.URL.Path == "/api/healthz" {
		s.writeSuccess(w, http.StatusOK, requestID, s.buildHealthResponse(r))
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/api/repos/discover" {
		s.handleDiscover(w, r, requestID)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/api/repos/refs" {
		s.handleRepoRefs(w, r, requestID)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/api/captcha" {
		s.handleNewCaptcha(w, r, requestID)
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

func (s *Server) buildHealthResponse(r *http.Request) healthResponse {
	nodeBaseURL := s.localBackendBaseURL(r)
	load := s.manager.LoadSnapshot()
	proxyBackendURLs := make([]string, 0, len(s.proxyBackends))
	for backendBaseURL := range s.proxyBackends {
		if backendBaseURL == "" {
			continue
		}
		if nodeBaseURL != "" && backendBaseURL == nodeBaseURL {
			continue
		}
		proxyBackendURLs = append(proxyBackendURLs, backendBaseURL)
	}
	sort.Strings(proxyBackendURLs)

	return healthResponse{
		Status:           "ok",
		CaptchaRequired:  s.cfg.RequireCaptcha,
		NodeBaseURL:      nodeBaseURL,
		ProxyBackendURLs: proxyBackendURLs,
		RunningBuilds:    load.RunningBuilds,
		QueuedBuilds:     load.QueuedBuilds,
		ConcurrentBuilds: load.ConcurrentBuilds,
	}
}

func (s *Server) localBackendBaseURL(r *http.Request) string {
	if s.nodeBaseURL != "" {
		return s.nodeBaseURL
	}

	host := strings.TrimSpace(r.Host)
	if host == "" {
		return ""
	}

	scheme := "http"
	forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if strings.EqualFold(forwardedProto, "https") || r.TLS != nil {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func (s *Server) tryProxyRequest(w http.ResponseWriter, r *http.Request, requestID string, localBackendBaseURL string) bool {
	targetBackendRaw := s.extractTargetBackend(r)
	if targetBackendRaw == "" {
		return false
	}

	targetBackendBaseURL, err := normalizeBackendBaseURL(targetBackendRaw)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "PROXY_TARGET_INVALID", err.Error(), nil)
		return true
	}

	if targetBackendBaseURL == localBackendBaseURL {
		return false
	}
	if s.nodeBaseURL != "" && targetBackendBaseURL == s.nodeBaseURL {
		return false
	}

	if _, allowed := s.proxyBackends[targetBackendBaseURL]; !allowed {
		s.writeError(
			w,
			http.StatusForbidden,
			requestID,
			"PROXY_TARGET_NOT_ALLOWED",
			"proxy target backend is not allowed",
			nil,
		)
		return true
	}

	if err := s.proxyToBackend(w, r, requestID, targetBackendBaseURL, localBackendBaseURL); err != nil {
		s.writeError(
			w,
			http.StatusServiceUnavailable,
			requestID,
			"PROXY_TARGET_UNAVAILABLE",
			"proxy target backend is unavailable",
			nil,
		)
		s.logger.Printf("proxy request failed: target=%s path=%s error=%v", targetBackendBaseURL, r.URL.Path, err)
		return true
	}

	return true
}

func (s *Server) extractTargetBackend(r *http.Request) string {
	if targetBackend := strings.TrimSpace(r.Header.Get(targetBackendHeader)); targetBackend != "" {
		return targetBackend
	}

	return strings.TrimSpace(r.URL.Query().Get(targetBackendQueryParam))
}

func (s *Server) proxyToBackend(
	w http.ResponseWriter,
	r *http.Request,
	requestID string,
	targetBackendBaseURL string,
	localBackendBaseURL string,
) error {
	proxyRequestURL := buildProxyRequestURL(targetBackendBaseURL, r.URL)

	proxyRequest, err := http.NewRequestWithContext(r.Context(), r.Method, proxyRequestURL, r.Body)
	if err != nil {
		return fmt.Errorf("create proxy request: %w", err)
	}

	copyProxyRequestHeaders(proxyRequest.Header, r.Header)
	proxyRequest.Header.Del(targetBackendHeader)
	proxyRequest.Header.Del("Origin")
	proxyRequest.Header.Set("X-Request-ID", requestID)
	if localBackendBaseURL != "" {
		proxyRequest.Header.Set(proxiedViaHeader, localBackendBaseURL)
	}

	proxyResponse, err := s.proxyClient.Do(proxyRequest)
	if err != nil {
		return fmt.Errorf("send proxy request: %w", err)
	}
	defer proxyResponse.Body.Close()

	copyProxyResponseHeaders(w.Header(), proxyResponse.Header)

	resolvedServedBy := strings.TrimSpace(proxyResponse.Header.Get(servedByHeader))
	if resolvedServedBy == "" {
		resolvedServedBy = targetBackendBaseURL
	}
	w.Header().Set(servedByHeader, resolvedServedBy)
	if localBackendBaseURL != "" {
		w.Header().Set(proxiedViaHeader, localBackendBaseURL)
	}

	w.WriteHeader(proxyResponse.StatusCode)
	if strings.HasPrefix(strings.ToLower(proxyResponse.Header.Get("Content-Type")), "text/event-stream") {
		if flusher, ok := w.(http.Flusher); ok {
			buffer := make([]byte, 1024)
			for {
				bytesRead, readErr := proxyResponse.Body.Read(buffer)
				if bytesRead > 0 {
					if _, writeErr := w.Write(buffer[:bytesRead]); writeErr != nil {
						return nil
					}
					flusher.Flush()
				}

				if errors.Is(readErr, io.EOF) {
					return nil
				}
				if readErr != nil {
					return nil
				}
			}
		}
	}

	if _, err := io.Copy(w, proxyResponse.Body); err != nil {
		s.logger.Printf("proxy response copy failed: %v", err)
	}

	return nil
}

func buildProxyRequestURL(targetBackendBaseURL string, sourceURL *url.URL) string {
	queryValues := sourceURL.Query()
	queryValues.Del(targetBackendQueryParam)

	baseURL := strings.TrimSuffix(targetBackendBaseURL, "/") + sourceURL.Path
	encodedQuery := queryValues.Encode()
	if encodedQuery == "" {
		return baseURL
	}

	return baseURL + "?" + encodedQuery
}

func copyProxyRequestHeaders(target http.Header, source http.Header) {
	for headerName, values := range source {
		if isHopByHopHeader(headerName) {
			continue
		}
		for _, value := range values {
			target.Add(headerName, value)
		}
	}
}

func copyProxyResponseHeaders(target http.Header, source http.Header) {
	for headerName, values := range source {
		if isHopByHopHeader(headerName) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(headerName), "access-control-") {
			continue
		}
		if strings.EqualFold(headerName, servedByHeader) || strings.EqualFold(headerName, proxiedViaHeader) {
			continue
		}
		for _, value := range values {
			target.Add(headerName, value)
		}
	}
}

func isHopByHopHeader(headerName string) bool {
	switch strings.ToLower(strings.TrimSpace(headerName)) {
	case "connection", "proxy-connection", "keep-alive", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func normalizeBackendBaseURL(value string) (string, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return "", errors.New("target backend URL must not be empty")
	}

	parsedValue, err := url.Parse(trimmedValue)
	if err != nil {
		return "", fmt.Errorf("invalid target backend URL: %w", err)
	}

	if parsedValue.Scheme != "http" && parsedValue.Scheme != "https" {
		return "", errors.New("target backend URL scheme must be http or https")
	}
	if parsedValue.Host == "" {
		return "", errors.New("target backend URL host is required")
	}
	if parsedValue.Path != "" && parsedValue.Path != "/" {
		return "", errors.New("target backend URL must not contain a path")
	}
	if parsedValue.RawQuery != "" || parsedValue.Fragment != "" {
		return "", errors.New("target backend URL must not contain query or fragment")
	}

	parsedValue.Path = ""
	parsedValue.RawPath = ""
	return strings.TrimSuffix(parsedValue.String(), "/"), nil
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request, requestID string) {
	var req discoverRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	captchaSessionToken := ""
	if s.cfg.RequireCaptcha {
		captchaSessionToken = strings.TrimSpace(req.CaptchaSessionToken)
		if captchaSessionToken != "" {
			if err := s.validateCaptchaSession(r.RemoteAddr, captchaSessionToken); err != nil {
				captchaSessionToken = ""
			}
		}

		if captchaSessionToken == "" {
			if err := s.validateCaptcha(r.RemoteAddr, req.CaptchaID, req.CaptchaAnswer); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_CAPTCHA", err.Error(), nil)
				return
			}

			issuedSessionToken, err := s.createCaptchaSession(r.RemoteAddr)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "CAPTCHA_SESSION_FAILED", err.Error(), nil)
				return
			}
			captchaSessionToken = issuedSessionToken
		}
	}

	devices, err := s.manager.Discover(r.Context(), req.RepoURL, req.Ref)
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, requestID, "DISCOVERY_FAILED", err.Error(), nil)
		return
	}

	data := discoverResponse{
		RepoURL:             req.RepoURL,
		Ref:                 req.Ref,
		Devices:             devices,
		CaptchaSessionToken: captchaSessionToken,
	}
	s.writeSuccess(w, http.StatusOK, requestID, data)
}

func (s *Server) handleRepoRefs(w http.ResponseWriter, r *http.Request, requestID string) {
	var req repoRefsRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	refs, err := s.manager.DiscoverRefs(r.Context(), req.RepoURL)
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, requestID, "REFS_DISCOVERY_FAILED", err.Error(), nil)
		return
	}

	data := repoRefsResponse{
		RepoURL:        req.RepoURL,
		DefaultBranch:  refs.DefaultBranch,
		RecentBranches: toRepoRefViews(refs.RecentBranches),
		RecentTags:     toRepoRefViews(refs.RecentTags),
	}
	s.writeSuccess(w, http.StatusOK, requestID, data)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request, requestID string) {
	var req createJobRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, requestID, "INVALID_REQUEST", err.Error(), nil)
		return
	}

	captchaSessionToken := ""
	if s.cfg.RequireCaptcha {
		captchaSessionToken = strings.TrimSpace(req.CaptchaSessionToken)
		if captchaSessionToken != "" {
			if err := s.validateCaptchaSession(r.RemoteAddr, captchaSessionToken); err != nil {
				captchaSessionToken = ""
			}
		}

		if captchaSessionToken == "" {
			if err := s.validateCaptcha(r.RemoteAddr, req.CaptchaID, req.CaptchaAnswer); err != nil {
				s.writeError(w, http.StatusBadRequest, requestID, "INVALID_CAPTCHA", err.Error(), nil)
				return
			}

			issuedSessionToken, err := s.createCaptchaSession(r.RemoteAddr)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, requestID, "CAPTCHA_SESSION_FAILED", err.Error(), nil)
				return
			}
			captchaSessionToken = issuedSessionToken
		}
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

	response := s.presentState(state)
	response.CaptchaSessionToken = captchaSessionToken
	s.writeSuccess(w, http.StatusCreated, requestID, response)
}

func (s *Server) handleNewCaptcha(w http.ResponseWriter, r *http.Request, requestID string) {
	if !s.cfg.RequireCaptcha {
		s.writeSuccess(w, http.StatusOK, requestID, captchaResponse{CaptchaRequired: false})
		return
	}

	challenge, err := s.newCaptcha(r.RemoteAddr)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, requestID, "CAPTCHA_GENERATION_FAILED", err.Error(), nil)
		return
	}
	challenge.CaptchaRequired = true

	s.writeSuccess(w, http.StatusOK, requestID, challenge)
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
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-MFB-Target-Backend")
	w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-MFB-Served-By, X-MFB-Proxied-Via")
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
		ID:              state.ID,
		RepoURL:         state.RepoURL,
		Ref:             state.Ref,
		Device:          state.Device,
		Status:          state.Status,
		QueuePosition:   state.QueuePosition,
		QueueETASeconds: state.QueueETASeconds,
		CreatedAt:       state.CreatedAt,
		StartedAt:       state.StartedAt,
		FinishedAt:      state.FinishedAt,
		Error:           state.Error,
		LogLines:        state.LogLines,
		Artifacts:       toArtifactViews(state.ID, state.Artifacts),
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

func toRepoRefViews(refs []jobs.RepoRef) []repoRefView {
	views := make([]repoRefView, len(refs))
	for index, ref := range refs {
		views[index] = repoRefView{
			Name:      ref.Name,
			Commit:    ref.Commit,
			UpdatedAt: ref.UpdatedAt,
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
	host := normalizeRemoteHost(remoteAddr)

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
	RepoURL             string `json:"repoUrl"`
	Ref                 string `json:"ref"`
	CaptchaID           string `json:"captchaId,omitempty"`
	CaptchaAnswer       string `json:"captchaAnswer,omitempty"`
	CaptchaSessionToken string `json:"captchaSessionToken,omitempty"`
}

type repoRefsRequest struct {
	RepoURL string `json:"repoUrl"`
}

type discoverResponse struct {
	RepoURL             string   `json:"repoUrl"`
	Ref                 string   `json:"ref,omitempty"`
	Devices             []string `json:"devices"`
	CaptchaSessionToken string   `json:"captchaSessionToken,omitempty"`
}

type repoRefsResponse struct {
	RepoURL        string        `json:"repoUrl"`
	DefaultBranch  string        `json:"defaultBranch,omitempty"`
	RecentBranches []repoRefView `json:"recentBranches"`
	RecentTags     []repoRefView `json:"recentTags"`
}

type createJobRequest struct {
	RepoURL             string `json:"repoUrl"`
	Ref                 string `json:"ref"`
	Device              string `json:"device"`
	CaptchaID           string `json:"captchaId,omitempty"`
	CaptchaAnswer       string `json:"captchaAnswer,omitempty"`
	CaptchaSessionToken string `json:"captchaSessionToken,omitempty"`
}

type captchaResponse struct {
	CaptchaRequired bool      `json:"captchaRequired"`
	CaptchaID       string    `json:"captchaId,omitempty"`
	Question        string    `json:"question,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
}

type healthResponse struct {
	Status           string   `json:"status"`
	CaptchaRequired  bool     `json:"captchaRequired"`
	NodeBaseURL      string   `json:"nodeBaseUrl,omitempty"`
	ProxyBackendURLs []string `json:"proxyBackendUrls,omitempty"`
	RunningBuilds    int      `json:"runningBuilds"`
	QueuedBuilds     int      `json:"queuedBuilds"`
	ConcurrentBuilds int      `json:"concurrentBuilds"`
}

type logsResponse struct {
	Lines []string `json:"lines"`
}

type stateResponse struct {
	ID                  string         `json:"id"`
	RepoURL             string         `json:"repoUrl"`
	Ref                 string         `json:"ref,omitempty"`
	Device              string         `json:"device"`
	Status              jobs.Status    `json:"status"`
	CaptchaSessionToken string         `json:"captchaSessionToken,omitempty"`
	QueuePosition       *int           `json:"queuePosition,omitempty"`
	QueueETASeconds     *int           `json:"queueEtaSeconds,omitempty"`
	CreatedAt           time.Time      `json:"createdAt"`
	StartedAt           *time.Time     `json:"startedAt,omitempty"`
	FinishedAt          *time.Time     `json:"finishedAt,omitempty"`
	Error               string         `json:"error,omitempty"`
	LogLines            int            `json:"logLines"`
	Artifacts           []artifactView `json:"artifacts"`
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

type repoRefView struct {
	Name      string     `json:"name"`
	Commit    string     `json:"commit,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}
