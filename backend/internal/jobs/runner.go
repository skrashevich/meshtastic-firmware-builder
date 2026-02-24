package jobs

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"meshtastic-firmware-builder/backend/internal/config"
)

func runBuildInContainer(ctx context.Context, cfg config.Config, repoPath string, device string, onLine func(string)) error {
	containerProjectPath := "/workspace/repo"

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
