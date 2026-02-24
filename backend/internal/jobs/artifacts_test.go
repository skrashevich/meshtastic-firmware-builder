package jobs

import (
	"os"
	"path/filepath"
	"strings"
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

	// Create firmware files (should be collected)
	firmwareFiles := map[string]string{
		filepath.Join(root, ".pio", "build", device, "firmware.bin"):             "bin",
		filepath.Join(root, ".pio", "build", device, "firmware.hex"):             "hex",
		filepath.Join(root, ".pio", "build", device, "firmware.uf2"):             "uf2",
		filepath.Join(root, ".pio", "build", device, "firmware.elf"):             "elf",
		filepath.Join(root, ".pio", "build", device, "nested", "bootloader.bin"): "boot",
	}

	// Create non-firmware files (should be ignored)
	nonFirmwareFiles := map[string]string{
		filepath.Join(root, ".pio", "build", device, "firmware.map"):        "map",
		filepath.Join(root, ".pio", "build", device, "output.o"):            "obj",
		filepath.Join(root, ".pio", "build", device, "library.a"):           "lib",
		filepath.Join(root, ".pio", "build", device, "nested", "debug.txt"): "debug",
		filepath.Join(root, ".pio", "build", device, "readme.md"):           "doc",
		filepath.Join(root, ".pio", "build", device, "config.json"):         "config",
	}

	// Write all files
	for path, content := range firmwareFiles {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("create firmware file %s: %v", path, err)
		}
	}
	for path, content := range nonFirmwareFiles {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("create non-firmware file %s: %v", path, err)
		}
	}

	artifacts, err := collectArtifacts(root, device)
	if err != nil {
		t.Fatalf("collectArtifacts failed: %v", err)
	}

	// Should only collect firmware files (5 files)
	if len(artifacts) != 5 {
		t.Fatalf("unexpected artifacts count: got=%d want=5 (only firmware files)", len(artifacts))
	}

	// Verify all collected artifacts are firmware files
	filenames := make([]string, len(artifacts))
	for i, artifact := range artifacts {
		filenames[i] = artifact.Name
		if artifact.ID == "" {
			t.Fatalf("artifact ID must not be empty")
		}
		if artifact.AbsolutePath() == "" {
			t.Fatalf("artifact absolute path must not be empty")
		}

		// Verify extension is firmware type
		ext := filepath.Ext(artifact.Name)
		isFirmware := false
		for _, fwExt := range firmwareExtensions {
			if strings.ToLower(ext) == fwExt {
				isFirmware = true
				break
			}
		}
		if !isFirmware {
			t.Fatalf("non-firmware file collected: %s", artifact.Name)
		}
	}

	// Verify specific firmware files are collected
	expectedFiles := []string{"bootloader.bin", "firmware.bin", "firmware.hex", "firmware.uf2", "firmware.elf"}
	for _, expected := range expectedFiles {
		found := false
		for _, filename := range filenames {
			if filename == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected firmware file not found: %s (collected: %v)", expected, filenames)
		}
	}
}
