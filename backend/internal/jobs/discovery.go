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

type variantProject struct {
	Name         string
	RelativePath string
	AbsolutePath string
	EnvName      string // PlatformIO environment name from platformio.ini
}

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
	entries, err := collectVariantProjects(variantsDir)
	if err != nil {
		return nil, err
	}

	devices := make([]string, 0, len(entries))
	for _, entry := range entries {
		relative := strings.TrimSpace(entry.RelativePath)
		if relative == "" {
			continue
		}
		if err := ValidateDeviceSelection(relative); err != nil {
			continue
		}
		devices = append(devices, relative)
	}

	sort.Strings(devices)
	return devices, nil
}

func findVariantProject(repoPath string, selection string) (variantProject, error) {
	if err := ValidateDeviceSelection(selection); err != nil {
		return variantProject{}, err
	}

	variantsDir := filepath.Join(repoPath, "variants")
	entries, err := collectVariantProjects(variantsDir)
	if err != nil {
		return variantProject{}, err
	}

	normalizedSelection := filepath.ToSlash(strings.TrimSpace(selection))
	if strings.Contains(normalizedSelection, "/") {
		for _, entry := range entries {
			if entry.RelativePath == normalizedSelection {
				return entry, nil
			}
		}
		return variantProject{}, nil
	}

	matches := make([]variantProject, 0, 4)
	for _, entry := range entries {
		if entry.Name == normalizedSelection {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 0 {
		return variantProject{}, nil
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.RelativePath)
		}
		return variantProject{}, fmt.Errorf("device %q is ambiguous, choose one of: %s", normalizedSelection, strings.Join(paths, ", "))
	}

	return matches[0], nil
}

func collectVariantProjects(variantsDir string) ([]variantProject, error) {
	entries := make([]variantProject, 0, 128)

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

		relPath, err := filepath.Rel(variantsDir, path)
		if err != nil {
			return err
		}

		// Extract PlatformIO environment name from platformio.ini
		envName, err := extractPlatformIOEnvName(path)
		if err != nil {
			return err
		}

		entries = append(entries, variantProject{
			Name:         name,
			RelativePath: filepath.ToSlash(relPath),
			AbsolutePath: path,
			EnvName:      envName,
		})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, fmt.Errorf("read variants directory: %w", err)
	}

	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].RelativePath < entries[j].RelativePath
	})

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

func extractPlatformIOEnvName(devicePath string) (string, error) {
	configPath := filepath.Join(devicePath, "platformio.ini")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read platformio.ini: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		// Trim leading/trailing whitespace
		line = strings.TrimSpace(line)
		// Remove all spaces from the line to handle [ env: name ] cases
		// PlatformIO environment names don't contain spaces
		line = strings.ReplaceAll(line, " ", "")

		// Match [env:name] format
		if strings.HasPrefix(line, "[env:") && strings.HasSuffix(line, "]") {
			// Extract content between brackets: [env:name] -> env:name
			envSection := strings.TrimPrefix(line, "[")
			envSection = strings.TrimSuffix(envSection, "]")
			// Extract environment name: env:name -> name
			if strings.HasPrefix(envSection, "env:") {
				envName := strings.TrimPrefix(envSection, "env:")
				if envName != "" {
					return envName, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no [env:*] section found in platformio.ini")
}
