package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareBuildConfigOverrides(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	platformioPath := filepath.Join(repoPath, "platformio.ini")
	if err := os.WriteFile(platformioPath, []byte("[env:tbeam]\nbuild_flags = -DBASE\nlib_deps = base/lib\n"), 0o644); err != nil {
		t.Fatalf("write base platformio.ini: %v", err)
	}

	configPath, envName, err := prepareBuildConfigOverrides(repoPath, "tbeam", "abc123", BuildOptions{})
	if err != nil {
		t.Fatalf("unexpected error for empty options: %v", err)
	}
	if configPath != "" {
		t.Fatalf("expected empty config path, got %q", configPath)
	}
	if envName != "tbeam" {
		t.Fatalf("expected base env name, got %q", envName)
	}

	options := BuildOptions{
		BuildFlags: []string{"-DUSER_FLAG=1", "-Wall"},
		LibDeps:    []string{"bblanchon/ArduinoJson @ ^7"},
	}
	configPath, envName, err = prepareBuildConfigOverrides(repoPath, "tbeam", "abc123", options)
	if err != nil {
		t.Fatalf("prepare build override config: %v", err)
	}
	if configPath != "" {
		t.Fatalf("expected platformio.ini to be patched in place, got config path %q", configPath)
	}
	if !strings.HasPrefix(envName, "mfb-custom-") {
		t.Fatalf("unexpected override env name: %q", envName)
	}

	content, err := os.ReadFile(platformioPath)
	if err != nil {
		t.Fatalf("read patched platformio.ini: %v", err)
	}
	text := string(content)

	checks := []string{
		"[env:tbeam]",
		"extends = env:tbeam",
		"build_flags =",
		"${env:tbeam.build_flags}",
		"-DUSER_FLAG=1",
		"lib_deps =",
		"${env:tbeam.lib_deps}",
		"bblanchon/ArduinoJson @ ^7",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("generated config missing %q:\n%s", check, text)
		}
	}
}
