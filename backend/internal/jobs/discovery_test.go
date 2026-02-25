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

	dirs := []string{
		"esp32/tbeam",
		"esp32/t-deck",
		"esp32/heltec-v3",
		"esp32/.internal",
		"esp32/bad name",
		"esp32/no-config",
		"nrf52840/tbeam",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(variantsDir, dir), 0o755); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", "tbeam", "platformio.ini"), []byte("[env:tbeam]\n"), 0o644); err != nil {
		t.Fatalf("create tbeam platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", "t-deck", "platformio.ini"), []byte("[env:t-deck]\n[env:t-deck-tft]\n"), 0o644); err != nil {
		t.Fatalf("create t-deck platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", "heltec-v3", "platformio.ini"), []byte("[env:heltec-v3]\n"), 0o644); err != nil {
		t.Fatalf("create heltec-v3 platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", ".internal", "platformio.ini"), []byte("[env:internal]\n"), 0o644); err != nil {
		t.Fatalf("create .internal platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", "bad name", "platformio.ini"), []byte("[env:bad]\n"), 0o644); err != nil {
		t.Fatalf("create bad name platformio.ini: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "nrf52840", "tbeam", "platformio.ini"), []byte("[env:tbeam_nrf]\n"), 0o644); err != nil {
		t.Fatalf("create duplicate name platformio.ini: %v", err)
	}

	if err := os.WriteFile(filepath.Join(variantsDir, "README.md"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	got, err := listVariantDirectories(root)
	if err != nil {
		t.Fatalf("listVariantDirectories failed: %v", err)
	}

	want := []string{"heltec-v3", "t-deck", "t-deck-tft", "tbeam", "tbeam_nrf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected variants: got=%v want=%v", got, want)
	}
}

func TestFindVariantProjectPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	variantsDir := filepath.Join(root, "variants")
	if err := os.MkdirAll(filepath.Join(variantsDir, "esp32", "tbeam"), 0o755); err != nil {
		t.Fatalf("create esp32/tbeam: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(variantsDir, "nrf52840", "tbeam"), 0o755); err != nil {
		t.Fatalf("create nrf52840/tbeam: %v", err)
	}

	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", "tbeam", "platformio.ini"), []byte("[env:tbeam_esp32]\n"), 0o644); err != nil {
		t.Fatalf("create esp32 config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "nrf52840", "tbeam", "platformio.ini"), []byte("[env:tbeam_nrf]\n"), 0o644); err != nil {
		t.Fatalf("create nrf config: %v", err)
	}

	project, err := findVariantProject(root, "esp32/tbeam")
	if err != nil {
		t.Fatalf("findVariantProject failed: %v", err)
	}

	want := filepath.Join(variantsDir, "esp32", "tbeam")
	if project.AbsolutePath != want {
		t.Fatalf("unexpected project path: got=%q want=%q", project.AbsolutePath, want)
	}
	if project.EnvName != "tbeam_esp32" {
		t.Fatalf("unexpected env name: got=%q want=%q", project.EnvName, "tbeam_esp32")
	}
}

func TestFindVariantProjectByEnvTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	variantsDir := filepath.Join(root, "variants")
	if err := os.MkdirAll(filepath.Join(variantsDir, "esp32", "t-deck"), 0o755); err != nil {
		t.Fatalf("create esp32/t-deck: %v", err)
	}

	content := "[env:t-deck]\n[env:t-deck-tft]\n"
	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", "t-deck", "platformio.ini"), []byte(content), 0o644); err != nil {
		t.Fatalf("create t-deck config: %v", err)
	}

	project, err := findVariantProject(root, "t-deck-tft")
	if err != nil {
		t.Fatalf("findVariantProject failed: %v", err)
	}

	wantPath := filepath.Join(variantsDir, "esp32", "t-deck")
	if project.AbsolutePath != wantPath {
		t.Fatalf("unexpected project path: got=%q want=%q", project.AbsolutePath, wantPath)
	}
	if project.EnvName != "t-deck-tft" {
		t.Fatalf("unexpected env name: got=%q want=%q", project.EnvName, "t-deck-tft")
	}
}

func TestFindVariantProjectRejectsAmbiguousName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	variantsDir := filepath.Join(root, "variants")
	if err := os.MkdirAll(filepath.Join(variantsDir, "esp32", "diy"), 0o755); err != nil {
		t.Fatalf("create esp32/diy: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(variantsDir, "nrf52840", "diy"), 0o755); err != nil {
		t.Fatalf("create nrf52840/diy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "esp32", "diy", "platformio.ini"), []byte("[env:diy]\n"), 0o644); err != nil {
		t.Fatalf("create esp32 config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(variantsDir, "nrf52840", "diy", "platformio.ini"), []byte("[env:diy]\n"), 0o644); err != nil {
		t.Fatalf("create nrf config: %v", err)
	}

	_, err := findVariantProject(root, "diy")
	if err == nil {
		t.Fatalf("expected ambiguity error")
	}
}

func TestExtractPlatformIOEnvNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		expectedEnvs []string
	}{
		{
			name: "simple env name",
			content: `[env:tbeam]
board = esp32dev
framework = arduino
`,
			expectedEnvs: []string{"tbeam"},
		},
		{
			name: "env name with slash",
			content: `[env:esp32/tbeam]
board = esp32dev
framework = arduino
`,
			expectedEnvs: []string{"esp32/tbeam"},
		},
		{
			name: "env name with hyphen",
			content: `[env:esp32-tbeam]
board = esp32dev
framework = arduino
`,
			expectedEnvs: []string{"esp32-tbeam"},
		},
		{
			name: "multiple envs",
			content: `[env:tbeam]
board = esp32dev
framework = arduino

[env:nano]
board = nanoatmega328
framework = arduino
`,
			expectedEnvs: []string{"tbeam", "nano"},
		},
		{
			name: "no env section",
			content: `[platformio]
default_envs = tbeam
`,
			expectedEnvs: []string{},
		},
		{
			name: "env with whitespace",
			content: `[  env: tbeam-core  ]
board = esp32dev
framework = arduino
`,
			expectedEnvs: []string{"tbeam-core"},
		},
		{
			name: "env with trailing comment",
			content: `[env:t-deck-tft] ; display mode
board = esp32dev
framework = arduino
`,
			expectedEnvs: []string{"t-deck-tft"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			configPath := filepath.Join(root, "platformio.ini")

			if err := os.WriteFile(configPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write platformio.ini: %v", err)
			}

			envNames, err := extractPlatformIOEnvNames(root)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(envNames, tt.expectedEnvs) {
				t.Fatalf("env names mismatch: got=%v want=%v", envNames, tt.expectedEnvs)
			}
		})
	}
}

func TestExtractPlatformIOEnvConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := `[env]
build_flags = -DGLOBAL=1
  -Wall
lib_deps = SPI

[env:tbeam]
build_flags = -DTBEAM=1
lib_deps =
  bblanchon/ArduinoJson @ ^7

[env:heltec]
lib_deps =
  sandeepmistry/LoRa @ ^0.8.0
`
	if err := os.WriteFile(filepath.Join(root, "platformio.ini"), []byte(content), 0o644); err != nil {
		t.Fatalf("write platformio.ini: %v", err)
	}

	envNames, envOptions, err := extractPlatformIOEnvConfig(root)
	if err != nil {
		t.Fatalf("extractPlatformIOEnvConfig failed: %v", err)
	}

	if !reflect.DeepEqual(envNames, []string{"tbeam", "heltec"}) {
		t.Fatalf("unexpected env names: %v", envNames)
	}

	tbeamOptions := envOptions["tbeam"]
	if !reflect.DeepEqual(tbeamOptions.BuildFlags, []string{"-DGLOBAL=1", "-Wall", "-DTBEAM=1"}) {
		t.Fatalf("unexpected tbeam build flags: %v", tbeamOptions.BuildFlags)
	}
	if !reflect.DeepEqual(tbeamOptions.LibDeps, []string{"SPI", "bblanchon/ArduinoJson @ ^7"}) {
		t.Fatalf("unexpected tbeam lib deps: %v", tbeamOptions.LibDeps)
	}

	heltecOptions := envOptions["heltec"]
	if !reflect.DeepEqual(heltecOptions.BuildFlags, []string{"-DGLOBAL=1", "-Wall"}) {
		t.Fatalf("unexpected heltec build flags: %v", heltecOptions.BuildFlags)
	}
	if !reflect.DeepEqual(heltecOptions.LibDeps, []string{"SPI", "sandeepmistry/LoRa @ ^0.8.0"}) {
		t.Fatalf("unexpected heltec lib deps: %v", heltecOptions.LibDeps)
	}
}

func TestListVariantDevicesIncludesBuildOptions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	devicePath := filepath.Join(root, "variants", "esp32", "tbeam")
	if err := os.MkdirAll(devicePath, 0o755); err != nil {
		t.Fatalf("create device dir: %v", err)
	}

	content := `[env]
build_flags = -DGLOBAL=1

[env:tbeam]
build_flags = -DTBEAM=1
lib_deps =
  bblanchon/ArduinoJson @ ^7
`
	if err := os.WriteFile(filepath.Join(devicePath, "platformio.ini"), []byte(content), 0o644); err != nil {
		t.Fatalf("write platformio.ini: %v", err)
	}

	devices, err := listVariantDevices(root)
	if err != nil {
		t.Fatalf("listVariantDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("unexpected devices count: got=%d want=1", len(devices))
	}

	if devices[0].Name != "tbeam" {
		t.Fatalf("unexpected device name: %q", devices[0].Name)
	}
	if !reflect.DeepEqual(devices[0].BuildFlags, []string{"-DGLOBAL=1", "-DTBEAM=1"}) {
		t.Fatalf("unexpected build flags: %v", devices[0].BuildFlags)
	}
	if !reflect.DeepEqual(devices[0].LibDeps, []string{"bblanchon/ArduinoJson @ ^7"}) {
		t.Fatalf("unexpected lib deps: %v", devices[0].LibDeps)
	}
}
