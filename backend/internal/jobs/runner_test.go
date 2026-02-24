package jobs

import "testing"

func TestResolveDockerHostPath(t *testing.T) {
	t.Parallel()

	t.Run("no host mapping", func(t *testing.T) {
		t.Parallel()
		got, err := resolveDockerHostPath("/app/build-workdir/jobs/a/repo", "/app/build-workdir", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/app/build-workdir/jobs/a/repo" {
			t.Fatalf("unexpected path: %q", got)
		}
	})

	t.Run("with host mapping", func(t *testing.T) {
		t.Parallel()
		got, err := resolveDockerHostPath("/app/build-workdir/jobs/a/repo", "/app/build-workdir", "/Users/dev/project/build-workdir")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "/Users/dev/project/build-workdir/jobs/a/repo"
		if got != want {
			t.Fatalf("unexpected path: got=%q want=%q", got, want)
		}
	})

	t.Run("outside workdir", func(t *testing.T) {
		t.Parallel()
		_, err := resolveDockerHostPath("/tmp/other", "/app/build-workdir", "/Users/dev/project/build-workdir")
		if err == nil {
			t.Fatalf("expected error for path outside workdir")
		}
	})
}
