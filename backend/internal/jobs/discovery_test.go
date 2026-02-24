package jobs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListVariantDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	variantsDir := filepath.Join(root, "variants")
	if err := os.MkdirAll(variantsDir, 0o755); err != nil {
		t.Fatalf("create variants dir: %v", err)
	}

	dirs := []string{"tbeam", "heltec-v3", ".internal", "bad name"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(variantsDir, dir), 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(variantsDir, "README.md"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	got, err := listVariantDirectories(root)
	if err != nil {
		t.Fatalf("listVariantDirectories failed: %v", err)
	}

	want := []string{"heltec-v3", "tbeam"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got=%v want=%v", got, want)
	}
}
