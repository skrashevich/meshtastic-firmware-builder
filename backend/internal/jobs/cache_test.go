package jobs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildFirmwareCacheKey(t *testing.T) {
	t.Parallel()

	options := BuildOptions{
		BuildFlags: []string{"-DUSER_NAME=alice", "-Wall"},
		LibDeps:    []string{"bblanchon/ArduinoJson @ ^7"},
	}

	first, err := buildFirmwareCacheKey("https://github.com/example/firmware.git", "abc1234", "tbeam", options)
	if err != nil {
		t.Fatalf("buildFirmwareCacheKey failed: %v", err)
	}
	second, err := buildFirmwareCacheKey("https://github.com/example/firmware.git", "abc1234", "tbeam", options)
	if err != nil {
		t.Fatalf("buildFirmwareCacheKey failed: %v", err)
	}

	if first != second {
		t.Fatalf("cache key must be deterministic: %q != %q", first, second)
	}

	third, err := buildFirmwareCacheKey("https://github.com/example/firmware.git", "abc1235", "tbeam", options)
	if err != nil {
		t.Fatalf("buildFirmwareCacheKey failed: %v", err)
	}
	if first == third {
		t.Fatalf("cache key must change when commit changes")
	}

	fourth, err := buildFirmwareCacheKey("https://github.com/example/firmware.git", "abc1234", "tbeam", BuildOptions{
		BuildFlags: []string{"-DUSER_NAME=bob"},
		LibDeps:    []string{"bblanchon/ArduinoJson @ ^7"},
	})
	if err != nil {
		t.Fatalf("buildFirmwareCacheKey failed: %v", err)
	}
	if first == fourth {
		t.Fatalf("cache key must change when build options change")
	}
}

func TestStoreAndLoadArtifactsFromFirmwareCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	sourceDir := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(sourceDir, "nested"), 0o755); err != nil {
		t.Fatalf("create source dir: %v", err)
	}

	firstPath := filepath.Join(sourceDir, "firmware.bin")
	secondPath := filepath.Join(sourceDir, "nested", "bootloader.bin")
	if err := os.WriteFile(firstPath, []byte("firmware-data"), 0o644); err != nil {
		t.Fatalf("write first artifact: %v", err)
	}
	if err := os.WriteFile(secondPath, []byte("boot-data"), 0o644); err != nil {
		t.Fatalf("write second artifact: %v", err)
	}

	artifacts := []Artifact{
		{Name: "firmware.bin", RelativePath: "firmware.bin", absPath: firstPath},
		{Name: "bootloader.bin", RelativePath: "nested/bootloader.bin", absPath: secondPath},
	}

	if err := storeArtifactsInFirmwareCache(root, key, artifacts); err != nil {
		t.Fatalf("storeArtifactsInFirmwareCache failed: %v", err)
	}

	loaded, hit, err := loadArtifactsFromFirmwareCache(root, key)
	if err != nil {
		t.Fatalf("loadArtifactsFromFirmwareCache failed: %v", err)
	}
	if !hit {
		t.Fatalf("expected cache hit")
	}
	if len(loaded) != 2 {
		t.Fatalf("unexpected loaded artifacts count: got=%d want=2", len(loaded))
	}

	if loaded[0].ID != "1" || loaded[1].ID != "2" {
		t.Fatalf("unexpected artifact IDs: %+v", loaded)
	}

	paths := []string{loaded[0].RelativePath, loaded[1].RelativePath}
	if !reflect.DeepEqual(paths, []string{"firmware.bin", "nested/bootloader.bin"}) {
		t.Fatalf("unexpected loaded relative paths: %v", paths)
	}

	for _, artifact := range loaded {
		if _, err := os.Stat(artifact.AbsolutePath()); err != nil {
			t.Fatalf("cached artifact missing %q: %v", artifact.RelativePath, err)
		}
	}
}

func TestLoadArtifactsFromFirmwareCacheMiss(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	key := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	loaded, hit, err := loadArtifactsFromFirmwareCache(root, key)
	if err != nil {
		t.Fatalf("loadArtifactsFromFirmwareCache failed: %v", err)
	}
	if hit {
		t.Fatalf("expected cache miss")
	}
	if len(loaded) != 0 {
		t.Fatalf("expected no artifacts on miss: %v", loaded)
	}
}
