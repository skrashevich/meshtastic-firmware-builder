package jobs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var envSectionPattern = regexp.MustCompile(`^\[\s*env\s*:\s*([^\]]+?)\s*\]\s*(?:[;#].*)?$`)

type variantProject struct {
	Name         string
	RelativePath string
	AbsolutePath string
	EnvName      string
	EnvNames     []string
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

	devices := make([]string, 0, len(entries)*2)
	seen := make(map[string]struct{}, len(entries)*2)
	for _, entry := range entries {
		for _, envName := range entry.EnvNames {
			target := strings.TrimSpace(envName)
			if target == "" {
				continue
			}
			if err := ValidateDeviceSelection(target); err != nil {
				continue
			}
			if _, exists := seen[target]; exists {
				continue
			}
			seen[target] = struct{}{}
			devices = append(devices, target)
		}
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

	for _, entry := range entries {
		if entry.RelativePath == normalizedSelection {
			return resolveEntryEnvironment(entry)
		}
	}

	envMatches := make([]variantProject, 0, 4)
	for _, entry := range entries {
		for _, envName := range entry.EnvNames {
			if envName == normalizedSelection {
				match := entry
				match.EnvName = envName
				envMatches = append(envMatches, match)
				break
			}
		}
	}
	if len(envMatches) == 1 {
		return envMatches[0], nil
	}
	if len(envMatches) > 1 {
		options := make([]string, 0, len(envMatches))
		for _, match := range envMatches {
			options = append(options, fmt.Sprintf("%s (%s)", match.RelativePath, match.EnvName))
		}
		sort.Strings(options)
		return variantProject{}, fmt.Errorf("target %q is ambiguous, choose one of: %s", normalizedSelection, strings.Join(options, ", "))
	}

	nameMatches := make([]variantProject, 0, 4)
	for _, entry := range entries {
		if entry.Name == normalizedSelection {
			nameMatches = append(nameMatches, entry)
		}
	}
	if len(nameMatches) == 0 {
		return variantProject{}, nil
	}
	if len(nameMatches) > 1 {
		paths := make([]string, 0, len(nameMatches))
		for _, match := range nameMatches {
			paths = append(paths, match.RelativePath)
		}
		return variantProject{}, fmt.Errorf("device %q is ambiguous, choose one of: %s", normalizedSelection, strings.Join(paths, ", "))
	}

	return resolveEntryEnvironment(nameMatches[0])
}

func resolveEntryEnvironment(entry variantProject) (variantProject, error) {
	if len(entry.EnvNames) == 0 {
		return variantProject{}, fmt.Errorf("device %q has no [env:*] targets in platformio.ini", entry.RelativePath)
	}
	if len(entry.EnvNames) == 1 {
		entry.EnvName = entry.EnvNames[0]
		return entry, nil
	}

	preferred := entry.Name
	for _, envName := range entry.EnvNames {
		if envName == preferred {
			entry.EnvName = envName
			return entry, nil
		}
	}

	return variantProject{}, fmt.Errorf("device %q has multiple build targets, choose one of: %s", entry.RelativePath, strings.Join(entry.EnvNames, ", "))
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
		relPath = filepath.ToSlash(relPath)
		if err := ValidateDeviceSelection(relPath); err != nil {
			return nil
		}

		envNames, err := extractPlatformIOEnvNames(path)
		if err != nil {
			return err
		}
		if len(envNames) == 0 {
			return nil
		}

		entries = append(entries, variantProject{
			Name:         name,
			RelativePath: relPath,
			AbsolutePath: path,
			EnvNames:     envNames,
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

func extractPlatformIOEnvNames(devicePath string) ([]string, error) {
	configPath := filepath.Join(devicePath, "platformio.ini")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read platformio.ini: %w", err)
	}

	envNames := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		match := envSectionPattern.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}

		envName := strings.TrimSpace(match[1])
		if envName == "" {
			continue
		}
		if err := ValidateDevice(envName); err != nil {
			continue
		}
		if _, exists := seen[envName]; exists {
			continue
		}

		seen[envName] = struct{}{}
		envNames = append(envNames, envName)
	}

	return envNames, nil
}
