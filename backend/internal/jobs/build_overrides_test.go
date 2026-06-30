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

	configPath, envName, err := prepareBuildConfigOverrides(repoPath, "tbeam", "abc123", "2.7.26.54e0d8d", BuildOptions{})
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
	configPath, envName, err = prepareBuildConfigOverrides(repoPath, "tbeam", "abc123", "2.7.26.54e0d8d", options)
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
		"-DAPP_VERSION=\\\"2.7.26.54e0d8d\\\"",
		"-DAPP_VERSION_SHORT=\\\"2.7.26\\\"",
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

func TestRenderBuildOverrideConfigAddsVersionFallbacksForLibOnlyOverrides(t *testing.T) {
	t.Parallel()

	text := renderBuildOverrideConfig("t-deck-tft", "mfb-custom-job", "2.7.26.54e0d8d", BuildOptions{
		LibDeps: []string{"https://github.com/skrashevich/device-ui/archive/refs/heads/master.zip"},
	})

	checks := []string{
		"build_flags =",
		"${env:t-deck-tft.build_flags}",
		"-DAPP_VERSION=\\\"2.7.26.54e0d8d\\\"",
		"-DAPP_VERSION_SHORT=\\\"2.7.26\\\"",
		"lib_deps =",
		"https://github.com/skrashevich/device-ui/archive/refs/heads/master.zip",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("generated config missing %q:\n%s", check, text)
		}
	}
}

func TestRenderBuildOverrideConfigDoesNotDuplicateUserAppVersion(t *testing.T) {
	t.Parallel()

	text := renderBuildOverrideConfig("tbeam", "mfb-custom-job", "2.7.26.54e0d8d", BuildOptions{
		BuildFlags: []string{"-DAPP_VERSION=\\\"custom\\\""},
		LibDeps:    []string{"custom/lib"},
	})

	if strings.Contains(text, "2.7.26.54e0d8d") {
		t.Fatalf("generated config should not add version fallback when user supplied APP_VERSION:\n%s", text)
	}
	if !strings.Contains(text, "-DAPP_VERSION=\\\"custom\\\"") {
		t.Fatalf("generated config missing user APP_VERSION:\n%s", text)
	}
}
