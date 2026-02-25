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

var (
	envSectionPattern = regexp.MustCompile(`^\[\s*env\s*:\s*([^\]]+?)\s*\]\s*(?:[;#].*)?$`)
	sectionPattern    = regexp.MustCompile(`^\[\s*([^\]]+?)\s*\]\s*(?:[;#].*)?$`)
)

const commonEnvSectionKey = "__common_env__"

type DiscoveredDevice struct {
	Name       string
	BuildFlags []string
	LibDeps    []string
}

type variantProject struct {
	Name         string
	RelativePath string
	AbsolutePath string
	EnvName      string
	EnvNames     []string
	EnvOptions   map[string]BuildOptions
}

func discoverDevices(ctx context.Context, discoveryRoot string, repoURL string, ref string) ([]DiscoveredDevice, error) {
	tempDir, err := os.MkdirTemp(discoveryRoot, "discover-*")
	if err != nil {
		return nil, fmt.Errorf("create discovery workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)

	repoPath := filepath.Join(tempDir, "repo")
	if err := cloneRepository(ctx, repoURL, ref, repoPath, nil); err != nil {
		return nil, err
	}

	devices, err := listVariantDevices(repoPath)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, fmt.Errorf("no final devices found in variants directory")
	}

	return devices, nil
}

func listVariantDirectories(repoPath string) ([]string, error) {
	devices, err := listVariantDevices(repoPath)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(devices))
	for _, device := range devices {
		names = append(names, device.Name)
	}
	return names, nil
}

func listVariantDevices(repoPath string) ([]DiscoveredDevice, error) {
	variantsDir := filepath.Join(repoPath, "variants")
	entries, err := collectVariantProjects(variantsDir)
	if err != nil {
		return nil, err
	}

	devices := make([]DiscoveredDevice, 0, len(entries)*2)
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

			options := entry.EnvOptions[target]
			devices = append(devices, DiscoveredDevice{
				Name:       target,
				BuildFlags: append([]string(nil), options.BuildFlags...),
				LibDeps:    append([]string(nil), options.LibDeps...),
			})
		}
	}

	sort.Slice(devices, func(i int, j int) bool {
		return devices[i].Name < devices[j].Name
	})

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

		envNames, envOptions, err := extractPlatformIOEnvConfig(path)
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
			EnvOptions:   envOptions,
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
	envNames, _, err := extractPlatformIOEnvConfig(devicePath)
	if err != nil {
		return nil, err
	}
	return envNames, nil
}

func extractPlatformIOEnvConfig(devicePath string) ([]string, map[string]BuildOptions, error) {
	configPath := filepath.Join(devicePath, "platformio.ini")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read platformio.ini: %w", err)
	}

	envNames := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	allOptions := make(map[string]BuildOptions, 4)

	currentEnv := ""
	currentOption := ""
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			currentOption = ""
			continue
		}
		if strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		sectionMatch := sectionPattern.FindStringSubmatch(trimmed)
		if len(sectionMatch) == 2 {
			sectionName := strings.TrimSpace(sectionMatch[1])
			currentOption = ""

			if strings.EqualFold(sectionName, "env") {
				currentEnv = commonEnvSectionKey
				if _, exists := allOptions[currentEnv]; !exists {
					allOptions[currentEnv] = BuildOptions{}
				}
				continue
			}

			envMatch := envSectionPattern.FindStringSubmatch(trimmed)
			if len(envMatch) != 2 {
				currentEnv = ""
				continue
			}

			envName := strings.TrimSpace(envMatch[1])
			if envName == "" {
				currentEnv = ""
				continue
			}
			if err := ValidateDevice(envName); err != nil {
				currentEnv = ""
				continue
			}

			currentEnv = envName
			if _, exists := allOptions[currentEnv]; !exists {
				allOptions[currentEnv] = BuildOptions{}
			}
			if _, exists := seen[envName]; !exists {
				seen[envName] = struct{}{}
				envNames = append(envNames, envName)
			}
			continue
		}

		if currentEnv == "" {
			currentOption = ""
			continue
		}

		isContinuation := (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && currentOption != ""
		if isContinuation {
			value := parseOptionValue(trimmed)
			if value != "" {
				appendBuildOptionValue(allOptions, currentEnv, currentOption, value)
			}
			continue
		}

		key, value, ok := splitIniOption(trimmed)
		if !ok {
			currentOption = ""
			continue
		}

		normalizedKey := strings.ToLower(key)
		switch normalizedKey {
		case "build_flags", "lib_deps":
			currentOption = normalizedKey
			parsedValue := parseOptionValue(value)
			if parsedValue != "" {
				appendBuildOptionValue(allOptions, currentEnv, currentOption, parsedValue)
			}
		default:
			currentOption = ""
		}
	}

	resolvedOptions := make(map[string]BuildOptions, len(envNames))
	commonOptions := allOptions[commonEnvSectionKey]
	for _, envName := range envNames {
		envOptions := allOptions[envName]
		resolvedOptions[envName] = BuildOptions{
			BuildFlags: append(append([]string(nil), commonOptions.BuildFlags...), envOptions.BuildFlags...),
			LibDeps:    append(append([]string(nil), commonOptions.LibDeps...), envOptions.LibDeps...),
		}
	}

	return envNames, resolvedOptions, nil
}

func splitIniOption(line string) (string, string, bool) {
	index := strings.Index(line, "=")
	if index < 0 {
		return "", "", false
	}

	key := strings.TrimSpace(line[:index])
	if key == "" {
		return "", "", false
	}

	value := strings.TrimSpace(line[index+1:])
	return key, value, true
}

func parseOptionValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	for index := 0; index < len(trimmed); index++ {
		char := trimmed[index]
		if char != ';' && char != '#' {
			continue
		}
		if index == 0 {
			return ""
		}
		prev := trimmed[index-1]
		if prev != ' ' && prev != '\t' {
			continue
		}

		return strings.TrimSpace(trimmed[:index])
	}

	return trimmed
}

func appendBuildOptionValue(options map[string]BuildOptions, envName string, option string, value string) {
	entry := options[envName]
	switch option {
	case "build_flags":
		entry.BuildFlags = append(entry.BuildFlags, value)
	case "lib_deps":
		entry.LibDeps = append(entry.LibDeps, value)
	}
	options[envName] = entry
}
