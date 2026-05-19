package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	workdir := t.TempDir()
	t.Setenv("APP_WORKDIR", workdir)
	t.Setenv("APP_STATS_PASSWORD", "")
	t.Setenv("APP_REQUIRE_CAPTCHA", "")
	t.Setenv("APP_PORT", "")
	t.Setenv("APP_CONCURRENT_BUILDS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != defaultPort {
		t.Fatalf("expected port %d, got %d", defaultPort, cfg.Port)
	}
	if cfg.ConcurrentBuilds != defaultConcurrentBuilds {
		t.Fatalf("expected concurrent builds %d, got %d", defaultConcurrentBuilds, cfg.ConcurrentBuilds)
	}
	if !cfg.RequireCaptcha {
		t.Fatalf("expected captcha enabled by default")
	}
	if cfg.StatsPassword != "" {
		t.Fatalf("expected empty stats password by default")
	}

	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		t.Fatalf("abs workdir: %v", err)
	}
	if cfg.WorkDir != absWorkdir {
		t.Fatalf("expected workdir %q, got %q", absWorkdir, cfg.WorkDir)
	}
}

func TestLoadCustomValues(t *testing.T) {
	workdir := t.TempDir()
	t.Setenv("APP_WORKDIR", workdir)
	t.Setenv("APP_PORT", "9090")
	t.Setenv("APP_CONCURRENT_BUILDS", "2")
	t.Setenv("APP_REQUIRE_CAPTCHA", "false")
	t.Setenv("APP_STATS_PASSWORD", "stats-secret")
	t.Setenv("APP_ALLOWED_ORIGINS", "http://a.test, http://b.test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.ConcurrentBuilds != 2 {
		t.Fatalf("expected concurrent builds 2, got %d", cfg.ConcurrentBuilds)
	}
	if cfg.RequireCaptcha {
		t.Fatalf("expected captcha disabled")
	}
	if cfg.StatsPassword != "stats-secret" {
		t.Fatalf("unexpected stats password: %q", cfg.StatsPassword)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 allowed origins, got %d", len(cfg.AllowedOrigins))
	}
}

func TestLoadRejectsInvalidConcurrentBuilds(t *testing.T) {
	workdir := t.TempDir()
	t.Setenv("APP_WORKDIR", workdir)
	t.Setenv("APP_CONCURRENT_BUILDS", "0")

	if _, err := Load(); err == nil {
		t.Fatalf("expected error for APP_CONCURRENT_BUILDS=0")
	}
}

func TestBoolEnv(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		fallback bool
		want     bool
		wantErr  bool
	}{
		{name: "empty uses fallback", raw: "", fallback: true, want: true},
		{name: "true literal", raw: "1", fallback: false, want: true},
		{name: "false literal", raw: "false", fallback: true, want: false},
		{name: "invalid value", raw: "maybe", fallback: false, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tc.raw)
			got, err := boolEnv("TEST_BOOL", tc.fallback)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	t.Parallel()

	got := splitCSV(" http://a , ,http://b ")
	if len(got) != 2 {
		t.Fatalf("expected 2 values, got %v", got)
	}
	if got[0] != "http://a" || got[1] != "http://b" {
		t.Fatalf("unexpected values: %v", got)
	}
}
