package jobs

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/buildlogs"
)

// MigrateFirmwareCacheMetadata backfills repoUrl, ref, and device into
// existing cache manifests by matching them against build logs.
// It matches by comparing artifact file names and creation timestamps.
// This is a one-time best-effort migration; unmatched entries are left as-is.
func MigrateFirmwareCacheMetadata(cacheRootPath string, buildLogsStore *buildlogs.Store, logger *log.Logger) {
	root := strings.TrimSpace(cacheRootPath)
	if root == "" {
		logger.Printf("cache migration: skipped, no cache root path configured")
		return
	}

	dirEntries, err := os.ReadDir(root)
	if err != nil {
		logger.Printf("cache migration: cannot read cache dir %s: %v", root, err)
		return
	}

	// Collect cache entries that need migration.
	type pendingEntry struct {
		dirName      string
		manifest     firmwareCacheManifest
		manifestPath string
	}
	var pending []pendingEntry
	alreadyOk := 0

	for _, de := range dirEntries {
		if !de.IsDir() || len(de.Name()) != 64 || !isLowerHexString(de.Name()) {
			continue
		}

		manifestPath := filepath.Join(root, de.Name(), firmwareCacheManifestName)
		content, err := os.ReadFile(manifestPath)
		if err != nil {
			logger.Printf("cache migration: cannot read manifest %s: %v", de.Name()[:12], err)
			continue
		}

		var manifest firmwareCacheManifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			logger.Printf("cache migration: cannot parse manifest %s: %v", de.Name()[:12], err)
			continue
		}

		// Skip entries that already have metadata.
		if manifest.RepoURL != "" {
			alreadyOk++
			continue
		}

		pending = append(pending, pendingEntry{
			dirName:      de.Name(),
			manifest:     manifest,
			manifestPath: manifestPath,
		})
	}

	logger.Printf("cache migration: found %d entries needing migration, %d already have metadata", len(pending), alreadyOk)

	if len(pending) == 0 {
		return
	}

	// Load all build logs (successful ones).
	allLogs, err := buildLogsStore.List(0)
	if err != nil {
		logger.Printf("cache migration: cannot load build logs: %v", err)
		return
	}
	if len(allLogs) == 0 {
		logger.Printf("cache migration: no build logs found, cannot match")
		return
	}

	logger.Printf("cache migration: loaded %d build log entries", len(allLogs))

	// Build an index of successful logs.
	type logInfo struct {
		jobID     string
		repoURL   string
		ref       string
		device    string
		createdAt time.Time
	}

	var logIndex []logInfo
	for _, entry := range allLogs {
		if entry.Status != "success" {
			continue
		}

		logIndex = append(logIndex, logInfo{
			jobID:     entry.JobID,
			repoURL:   entry.RepoURL,
			ref:       entry.Ref,
			device:    entry.Device,
			createdAt: entry.CreatedAt,
		})
	}

	logger.Printf("cache migration: %d successful build logs available for matching", len(logIndex))

	migrated := 0
	unmatched := 0
	for _, pe := range pending {
		// Try to match by device name extracted from artifact file names.
		// Firmware files are named like: firmware-<device>-<version>.bin
		deviceFromArtifacts := extractDeviceFromArtifacts(pe.manifest.Artifacts)

		artifactNames := make([]string, 0, len(pe.manifest.Artifacts))
		for _, a := range pe.manifest.Artifacts {
			artifactNames = append(artifactNames, a.Name)
		}

		logger.Printf("cache migration: processing %s (created %s, %d artifacts, extracted device=%q, files: %s)",
			pe.dirName[:12],
			pe.manifest.CreatedAt.Format("2006-01-02 15:04:05"),
			len(pe.manifest.Artifacts),
			deviceFromArtifacts,
			strings.Join(artifactNames, ", "),
		)

		var bestMatch *logInfo
		bestTimeDiff := time.Duration(1<<63 - 1)

		for i := range logIndex {
			li := &logIndex[i]

			// Match by device name if we could extract it.
			if deviceFromArtifacts != "" {
				if !strings.EqualFold(li.device, deviceFromArtifacts) {
					continue
				}
			}

			// Match by timestamp proximity (within 10 minutes).
			diff := pe.manifest.CreatedAt.Sub(li.createdAt)
			if diff < 0 {
				diff = -diff
			}
			if diff > 10*time.Minute {
				continue
			}

			if diff < bestTimeDiff {
				bestTimeDiff = diff
				bestMatch = li
			}
		}

		if bestMatch == nil {
			logger.Printf("cache migration: %s — no matching build log found", pe.dirName[:12])
			unmatched++
			continue
		}

		logger.Printf("cache migration: %s — matched to job %s (device=%s, repo=%s, ref=%s, time diff=%s)",
			pe.dirName[:12],
			bestMatch.jobID[:12],
			bestMatch.device,
			bestMatch.repoURL,
			bestMatch.ref,
			bestTimeDiff,
		)

		pe.manifest.RepoURL = bestMatch.repoURL
		pe.manifest.Ref = bestMatch.ref
		pe.manifest.Device = bestMatch.device

		data, err := json.Marshal(pe.manifest)
		if err != nil {
			logger.Printf("cache migration: %s — marshal error: %v", pe.dirName[:12], err)
			continue
		}
		if err := os.WriteFile(pe.manifestPath, data, 0o644); err != nil {
			logger.Printf("cache migration: %s — write error: %v", pe.dirName[:12], err)
			continue
		}
		migrated++
	}

	logger.Printf("cache migration: done — migrated %d, unmatched %d, total pending %d", migrated, unmatched, len(pending))
}

// extractDeviceFromArtifacts tries to extract a device name from firmware
// artifact file names. Firmware files follow the pattern:
// firmware-<device>-<version>.bin or firmware-<device>-<version>.elf
func extractDeviceFromArtifacts(artifacts []firmwareCacheArtifact) string {
	// Sort to prefer longer names (more specific).
	names := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		names = append(names, a.Name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })

	for _, name := range names {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "firmware-") {
			continue
		}
		// Skip generic names like "firmware.bin", "firmware.elf".
		base := strings.TrimPrefix(lower, "firmware-")
		if base == "" {
			continue
		}

		// Find where version starts: look for pattern like -X.Y.Z or -<hexcommit>
		// Device name is between "firmware-" and the version/commit part.
		// Examples:
		//   firmware-heltec-wireless-paper-2.7.21.3fcbfe4.bin → heltec-wireless-paper
		//   firmware-t-echo-2.7.20.b941dab.elf → t-echo
		//   firmware-mfb-custom-e57fc28764c0c40f-2.7.20.016e68e.bin → mfb-custom-e57fc28764c0c40f

		// Strategy: split by '-', find the first segment that looks like a version number.
		parts := strings.Split(base, "-")
		versionIdx := -1
		for i, p := range parts {
			if len(p) > 0 && p[0] >= '0' && p[0] <= '9' {
				// Check if it looks like a version: starts with digit and contains a dot.
				if strings.Contains(p, ".") {
					versionIdx = i
					break
				}
			}
		}

		if versionIdx > 0 {
			device := strings.Join(parts[:versionIdx], "-")
			return device
		}
	}

	return ""
}
