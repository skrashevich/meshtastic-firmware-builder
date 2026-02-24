package jobs

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"meshtastic-firmware-builder/backend/internal/config"
)

func runBuildInContainer(ctx context.Context, cfg config.Config, repoPath string, projectPath string, device string, onLine func(string)) error {
	relProjectPath, err := filepath.Rel(repoPath, projectPath)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	if relProjectPath == "." {
		relProjectPath = ""
	}
	if strings.HasPrefix(relProjectPath, "..") {
		return fmt.Errorf("project path %q is outside repository", projectPath)
	}

	containerProjectPath := "/workspace/repo"
	if relProjectPath != "" {
		containerProjectPath = path.Join(containerProjectPath, filepath.ToSlash(relProjectPath))
	}

	repoMount := fmt.Sprintf("%s:/workspace/repo", repoPath)
	cacheMount := fmt.Sprintf("%s:/root/.platformio", cfg.PlatformIOCache)

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
