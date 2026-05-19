package httpapi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/config"
)

func TestHandleHealthz(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{
		RequireCaptcha: false,
		StatsPassword:  "secret",
	}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var envelope struct {
		Data healthResponse `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.Status != "ok" {
		t.Fatalf("unexpected status: %q", envelope.Data.Status)
	}
	if envelope.Data.CaptchaRequired {
		t.Fatalf("expected captchaRequired=false")
	}
	if !envelope.Data.StatsEnabled {
		t.Fatalf("expected statsEnabled=true when stats password is set")
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected X-Request-ID header")
	}
}

func TestHandleHealthzCaptchaRequired(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{RequireCaptcha: true}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	server.ServeHTTP(recorder, request)

	var envelope struct {
		Data healthResponse `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.Data.CaptchaRequired {
		t.Fatalf("expected captchaRequired=true")
	}
}

func TestHandleCORSRejectsUnknownOrigin(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{
		AllowedOrigins: []string{"http://localhost:5173"},
	}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	request.Header.Set("Origin", "http://evil.example")
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", recorder.Code)
	}
}

func TestHandleCORSAllowsConfiguredOrigin(t *testing.T) {
	t.Parallel()

	const origin = "http://localhost:5173"
	server := NewServer(config.Config{
		AllowedOrigins: []string{origin},
	}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/healthz", nil)
	request.Header.Set("Origin", origin)
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("expected Access-Control-Allow-Origin=%q, got %q", origin, got)
	}
}

func TestHandleNewCaptchaDisabled(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{RequireCaptcha: false}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/captcha", nil)
	server.ServeHTTP(recorder, request)

	var envelope struct {
		Data struct {
			CaptchaRequired bool `json:"captchaRequired"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.CaptchaRequired {
		t.Fatalf("expected captchaRequired=false when captcha is disabled")
	}
}

func TestHandleNewCaptchaEnabled(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{RequireCaptcha: true}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/captcha", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	server.ServeHTTP(recorder, request)

	var envelope struct {
		Data struct {
			CaptchaRequired bool   `json:"captchaRequired"`
			CaptchaID       string `json:"captchaId"`
			Question        string `json:"question"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.Data.CaptchaRequired {
		t.Fatalf("expected captchaRequired=true")
	}
	if envelope.Data.CaptchaID == "" || envelope.Data.Question == "" {
		t.Fatalf("expected captcha challenge payload")
	}
}

func TestHandleStatsRequiresAuth(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{StatsPassword: "secret"}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", recorder.Code)
	}
}

func TestHandleStatsHiddenWhenDisabled(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	request.Header.Set("Authorization", "Bearer anything")
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}
}

func TestHandleUnknownRoute(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}

	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error.Code != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND error code, got %q", envelope.Error.Code)
	}
}

func TestHandleCreateJobValidation(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{RequireCaptcha: false}, nil, log.New(io.Discard, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(`{"repoUrl":"","ref":"","device":""}`))
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusOK {
		t.Fatalf("expected validation error, got 200")
	}
}
