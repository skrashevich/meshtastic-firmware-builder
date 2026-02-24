package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort                = 8080
	defaultWorkDir             = "../build-workdir"
	defaultConcurrentBuilds    = 1
	defaultRetentionHours      = 168
	defaultBuildTimeoutMinutes = 60
	defaultBuilderImage        = "meshtastic-pio-builder:latest"
	defaultPlatformIOJobs      = 1
	defaultAllowedOrigins      = "http://localhost:5173"
	defaultMaxLogLines         = 20000
	defaultBuildRateLimit      = 10
)

type Config struct {
	Port              int
	WorkDir           string
	DockerHostWorkDir string
	ConcurrentBuilds  int
	Retention         time.Duration
	BuildTimeout      time.Duration
	BuilderImage      string
	PlatformIOJobs    int
	AllowedOrigins    []string
	PlatformIOCache   string
	DockerHostCache   string
	MaxLogLines       int
	BuildRateLimit    int
	CleanupInterval   time.Duration
	DiscoveryRootPath string
	JobsRootPath      string
}

func Load() (Config, error) {
	port, err := intEnv("APP_PORT", defaultPort)
	if err != nil {
		return Config{}, err
	}

	concurrentBuilds, err := intEnv("APP_CONCURRENT_BUILDS", defaultConcurrentBuilds)
	if err != nil {
		return Config{}, err
	}
	if concurrentBuilds < 1 {
		return Config{}, fmt.Errorf("APP_CONCURRENT_BUILDS must be >= 1")
	}

	retentionHours, err := intEnv("APP_RETENTION_HOURS", defaultRetentionHours)
	if err != nil {
		return Config{}, err
	}
	if retentionHours < 1 {
		return Config{}, fmt.Errorf("APP_RETENTION_HOURS must be >= 1")
	}

	buildTimeoutMinutes, err := intEnv("APP_BUILD_TIMEOUT_MINUTES", defaultBuildTimeoutMinutes)
	if err != nil {
		return Config{}, err
	}
	if buildTimeoutMinutes < 1 {
		return Config{}, fmt.Errorf("APP_BUILD_TIMEOUT_MINUTES must be >= 1")
	}

	platformIOJobs, err := intEnv("APP_PLATFORMIO_JOBS", defaultPlatformIOJobs)
	if err != nil {
		return Config{}, err
	}
	if platformIOJobs < 1 {
		return Config{}, fmt.Errorf("APP_PLATFORMIO_JOBS must be >= 1")
	}

	maxLogLines, err := intEnv("APP_MAX_LOG_LINES", defaultMaxLogLines)
	if err != nil {
		return Config{}, err
	}
	if maxLogLines < 100 {
		return Config{}, fmt.Errorf("APP_MAX_LOG_LINES must be >= 100")
	}

	buildRateLimit, err := intEnv("APP_BUILD_RATE_LIMIT_PER_MINUTE", defaultBuildRateLimit)
	if err != nil {
		return Config{}, err
	}
	if buildRateLimit < 1 {
		return Config{}, fmt.Errorf("APP_BUILD_RATE_LIMIT_PER_MINUTE must be >= 1")
	}

	workDir := os.Getenv("APP_WORKDIR")
	if strings.TrimSpace(workDir) == "" {
		workDir = defaultWorkDir
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve APP_WORKDIR: %w", err)
	}

	discoveryRoot := filepath.Join(workDir, "discovery")
	jobsRoot := filepath.Join(workDir, "jobs")

	platformIOCache := os.Getenv("APP_PLATFORMIO_CACHE_DIR")
	if strings.TrimSpace(platformIOCache) == "" {
		platformIOCache = filepath.Join(workDir, "platformio-cache")
	}
	platformIOCache, err = filepath.Abs(platformIOCache)
	if err != nil {
		return Config{}, fmt.Errorf("resolve APP_PLATFORMIO_CACHE_DIR: %w", err)
	}

	dockerHostWorkDir := strings.TrimSpace(os.Getenv("APP_DOCKER_HOST_WORKDIR"))
	if dockerHostWorkDir != "" {
		if !filepath.IsAbs(dockerHostWorkDir) {
			return Config{}, fmt.Errorf("APP_DOCKER_HOST_WORKDIR must be an absolute path")
		}
		dockerHostWorkDir = filepath.Clean(dockerHostWorkDir)
	}

	dockerHostCache := strings.TrimSpace(os.Getenv("APP_DOCKER_HOST_CACHE_DIR"))
	if dockerHostCache != "" {
		if !filepath.IsAbs(dockerHostCache) {
			return Config{}, fmt.Errorf("APP_DOCKER_HOST_CACHE_DIR must be an absolute path")
		}
		dockerHostCache = filepath.Clean(dockerHostCache)
	}

	if err := ensureDir(discoveryRoot); err != nil {
		return Config{}, err
	}
	if err := ensureDir(jobsRoot); err != nil {
		return Config{}, err
	}
	if err := ensureDir(platformIOCache); err != nil {
		return Config{}, err
	}

	builderImage := strings.TrimSpace(os.Getenv("APP_BUILDER_IMAGE"))
	if builderImage == "" {
		builderImage = defaultBuilderImage
	}

	allowedOrigins := splitCSV(os.Getenv("APP_ALLOWED_ORIGINS"))
	if len(allowedOrigins) == 0 {
		allowedOrigins = splitCSV(defaultAllowedOrigins)
	}

	return Config{
		Port:              port,
		WorkDir:           workDir,
		DockerHostWorkDir: dockerHostWorkDir,
		ConcurrentBuilds:  concurrentBuilds,
		Retention:         time.Duration(retentionHours) * time.Hour,
		BuildTimeout:      time.Duration(buildTimeoutMinutes) * time.Minute,
		BuilderImage:      builderImage,
		PlatformIOJobs:    platformIOJobs,
		AllowedOrigins:    allowedOrigins,
		PlatformIOCache:   platformIOCache,
		DockerHostCache:   dockerHostCache,
		MaxLogLines:       maxLogLines,
		BuildRateLimit:    buildRateLimit,
		CleanupInterval:   time.Hour,
		DiscoveryRootPath: discoveryRoot,
		JobsRootPath:      jobsRoot,
	}, nil
}

func intEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return value, nil
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	return nil
}
