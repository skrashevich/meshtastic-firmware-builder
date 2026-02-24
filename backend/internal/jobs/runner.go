package jobs

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"meshtastic-firmware-builder/backend/internal/config"
)

func runBuildInContainer(ctx context.Context, cfg config.Config, repoPath string, device string, onLine func(string)) error {
	containerProjectPath := "/workspace/repo"

	hostRepoPath, err := resolveDockerHostPath(repoPath, cfg.WorkDir, cfg.DockerHostWorkDir)
	if err != nil {
		return fmt.Errorf("resolve repository mount path: %w", err)
	}

	hostCachePath := cfg.PlatformIOCache
	if cfg.DockerHostCache != "" {
		hostCachePath = cfg.DockerHostCache
	} else {
		hostCachePath, err = resolveDockerHostPath(cfg.PlatformIOCache, cfg.WorkDir, cfg.DockerHostWorkDir)
		if err != nil {
			return fmt.Errorf("resolve cache mount path: %w", err)
		}
	}

	repoMount := fmt.Sprintf("%s:/workspace/repo", hostRepoPath)
	cacheMount := fmt.Sprintf("%s:/root/.platformio", hostCachePath)

	args := []string{
		"run",
		"--rm",
		"-e", "CI=true",
		"-e", "PLATFORMIO_NO_ANSI=true",
		"-v", repoMount,
		"-v", cacheMount,
		"-w", containerProjectPath,
		cfg.BuilderImage,
		"run",
		"-d", containerProjectPath,
		"-e", device,
		"-j", strconv.Itoa(cfg.PlatformIOJobs),
	}

	if onLine != nil {
		onLine("$ docker " + strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	if err := runCommandStreaming(ctx, cmd, onLine); err != nil {
		return fmt.Errorf("run build container: %w", err)
	}

	return nil
}

func resolveDockerHostPath(path string, containerRoot string, hostRoot string) (string, error) {
	if strings.TrimSpace(hostRoot) == "" {
		return path, nil
	}

	rel, err := filepath.Rel(containerRoot, path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q is outside APP_WORKDIR %q", path, containerRoot)
	}

	if rel == "." {
		return hostRoot, nil
	}

	return filepath.Join(hostRoot, rel), nil
}
