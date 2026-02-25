package jobs

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	scpLikeRepoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+:[A-Za-z0-9._/-]+(\.git)?$`)
	devicePattern      = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	devicePathPattern  = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	refPattern         = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,128}$`)
)

const (
	maxBuildOptionItems  = 128
	maxBuildOptionLength = 512
)

func ValidateRepoURL(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("repoUrl is required")
	}

	if hasWhitespace(value) {
		return errors.New("repoUrl must not contain whitespace")
	}

	if scpLikeRepoPattern.MatchString(value) {
		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("repoUrl is invalid: %w", err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("repoUrl must include scheme and host")
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ssh", "git":
	default:
		return errors.New("unsupported repository scheme")
	}

	if strings.Contains(parsed.Path, "..") {
		return errors.New("repoUrl path is invalid")
	}

	return nil
}

func ValidateRef(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if !refPattern.MatchString(value) {
		return errors.New("ref contains unsupported characters")
	}
	return nil
}

func ValidateDevice(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("device is required")
	}
	if !devicePattern.MatchString(value) {
		return errors.New("device contains unsupported characters")
	}
	// Prevent path traversal attacks
	if strings.Contains(value, "..") {
		return errors.New("device contains invalid path traversal")
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return errors.New("device contains invalid path separators")
	}
	return nil
}

func ValidateDeviceSelection(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("device is required")
	}
	if !devicePathPattern.MatchString(value) {
		return errors.New("device contains unsupported characters")
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return errors.New("device path is invalid")
	}
	if strings.Contains(value, "//") || strings.Contains(value, "..") {
		return errors.New("device path is invalid")
	}
	return nil
}

func NormalizeBuildOptions(raw BuildOptions) (BuildOptions, error) {
	buildFlags, err := normalizeBuildOptionValues("buildFlags", raw.BuildFlags)
	if err != nil {
		return BuildOptions{}, err
	}

	libDeps, err := normalizeBuildOptionValues("libDeps", raw.LibDeps)
	if err != nil {
		return BuildOptions{}, err
	}

	for _, flag := range buildFlags {
		if strings.HasPrefix(flag, "!") {
			return BuildOptions{}, errors.New("buildFlags values must not start with '!' command syntax")
		}
	}

	return BuildOptions{
		BuildFlags: buildFlags,
		LibDeps:    libDeps,
	}, nil
}

func normalizeBuildOptionValues(field string, items []string) ([]string, error) {
	if len(items) > maxBuildOptionItems {
		return nil, fmt.Errorf("%s supports up to %d entries", field, maxBuildOptionItems)
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if len(value) > maxBuildOptionLength {
			return nil, fmt.Errorf("%s entry exceeds %d characters", field, maxBuildOptionLength)
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("%s entries must be single-line values", field)
		}
		if hasControlChars(value) {
			return nil, fmt.Errorf("%s entries contain unsupported control characters", field)
		}
		result = append(result, value)
	}

	if len(result) > maxBuildOptionItems {
		return nil, fmt.Errorf("%s supports up to %d entries", field, maxBuildOptionItems)
	}

	return result, nil
}

func hasWhitespace(value string) bool {
	for _, char := range value {
		if char == ' ' || char == '\t' || char == '\n' || char == '\r' {
			return true
		}
	}
	return false
}

func hasControlChars(value string) bool {
	for _, char := range value {
		if char < 32 && char != '\t' {
			return true
		}
	}
	return false
}
