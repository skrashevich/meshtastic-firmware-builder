package jobs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func discoverDevices(ctx context.Context, discoveryRoot string, repoURL string, ref string) ([]string, error) {
	tempDir, err := os.MkdirTemp(discoveryRoot, "discover-*")
	if err != nil {
		return nil, fmt.Errorf("create discovery workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	repoPath := filepath.Join(tempDir, "repo")
	if err := cloneRepository(ctx, repoURL, ref, repoPath, nil); err != nil {
		return nil, err
	}

	devices, err := listVariantDirectories(repoPath)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("no final devices found in variants directory")
	}

	return devices, nil
}

func listVariantDirectories(repoPath string) ([]string, error) {
	variantsDir := filepath.Join(repoPath, "variants")
	entries, err := collectVariantEntries(variantsDir)
	if err != nil {
		return nil, err
	}

	devices := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := strings.TrimSpace(entry)
		if name == "" {
			continue
		}
		if err := ValidateDevice(name); err != nil {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		devices = append(devices, name)
	}

	sort.Strings(devices)
	return devices, nil
}

func variantExists(repoPath string, device string) (bool, error) {
	devices, err := listVariantDirectories(repoPath)
	if err != nil {
		return false, err
	}
	for _, value := range devices {
		if value == device {
			return true, nil
		}
	}
	return false, nil
}

func collectVariantEntries(variantsDir string) ([]string, error) {
	entries := make([]string, 0, 128)

	err := filepath.WalkDir(variantsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			if path == variantsDir {
				return nil
			}
			return filepath.SkipDir
		}

		if path == variantsDir {
			return nil
		}

		hasConfig, err := hasPlatformIOIni(path)
		if err != nil {
			return err
		}
		if !hasConfig {
			return nil
		}

		entries = append(entries, name)
		return filepath.SkipDir
	})
	if err != nil {
		return nil, fmt.Errorf("read variants directory: %w", err)
	}

	return entries, nil
}

func hasPlatformIOIni(devicePath string) (bool, error) {
	configPath := filepath.Join(devicePath, "platformio.ini")
	info, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", configPath, err)
	}
	return !info.IsDir(), nil
}
