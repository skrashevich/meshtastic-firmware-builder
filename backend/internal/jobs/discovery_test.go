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

	want := []string{"esp32/heltec-v3", "esp32/tbeam", "nrf52840/tbeam"}
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

func TestExtractPlatformIOEnvName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		expectedEnv string
		expectError bool
	}{
		{
			name: "simple env name",
			content: `[env:tbeam]
board = esp32dev
framework = arduino
`,
			expectedEnv: "tbeam",
			expectError: false,
		},
		{
			name: "env name with slash",
			content: `[env:esp32/tbeam]
board = esp32dev
framework = arduino
`,
			expectedEnv: "esp32/tbeam",
			expectError: false,
		},
		{
			name: "env name with hyphen",
			content: `[env:esp32-tbeam]
board = esp32dev
framework = arduino
`,
			expectedEnv: "esp32-tbeam",
			expectError: false,
		},
		{
			name: "multiple envs - first one",
			content: `[env:tbeam]
board = esp32dev
framework = arduino

[env:nano]
board = nanoatmega328
framework = arduino
`,
			expectedEnv: "tbeam",
			expectError: false,
		},
		{
			name: "no env section",
			content: `[platformio]
default_envs = tbeam
`,
			expectError: true,
		},
		{
			name: "env with whitespace",
			content: `[  env: tbeam-core  ]
board = esp32dev
framework = arduino
`,
			expectedEnv: "tbeam-core",
			expectError: false,
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

			envName, err := extractPlatformIOEnvName(root)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got env name: %q", envName)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if envName != tt.expectedEnv {
				t.Fatalf("env name mismatch: got=%q want=%q", envName, tt.expectedEnv)
			}
		})
	}
}
