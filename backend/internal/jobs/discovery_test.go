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

	if err := os.MkdirAll(filepath.Join(variantsDir, "unfinished-device"), 0o755); err != nil {
		t.Fatalf("create unfinished-device: %v", err)
	}

	if err := os.WriteFile(filepath.Join(variantsDir, "tbeam", "platformio.ini"), []byte("[env:tbeam]\n"), 0o644); err != nil {
		t.Fatalf("create tbeam platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "heltec-v3", "platformio.ini"), []byte("[env:heltec-v3]\n"), 0o644); err != nil {
		t.Fatalf("create heltec-v3 platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, ".internal", "platformio.ini"), []byte("[env:internal]\n"), 0o644); err != nil {
		t.Fatalf("create .internal platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "bad name", "platformio.ini"), []byte("[env:bad]\n"), 0o644); err != nil {
		t.Fatalf("create bad name platformio.ini: %v", err)
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
