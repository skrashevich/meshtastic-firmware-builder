package jobs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	device := "tbeam"

	buildDir := filepath.Join(root, ".pio", "build", device, "nested")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatalf("create build dir: %v", err)
	}

	files := map[string]string{
		filepath.Join(root, ".pio", "build", device, "firmware.bin"):      "bin",
		filepath.Join(root, ".pio", "build", device, "firmware.hex"):      "hex",
		filepath.Join(root, ".pio", "build", device, "nested", "map.txt"): "map",
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("create file %s: %v", path, err)
		}
	}

	artifacts, err := collectArtifacts(root, device)
	if err != nil {
		t.Fatalf("collectArtifacts failed: %v", err)
	}

	if len(artifacts) != 3 {
		t.Fatalf("unexpected artifacts count: got=%d want=3", len(artifacts))
	}

	for _, artifact := range artifacts {
		if artifact.ID == "" {
			t.Fatalf("artifact ID must not be empty")
		}
		if artifact.AbsolutePath() == "" {
			t.Fatalf("artifact absolute path must not be empty")
		}
	}
}
