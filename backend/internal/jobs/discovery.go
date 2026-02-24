package jobs

import (
	"context"
	"fmt"
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
	entries, err := os.ReadDir(variantsDir)
	if err != nil {
		return nil, fmt.Errorf("read variants directory: %w", err)
	}

	devices := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := strings.TrimSpace(entry.Name())
		if strings.HasPrefix(name, ".") {
			continue
		}
		if err := ValidateDevice(name); err != nil {
			continue
		}

		hasPlatformIOIni, err := hasVariantPlatformIOIni(variantsDir, name)
		if err != nil {
			return nil, err
		}
		if !hasPlatformIOIni {
			continue
		}

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

func hasVariantPlatformIOIni(variantsDir string, device string) (bool, error) {
	configPath := filepath.Join(variantsDir, device, "platformio.ini")
	info, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", configPath, err)
	}

	if info.IsDir() {
		return false, nil
	}

	return true, nil
}
