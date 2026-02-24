package jobs

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func cloneRepository(ctx context.Context, repoURL string, ref string, destination string, onLine func(string)) error {
	cloneArgs := []string{"clone", "--depth", "1", "--single-branch", repoURL, destination}
	if err := runGit(ctx, onLine, cloneArgs...); err != nil {
		return fmt.Errorf("clone repository: %w", err)
	}

	ref = strings.TrimSpace(ref)
	if ref != "" {
		fetchArgs := []string{"-C", destination, "fetch", "--depth", "1", "origin", ref}
		if err := runGit(ctx, onLine, fetchArgs...); err == nil {
			if err := runGit(ctx, onLine, "-C", destination, "checkout", "--force", "FETCH_HEAD"); err != nil {
				return fmt.Errorf("checkout fetched ref: %w", err)
			}
		} else {
			if err := runGit(ctx, onLine, "-C", destination, "checkout", "--force", ref); err != nil {
				return fmt.Errorf("checkout ref: %w", err)
			}
		}
	}

	optimizedSubmoduleArgs := []string{
		"-C", destination,
		"-c", "submodule.fetchJobs=8",
		"submodule", "update",
		"--init",
		"--recursive",
		"--depth", "1",
		"--jobs", "8",
		"--recommend-shallow",
	}
	if err := runGit(ctx, onLine, optimizedSubmoduleArgs...); err != nil {
		if onLine != nil {
			onLine("submodule optimized mode failed, retrying with compatibility flags")
		}
		if fallbackErr := runGit(ctx, onLine, "-C", destination, "submodule", "update", "--init", "--recursive", "--depth", "1"); fallbackErr != nil {
			return fmt.Errorf("update submodules: %w", fallbackErr)
		}
	}

	return nil
}

func runGit(ctx context.Context, onLine func(string), args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if onLine != nil {
		onLine("$ git " + strings.Join(args, " "))
	}
	if err := runCommandStreaming(ctx, cmd, onLine); err != nil {
		return err
	}
	return nil
}
