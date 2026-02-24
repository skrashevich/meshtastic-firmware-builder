package jobs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

func collectArtifacts(repoPath string, device string) ([]Artifact, error) {
	buildRoot := filepath.Join(repoPath, ".pio", "build", device)
	info, err := os.Stat(buildRoot)
	if err != nil {
		return nil, fmt.Errorf("read build output directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("build output path is not a directory")
	}

	artifacts := make([]Artifact, 0, 32)
	walkErr := filepath.WalkDir(buildRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}

		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(buildRoot, path)
		if err != nil {
			return err
		}

		artifacts = append(artifacts, Artifact{
			Name:         filepath.Base(path),
			RelativePath: filepath.ToSlash(relPath),
			Size:         fileInfo.Size(),
			absPath:      path,
		})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan build artifacts: %w", walkErr)
	}

	if len(artifacts) == 0 {
		return nil, fmt.Errorf("no artifacts found in build output")
	}

	sort.Slice(artifacts, func(i int, j int) bool {
		return artifacts[i].RelativePath < artifacts[j].RelativePath
	})

	for index := range artifacts {
		artifacts[index].ID = strconv.Itoa(index + 1)
	}

	return artifacts, nil
}
