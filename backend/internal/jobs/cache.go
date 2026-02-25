package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	firmwareCacheKeyVersion      = 1
	firmwareCacheManifestVersion = 1
	firmwareCacheFilesDirName    = "files"
	firmwareCacheManifestName    = "manifest.json"
)

type firmwareCacheKeyInput struct {
	Version    int      `json:"version"`
	RepoURL    string   `json:"repoUrl"`
	Commit     string   `json:"commit"`
	EnvName    string   `json:"envName"`
	BuildFlags []string `json:"buildFlags,omitempty"`
	LibDeps    []string `json:"libDeps,omitempty"`
}

type firmwareCacheManifest struct {
	Version   int                     `json:"version"`
	CreatedAt time.Time               `json:"createdAt"`
	Artifacts []firmwareCacheArtifact `json:"artifacts"`
}

type firmwareCacheArtifact struct {
	Name         string `json:"name"`
	RelativePath string `json:"relativePath"`
	Size         int64  `json:"size"`
}

func buildFirmwareCacheKey(repoURL string, commit string, envName string, options BuildOptions) (string, error) {
	input := firmwareCacheKeyInput{
		Version:    firmwareCacheKeyVersion,
		RepoURL:    strings.TrimSpace(repoURL),
		Commit:     strings.ToLower(strings.TrimSpace(commit)),
		EnvName:    strings.TrimSpace(envName),
		BuildFlags: append([]string(nil), options.BuildFlags...),
		LibDeps:    append([]string(nil), options.LibDeps...),
	}

	if input.RepoURL == "" {
		return "", errors.New("cache key requires repo URL")
	}
	if input.Commit == "" {
		return "", errors.New("cache key requires commit")
	}
	if input.EnvName == "" {
		return "", errors.New("cache key requires environment name")
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshal cache key payload: %w", err)
	}

	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func loadArtifactsFromFirmwareCache(cacheRootPath string, cacheKey string) ([]Artifact, bool, error) {
	cacheDir, err := firmwareCacheDirPath(cacheRootPath, cacheKey)
	if err != nil {
		return nil, false, err
	}

	manifestPath := filepath.Join(cacheDir, firmwareCacheManifestName)
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read cache manifest: %w", err)
	}

	var manifest firmwareCacheManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, false, fmt.Errorf("decode cache manifest: %w", err)
	}
	if manifest.Version != firmwareCacheManifestVersion {
		return nil, false, fmt.Errorf("unsupported cache manifest version: %d", manifest.Version)
	}
	if len(manifest.Artifacts) == 0 {
		return nil, false, errors.New("cache manifest contains no artifacts")
	}

	artifacts := make([]Artifact, 0, len(manifest.Artifacts))
	for _, item := range manifest.Artifacts {
		path, err := firmwareCacheArtifactPath(cacheDir, item.RelativePath)
		if err != nil {
			return nil, false, err
		}

		info, err := os.Stat(path)
		if err != nil {
			return nil, false, fmt.Errorf("read cached artifact %q: %w", item.RelativePath, err)
		}
		if info.IsDir() {
			return nil, false, fmt.Errorf("cached artifact %q is a directory", item.RelativePath)
		}
		if item.Size > 0 && info.Size() != item.Size {
			return nil, false, fmt.Errorf("cached artifact %q size mismatch", item.RelativePath)
		}

		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = filepath.Base(path)
		}

		artifacts = append(artifacts, Artifact{
			Name:         name,
			RelativePath: item.RelativePath,
			Size:         info.Size(),
			absPath:      path,
		})
	}

	assignArtifactIDs(artifacts)
	return artifacts, true, nil
}

func storeArtifactsInFirmwareCache(cacheRootPath string, cacheKey string, artifacts []Artifact) error {
	if len(artifacts) == 0 {
		return errors.New("no artifacts to store in firmware cache")
	}

	cacheDir, err := firmwareCacheDirPath(cacheRootPath, cacheKey)
	if err != nil {
		return err
	}

	if _, err := os.Stat(cacheDir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check cache destination: %w", err)
	}

	tempDir, err := os.MkdirTemp(cacheRootPath, "firmware-cache-*")
	if err != nil {
		return fmt.Errorf("create temporary cache directory: %w", err)
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.RemoveAll(tempDir)
		}
	}()

	manifest := firmwareCacheManifest{
		Version:   firmwareCacheManifestVersion,
		CreatedAt: time.Now().UTC(),
		Artifacts: make([]firmwareCacheArtifact, 0, len(artifacts)),
	}

	for _, artifact := range artifacts {
		sourcePath := strings.TrimSpace(artifact.AbsolutePath())
		if sourcePath == "" {
			return fmt.Errorf("artifact %q has empty source path", artifact.RelativePath)
		}

		destinationPath, err := firmwareCacheArtifactPath(tempDir, artifact.RelativePath)
		if err != nil {
			return err
		}
		if err := copyFile(sourcePath, destinationPath); err != nil {
			return fmt.Errorf("store cached artifact %q: %w", artifact.RelativePath, err)
		}

		info, err := os.Stat(destinationPath)
		if err != nil {
			return fmt.Errorf("read cached artifact %q: %w", artifact.RelativePath, err)
		}

		manifest.Artifacts = append(manifest.Artifacts, firmwareCacheArtifact{
			Name:         artifact.Name,
			RelativePath: artifact.RelativePath,
			Size:         info.Size(),
		})
	}

	sort.Slice(manifest.Artifacts, func(i int, j int) bool {
		return manifest.Artifacts[i].RelativePath < manifest.Artifacts[j].RelativePath
	})

	manifestContent, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode cache manifest: %w", err)
	}

	manifestPath := filepath.Join(tempDir, firmwareCacheManifestName)
	if err := os.WriteFile(manifestPath, manifestContent, 0o644); err != nil {
		return fmt.Errorf("write cache manifest: %w", err)
	}

	if err := os.Rename(tempDir, cacheDir); err != nil {
		if _, statErr := os.Stat(cacheDir); statErr == nil {
			return nil
		}
		return fmt.Errorf("activate firmware cache entry: %w", err)
	}

	cleanupTemp = false
	return nil
}

func firmwareCacheDirPath(cacheRootPath string, cacheKey string) (string, error) {
	root := strings.TrimSpace(cacheRootPath)
	if root == "" {
		return "", errors.New("firmware cache root path is required")
	}

	key := strings.TrimSpace(cacheKey)
	if len(key) != 64 || !isLowerHexString(key) {
		return "", errors.New("invalid firmware cache key")
	}

	return filepath.Join(root, key), nil
}

func firmwareCacheArtifactPath(cacheDir string, relativePath string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relativePath)))
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "", errors.New("invalid artifact relative path")
	}
	if filepath.IsAbs(cleaned) {
		return "", errors.New("artifact relative path must not be absolute")
	}
	if strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("artifact relative path escapes cache directory")
	}

	return filepath.Join(cacheDir, firmwareCacheFilesDirName, cleaned), nil
}

func copyFile(sourcePath string, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}

	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	return destination.Sync()
}

func shortCommit(commit string) string {
	trimmed := strings.TrimSpace(commit)
	if len(trimmed) <= 12 {
		return trimmed
	}
	return trimmed[:12]
}

func isLowerHexString(value string) bool {
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
