package jobs

import (
	"testing"
	"time"
)

func TestParseDefaultBranch(t *testing.T) {
	t.Parallel()

	output := "ref: refs/heads/develop\tHEAD\n8f8e8d8c8b8a7f6e5d4c3b2a1f0e9d8c7b6a5f4e\tHEAD\n"
	if got := parseDefaultBranch(output); got != "develop" {
		t.Fatalf("default branch mismatch: got=%q want=%q", got, "develop")
	}
}

func TestParseLsRemoteRefs(t *testing.T) {
	t.Parallel()

	output := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\trefs/heads/main\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\trefs/heads/develop\n" +
		"cccccccccccccccccccccccccccccccccccccccc\trefs/heads/main\n" +
		"dddddddddddddddddddddddddddddddddddddddd\trefs/heads/bad ref\n"

	refs := parseLsRemoteRefs(output, "refs/heads/")
	if len(refs) != 2 {
		t.Fatalf("unexpected refs count: got=%d want=2", len(refs))
	}
	if refs[0].Name != "main" || refs[1].Name != "develop" {
		t.Fatalf("unexpected refs order/content: %+v", refs)
	}
}

func TestParseForEachRefs(t *testing.T) {
	t.Parallel()

	output := "main\taaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\t1710000000\n" +
		"HEAD\tbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\t1710000001\n" +
		"v2.5.12\tcccccccccccccccccccccccccccccccccccccccc\t1710000100\n"

	refs := parseForEachRefs(output)
	if len(refs) != 2 {
		t.Fatalf("unexpected refs count: got=%d want=2", len(refs))
	}
	if refs[0].Name != "main" || refs[1].Name != "v2.5.12" {
		t.Fatalf("unexpected refs content: %+v", refs)
	}
	if refs[0].UpdatedAt == nil || refs[1].UpdatedAt == nil {
		t.Fatalf("updatedAt must be set for parsed timestamps")
	}

	want := time.Unix(1710000000, 0).UTC()
	if !refs[0].UpdatedAt.Equal(want) {
		t.Fatalf("unexpected parsed time: got=%s want=%s", refs[0].UpdatedAt, want)
	}
}

func TestEnsureDefaultBranchPresent(t *testing.T) {
	t.Parallel()

	result := RepoRefs{
		DefaultBranch:  "main",
		RecentBranches: []RepoRef{{Name: "develop"}},
	}

	ensureDefaultBranchPresent(&result)
	if len(result.RecentBranches) != 2 {
		t.Fatalf("unexpected branches count: got=%d want=2", len(result.RecentBranches))
	}
	if result.RecentBranches[0].Name != "main" {
		t.Fatalf("default branch should be prepended, got=%q", result.RecentBranches[0].Name)
	}
}
