package jobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	maxRecentBranches = 20
	maxRecentTags     = 20
)

type RepoRef struct {
	Name      string     `json:"name"`
	Commit    string     `json:"commit,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type RepoRefs struct {
	RepoURL        string    `json:"repoUrl"`
	DefaultBranch  string    `json:"defaultBranch,omitempty"`
	RecentBranches []RepoRef `json:"recentBranches"`
	RecentTags     []RepoRef `json:"recentTags"`
}

func discoverRefs(ctx context.Context, discoveryRoot string, repoURL string) (RepoRefs, error) {
	result := RepoRefs{
		RepoURL:        repoURL,
		RecentBranches: make([]RepoRef, 0, maxRecentBranches),
		RecentTags:     make([]RepoRef, 0, maxRecentTags),
	}

	defaultBranchOutput, err := runGitCapture(ctx, "ls-remote", "--symref", repoURL, "HEAD")
	if err != nil {
		return RepoRefs{}, fmt.Errorf("read repository refs: %w", err)
	}
	result.DefaultBranch = parseDefaultBranch(defaultBranchOutput)

	branchesOutput, err := runGitCapture(ctx, "ls-remote", "--heads", repoURL)
	if err != nil {
		return RepoRefs{}, fmt.Errorf("read remote branches: %w", err)
	}
	result.RecentBranches = parseLsRemoteRefs(branchesOutput, "refs/heads/")

	tagsOutput, err := runGitCapture(ctx, "ls-remote", "--tags", "--refs", repoURL)
	if err == nil {
		result.RecentTags = parseLsRemoteRefs(tagsOutput, "refs/tags/")
	}

	_ = enrichRefsWithDates(ctx, discoveryRoot, repoURL, &result)
	ensureDefaultBranchPresent(&result)
	result.RecentBranches = limitRepoRefs(result.RecentBranches, maxRecentBranches)
	result.RecentTags = limitRepoRefs(result.RecentTags, maxRecentTags)

	return result, nil
}

func enrichRefsWithDates(ctx context.Context, discoveryRoot string, repoURL string, result *RepoRefs) error {
	if result == nil {
		return nil
	}

	tempDir, err := os.MkdirTemp(discoveryRoot, "refs-*")
	if err != nil {
		return fmt.Errorf("create refs workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	repoPath := filepath.Join(tempDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		return fmt.Errorf("create refs repository directory: %w", err)
	}

	if _, err := runGitCapture(ctx, "init", "--quiet", repoPath); err != nil {
		return err
	}
	if _, err := runGitCapture(ctx, "-C", repoPath, "remote", "add", "origin", repoURL); err != nil {
		return err
	}
	if _, err := runGitCapture(ctx, "-C", repoPath, "fetch", "--depth", "1", "--no-tags", "origin", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return err
	}

	branchOutput, err := runGitCapture(
		ctx,
		"-C", repoPath,
		"for-each-ref", "refs/remotes/origin",
		"--sort=-committerdate",
		"--format=%(refname:strip=3)%09%(objectname)%09%(committerdate:unix)",
	)
	if err == nil {
		branches := parseForEachRefs(branchOutput)
		if len(branches) > 0 {
			result.RecentBranches = branches
		}
	}

	if _, err := runGitCapture(ctx, "-C", repoPath, "fetch", "--depth", "1", "--tags", "origin"); err == nil {
		tagsOutput, tagsErr := runGitCapture(
			ctx,
			"-C", repoPath,
			"for-each-ref", "refs/tags",
			"--sort=-creatordate",
			"--format=%(refname:strip=2)%09%(objectname)%09%(creatordate:unix)",
		)
		if tagsErr == nil {
			tags := parseForEachRefs(tagsOutput)
			if len(tags) > 0 {
				result.RecentTags = tags
			}
		}
	}

	return nil
}

func parseDefaultBranch(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		if fields[0] != "ref:" || fields[2] != "HEAD" {
			continue
		}
		if !strings.HasPrefix(fields[1], "refs/heads/") {
			continue
		}
		name := strings.TrimPrefix(fields[1], "refs/heads/")
		if ValidateRef(name) == nil {
			return name
		}
	}
	return ""
}

func parseLsRemoteRefs(output string, prefix string) []RepoRef {
	refs := make([]RepoRef, 0, 32)
	seen := make(map[string]struct{}, 32)

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		if !strings.HasPrefix(parts[1], prefix) {
			continue
		}

		name := strings.TrimPrefix(parts[1], prefix)
		if name == "" || name == "HEAD" || ValidateRef(name) != nil {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}

		seen[name] = struct{}{}
		refs = append(refs, RepoRef{Name: name, Commit: strings.TrimSpace(parts[0])})
	}

	return refs
}

func parseForEachRefs(output string) []RepoRef {
	refs := make([]RepoRef, 0, 32)
	seen := make(map[string]struct{}, 32)

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		commit := strings.TrimSpace(parts[1])
		if name == "" || name == "HEAD" || ValidateRef(name) != nil {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}

		ref := RepoRef{Name: name, Commit: commit}
		if len(parts) >= 3 {
			if stamp, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64); err == nil && stamp > 0 {
				timeValue := time.Unix(stamp, 0).UTC()
				ref.UpdatedAt = &timeValue
			}
		}

		seen[name] = struct{}{}
		refs = append(refs, ref)
	}

	return refs
}

func ensureDefaultBranchPresent(result *RepoRefs) {
	if result == nil {
		return
	}

	if strings.TrimSpace(result.DefaultBranch) == "" {
		if len(result.RecentBranches) > 0 {
			result.DefaultBranch = result.RecentBranches[0].Name
		}
		return
	}

	for _, branch := range result.RecentBranches {
		if branch.Name == result.DefaultBranch {
			return
		}
	}

	result.RecentBranches = append([]RepoRef{{Name: result.DefaultBranch}}, result.RecentBranches...)
}

func limitRepoRefs(refs []RepoRef, limit int) []RepoRef {
	if limit <= 0 || len(refs) <= limit {
		return refs
	}
	return refs[:limit]
}

func runGitCapture(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		message := strings.TrimSpace(string(output))
		if message == "" {
			return "", fmt.Errorf("git command failed: %w", err)
		}
		return "", fmt.Errorf("git command failed: %s", message)
	}

	return string(output), nil
}
