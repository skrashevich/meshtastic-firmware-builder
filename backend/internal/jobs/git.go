package jobs

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func cloneRepository(ctx context.Context, repoURL string, ref string, destination string, onLine func(string)) error {
	cloneArgs := []string{"clone", "--depth", "1", repoURL, destination}
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

	if err := runGit(ctx, onLine, "-C", destination, "submodule", "update", "--init", "--recursive", "--depth", "1"); err != nil {
		return fmt.Errorf("update submodules: %w", err)
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
