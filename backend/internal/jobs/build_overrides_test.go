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

func TestPrepareBuildConfigOverridesReplacesReferencedDeviceUILibDeps(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	platformioPath := filepath.Join(repoPath, "platformio.ini")
	content := strings.Join([]string{
		"[device-ui_base]",
		"lib_deps =",
		"  https://github.com/meshtastic/device-ui/archive/refs/heads/master.zip",
		"",
		"[env:t-deck-tft]",
		"build_flags = -DBASE",
		"lib_deps =",
		"  ${device-ui_base.lib_deps}",
		"",
	}, "\n")
	if err := os.WriteFile(platformioPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write base platformio.ini: %v", err)
	}

	replacement := "https://github.com/skrashevich/device-ui/archive/refs/heads/feature/yawerty-layout.zip"
	cacheBustedReplacement := replacement + "?mfb=mfb-custom-abc123"
	_, _, err := prepareBuildConfigOverrides(repoPath, "t-deck-tft", "abc123", "2.7.26.54e0d8d", BuildOptions{
		LibDeps: []string{replacement},
	})
	if err != nil {
		t.Fatalf("prepare build override config: %v", err)
	}

	patched, err := os.ReadFile(platformioPath)
	if err != nil {
		t.Fatalf("read patched platformio.ini: %v", err)
	}
	text := string(patched)
	if !strings.Contains(text, "  "+cacheBustedReplacement) {
		t.Fatalf("platformio.ini missing replacement lib_dep:\n%s", text)
	}
	if strings.Contains(text, "  ${device-ui_base.lib_deps}") {
		t.Fatalf("platformio.ini should replace referenced device-ui lib_dep:\n%s", text)
	}
	if strings.Contains(text, "  ${env:t-deck-tft.lib_deps}") {
		t.Fatalf("generated override should not re-add inherited lib_deps after replacement:\n%s", text)
	}
}

func TestPrepareBuildConfigOverridesReplacesVariantDeviceUILibDeps(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoPath, "platformio.ini"), []byte("[env]\nbuild_flags = -DBASE\n"), 0o644); err != nil {
		t.Fatalf("write root platformio.ini: %v", err)
	}

	variantPath := filepath.Join(repoPath, "variants", "esp32s3", "t-deck")
	if err := os.MkdirAll(variantPath, 0o755); err != nil {
		t.Fatalf("create variant dir: %v", err)
	}
	variantConfigPath := filepath.Join(variantPath, "platformio.ini")
	variantConfig := strings.Join([]string{
		"[env:t-deck-tft]",
		"lib_deps =",
		"  ${env:t-deck.lib_deps}",
		"  https://github.com/meshtastic/device-ui/archive/1c45ebc7433acb8ba3fe96a6f7deca9c43fa54cf.zip",
		"  https://github.com/bitbank2/bb_captouch/archive/refs/tags/1.3.1.zip",
		"",
	}, "\n")
	if err := os.WriteFile(variantConfigPath, []byte(variantConfig), 0o644); err != nil {
		t.Fatalf("write variant platformio.ini: %v", err)
	}

	replacement := "https://github.com/skrashevich/device-ui/archive/refs/heads/feature/yawerty-layout.zip"
	cacheBustedReplacement := replacement + "?mfb=mfb-custom-abc123"
	_, _, err := prepareBuildConfigOverrides(repoPath, "t-deck-tft", "abc123", "2.7.26.54e0d8d", BuildOptions{
		LibDeps: []string{replacement},
	})
	if err != nil {
		t.Fatalf("prepare build override config: %v", err)
	}

	patched, err := os.ReadFile(variantConfigPath)
	if err != nil {
		t.Fatalf("read patched variant platformio.ini: %v", err)
	}
	text := string(patched)
	checks := []string{
		"  ${env:t-deck.lib_deps}",
		"  " + cacheBustedReplacement,
		"  https://github.com/bitbank2/bb_captouch/archive/refs/tags/1.3.1.zip",
	}
	for _, check := range checks {
		if !strings.Contains(text, check) {
			t.Fatalf("patched variant platformio.ini missing %q:\n%s", check, text)
		}
	}
	if strings.Contains(text, "github.com/meshtastic/device-ui") {
		t.Fatalf("patched variant platformio.ini still contains upstream device-ui:\n%s", text)
	}

	root, err := os.ReadFile(filepath.Join(repoPath, "platformio.ini"))
	if err != nil {
		t.Fatalf("read root platformio.ini: %v", err)
	}
	if strings.Contains(string(root), replacement) {
		t.Fatalf("generated override should not append replacement after patching variant config:\n%s", string(root))
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
