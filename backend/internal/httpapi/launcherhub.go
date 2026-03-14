package httpapi

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/jobs"
	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/stats"
)

// LauncherHub-compatible API endpoints.
// Serves firmware from the existing build cache to enable OTA updates
// via Meshtastic Flasher and similar tools.

// --- JSON response types ---

type lhFirmwareListResponse struct {
	Total    int                  `json:"total"`
	PageSize int                  `json:"page_size"`
	Page     int                  `json:"page"`
	Items    []lhFirmwareListItem `json:"items"`
}

type lhFirmwareListItem struct {
	FID    string `json:"fid"`
	Name   string `json:"name"`
	Author string `json:"author"`
	Star   bool   `json:"star"`
}

type lhFirmwareDetailResponse struct {
	FID      string            `json:"fid"`
	Name     string            `json:"name"`
	Author   string            `json:"author"`
	Star     bool              `json:"star"`
	Versions []lhVersionDetail `json:"versions"`
}

type lhVersionDetail struct {
	Version     string `json:"version"`
	PublishedAt string `json:"published_at"`
	File        string `json:"file"`
	ZipURL      string `json:"zip_url"`
	FileSize    int64  `json:"Fs"`
	NoBoot      bool   `json:"nb"`
	HasSPIFFS   bool   `json:"s"`
	HasFAT1     bool   `json:"f"`
	HasFAT2     bool   `json:"f2"`
	AppSize     int64  `json:"as"`
	AppOffset   int64  `json:"ao"`
	SpiffsSize  int64  `json:"ss"`
	SpiffsOff   int64  `json:"so"`
	FatSize     int64  `json:"fs"`
	FatOff      int64  `json:"fo"`
	Fat2Size    int64  `json:"fs2"`
	Fat2Off     int64  `json:"fo2"`
}

// --- handlers ---

func (s *Server) handleLauncherHubFirmwares(w http.ResponseWriter, r *http.Request, requestID string) {
	fid := r.URL.Query().Get("fid")
	if fid != "" {
		s.handleLauncherHubFirmwareDetail(w, r, requestID, fid)
		return
	}

	category := r.URL.Query().Get("category")
	if category == "" {
		s.lhError(w, http.StatusBadRequest, "'category' query param is required")
		return
	}

	s.handleLauncherHubFirmwareList(w, r, requestID, category)
}

func (s *Server) handleLauncherHubFirmwareList(w http.ResponseWriter, r *http.Request, requestID string, category string) {
	cacheInfo := jobs.ScanFirmwareCache(s.cfg.FirmwareCachePath)

	// Build index: group cache entries by device+version+source.
	type entryKey struct {
		device  string
		version string
		source  string
	}
	entries := make(map[entryKey]jobs.FirmwareCacheEntry)

	for _, ce := range cacheInfo.Entries {
		if ce.Device == "" {
			continue
		}
		version := extractVersionFromCacheEntry(ce)
		if version == "" {
			continue
		}
		source := sourceFromRepoURL(ce.RepoURL)
		key := entryKey{device: ce.Device, version: version, source: source}
		// Keep the newest entry if duplicates.
		if existing, ok := entries[key]; !ok || ce.CreatedAt.After(existing.CreatedAt) {
			entries[key] = ce
		}
	}

	// Filter by device family.
	categoryLower := strings.ToLower(category)
	var items []lhFirmwareListItem

	for key := range entries {
		if !matchesDeviceFamily(key.device, categoryLower) {
			continue
		}

		fid := buildFID(key.device, key.version, key.source)
		name := fmt.Sprintf("%s %s [%s]", key.device, key.version, key.source)
		items = append(items, lhFirmwareListItem{
			FID:    fid,
			Name:   name,
			Author: key.source,
			Star:   false,
		})
	}

	// Sort by name descending (newest versions first — versions contain commit hashes
	// so this isn't perfect, but it groups by device).
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name > items[j].Name
	})

	// Pagination.
	pageSize := 20
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v := parseInt(p); v > 0 {
			page = v
		}
	}

	total := len(items)
	start := (page - 1) * pageSize
	if start >= len(items) {
		items = nil
	} else {
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		items = items[start:end]
	}
	if items == nil {
		items = []lhFirmwareListItem{}
	}

	s.lhJSON(w, http.StatusOK, lhFirmwareListResponse{
		Total:    total,
		PageSize: pageSize,
		Page:     page,
		Items:    items,
	})
}

func (s *Server) handleLauncherHubFirmwareDetail(w http.ResponseWriter, r *http.Request, requestID string, fid string) {
	device, version, source, err := parseFID(fid)
	if err != nil {
		s.lhError(w, http.StatusBadRequest, err.Error())
		return
	}

	cacheEntry, firmwarePath := s.findCacheEntry(device, version, source)
	if cacheEntry == nil {
		s.lhError(w, http.StatusNotFound, fmt.Sprintf(
			"Version '%s' not found for device '%s' in source '%s'", version, device, source))
		return
	}

	// Build download URL.
	escapedFID := url.QueryEscape(fid)
	baseURL := schemeHost(r)
	downloadURL := fmt.Sprintf("%s/api/launcherhub/download?fid=%s", baseURL, escapedFID)

	vd := lhVersionDetail{
		Version:     version,
		PublishedAt: cacheEntry.CreatedAt.Format("2006-01-02T15:04:05Z"),
		File:        downloadURL,
		ZipURL:      downloadURL,
		NoBoot:      true, // Cache stores OTA-ready binaries (no bootloader).
	}

	// Try to get file size.
	if firmwarePath != "" {
		if info, err := os.Stat(firmwarePath); err == nil {
			vd.FileSize = info.Size()
		}
	}

	// Try to parse partition table from partitions.bin in the same cache entry.
	partPath := s.findArtifactInCache(cacheEntry.Key, "partitions.bin")
	if partPath != "" {
		if pt, err := parsePartitionTable(partPath); err == nil {
			vd.NoBoot = false
			vd.AppOffset = pt.AppOffset
			vd.AppSize = pt.AppSize
			vd.HasSPIFFS = pt.SpiffsSize > 0
			vd.SpiffsOff = pt.SpiffsOffset
			vd.SpiffsSize = pt.SpiffsSize
			vd.HasFAT1 = pt.FatSize > 0
			vd.FatOff = pt.FatOffset
			vd.FatSize = pt.FatSize
			vd.HasFAT2 = pt.Fat2Size > 0
			vd.Fat2Off = pt.Fat2Offset
			vd.Fat2Size = pt.Fat2Size
		}
	}

	name := fmt.Sprintf("%s %s [%s]", device, version, source)
	s.lhJSON(w, http.StatusOK, lhFirmwareDetailResponse{
		FID:      fid,
		Name:     name,
		Author:   source,
		Star:     false,
		Versions: []lhVersionDetail{vd},
	})
}

func (s *Server) handleLauncherHubDownload(w http.ResponseWriter, r *http.Request, requestID string) {
	fid := r.URL.Query().Get("fid")
	if fid == "" {
		s.lhError(w, http.StatusBadRequest, "fid query param is required")
		return
	}

	device, version, source, err := parseFID(fid)
	if err != nil {
		s.lhError(w, http.StatusBadRequest, err.Error())
		return
	}

	cacheEntry, firmwarePath := s.findCacheEntry(device, version, source)
	if cacheEntry == nil || firmwarePath == "" {
		s.lhError(w, http.StatusNotFound, fmt.Sprintf(
			"Firmware not found for device '%s' version '%s'", device, version))
		return
	}

	f, err := os.Open(firmwarePath)
	if err != nil {
		s.lhError(w, http.StatusInternalServerError, "cannot read firmware file")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		s.lhError(w, http.StatusInternalServerError, "cannot stat firmware file")
		return
	}

	filename := fmt.Sprintf("firmware-%s-%s.bin", device, version)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	if s.stats != nil {
		s.stats.Record(stats.Event{
			Type:      stats.EventDownload,
			IP:        clientIP(r, s.cfg.TrustProxyHeaders),
			UserAgent: r.UserAgent(),
			Extra:     fmt.Sprintf("launcherhub:%s", fid),
		})
	}

	http.ServeContent(w, r, filename, info.ModTime(), f)
}

// --- helpers ---

func (s *Server) lhJSON(w http.ResponseWriter, status int, payload any) {
	s.writeJSON(w, status, payload)
}

func (s *Server) lhError(w http.ResponseWriter, status int, detail string) {
	s.writeJSON(w, status, map[string]string{"detail": detail})
}

// findCacheEntry locates a cache entry matching device+version+source
// and returns the path to the best firmware binary for OTA.
func (s *Server) findCacheEntry(device string, version string, source string) (*jobs.FirmwareCacheEntry, string) {
	cacheInfo := jobs.ScanFirmwareCache(s.cfg.FirmwareCachePath)

	for _, ce := range cacheInfo.Entries {
		if !strings.EqualFold(ce.Device, device) {
			continue
		}
		entryVersion := extractVersionFromCacheEntry(ce)
		if !strings.EqualFold(entryVersion, version) {
			continue
		}
		entrySource := sourceFromRepoURL(ce.RepoURL)
		if !strings.EqualFold(entrySource, source) {
			continue
		}

		// Find the best firmware binary: prefer firmware.bin (OTA-ready),
		// then firmware-<device>-*.bin, then *.factory.bin as last resort.
		firmwarePath := s.findBestFirmwareBinary(ce)
		return &ce, firmwarePath
	}

	return nil, ""
}

// findBestFirmwareBinary picks the best OTA binary from cache artifacts.
// Preference: device-specific firmware.bin > generic firmware.bin > factory.bin
func (s *Server) findBestFirmwareBinary(ce jobs.FirmwareCacheEntry) string {
	var genericFirmware, deviceFirmware, factoryFirmware string

	for _, a := range ce.Artifacts {
		lower := strings.ToLower(a.Name)
		path := filepath.Join(s.cfg.FirmwareCachePath, ce.Key, "files", a.Name)

		if lower == "firmware.bin" {
			genericFirmware = path
			continue
		}
		if strings.HasPrefix(lower, "firmware-") && strings.HasSuffix(lower, ".bin") && !strings.Contains(lower, "factory") {
			deviceFirmware = path
			continue
		}
		if strings.HasSuffix(lower, ".factory.bin") {
			factoryFirmware = path
			continue
		}
	}

	if deviceFirmware != "" {
		return deviceFirmware
	}
	if genericFirmware != "" {
		return genericFirmware
	}
	return factoryFirmware
}

// findArtifactInCache returns the absolute path to a named artifact in a cache entry.
func (s *Server) findArtifactInCache(cacheKey string, name string) string {
	path := filepath.Join(s.cfg.FirmwareCachePath, cacheKey, "files", name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// parseFID parses "device|version[|source]" into components.
func parseFID(fid string) (device, version, source string, err error) {
	parts := strings.Split(fid, "|")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", "", fmt.Errorf(
			"fid must be in format 'device|version[|src]', e.g. 't-deck|v2.7.18.fb3bf78|Official repo'")
	}

	device = strings.TrimSpace(parts[0])
	version = strings.TrimSpace(parts[1])
	source = "default"
	if len(parts) == 3 {
		decoded, err := url.QueryUnescape(strings.TrimSpace(parts[2]))
		if err == nil && decoded != "" {
			source = decoded
		}
	}

	if device == "" || version == "" {
		return "", "", "", fmt.Errorf("device and version are required in fid")
	}

	return device, version, source, nil
}

// buildFID constructs a firmware ID string.
func buildFID(device, version, source string) string {
	return fmt.Sprintf("%s|%s|%s", device, version, url.QueryEscape(source))
}

// sourceFromRepoURL extracts a human-readable source name from a repository URL.
// "https://github.com/meshtastic/firmware.git" → "meshtastic/firmware"
func sourceFromRepoURL(repoURL string) string {
	if repoURL == "" {
		return "default"
	}
	u := strings.TrimSuffix(repoURL, ".git")
	// Remove scheme + host.
	if idx := strings.Index(u, "://"); idx >= 0 {
		u = u[idx+3:]
	}
	// Remove host part.
	if idx := strings.Index(u, "/"); idx >= 0 {
		u = u[idx+1:]
	}
	u = strings.Trim(u, "/")
	if u == "" {
		return "default"
	}
	return u
}

// extractVersionFromCacheEntry extracts a version string from cache entry artifacts or ref.
// Tries firmware file names first (firmware-<device>-<version>.bin), falls back to ref.
func extractVersionFromCacheEntry(ce jobs.FirmwareCacheEntry) string {
	// Try to extract from artifact file names.
	for _, a := range ce.Artifacts {
		lower := strings.ToLower(a.Name)
		if !strings.HasPrefix(lower, "firmware-") {
			continue
		}
		// Remove extension.
		base := lower
		for _, ext := range []string{".factory.bin", ".bin", ".elf", ".hex", ".uf2"} {
			if strings.HasSuffix(base, ext) {
				base = strings.TrimSuffix(base, ext)
				break
			}
		}
		// Format: firmware-<device>-<version>
		// Find version: split by '-', find first part starting with digit + dot.
		parts := strings.Split(strings.TrimPrefix(base, "firmware-"), "-")
		for i, p := range parts {
			if len(p) > 0 && p[0] >= '0' && p[0] <= '9' && strings.Contains(p, ".") {
				return strings.Join(parts[i:], "-")
			}
		}
	}

	// Fall back to ref field.
	if ce.Ref != "" {
		return ce.Ref
	}

	return ""
}

// matchesDeviceFamily checks if a device name belongs to the queried family.
// Uses dash-prefix matching: "t-deck" matches "t-deck", "t-deck-tft", "t-deck-plus".
// Two devices are in the same family if they share a dash-segment prefix that is either:
//   - at least 2 segments long (prevents "t-deck" matching "t-echo"), or
//   - the full name of the device or query (allows "heltec" to match "heltec-v4").
func matchesDeviceFamily(device string, query string) bool {
	deviceLower := strings.ToLower(device)
	if deviceLower == query {
		return true
	}

	deviceParts := strings.Split(deviceLower, "-")
	queryParts := strings.Split(query, "-")

	// Count shared leading dash-segments.
	maxShared := len(deviceParts)
	if len(queryParts) < maxShared {
		maxShared = len(queryParts)
	}
	shared := 0
	for i := 0; i < maxShared; i++ {
		if deviceParts[i] != queryParts[i] {
			break
		}
		shared++
	}

	if shared == 0 {
		return false
	}

	// Match if shared prefix covers the entire device or query name,
	// or if the shared prefix is at least 2 segments (a meaningful family root).
	return shared == len(deviceParts) || shared == len(queryParts) || shared >= 2
}

// schemeHost returns the scheme+host for building absolute URLs.
func schemeHost(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// --- ESP32 partition table parser ---

// partitionInfo holds parsed partition layout info.
type partitionInfo struct {
	AppOffset   int64
	AppSize     int64
	SpiffsOffset int64
	SpiffsSize  int64
	FatOffset   int64
	FatSize     int64
	Fat2Offset  int64
	Fat2Size    int64
}

// ESP32 partition table constants.
const (
	partEntrySize  = 32
	partMagicByte  = 0xAA
	partMagicByte2 = 0x50

	partTypeApp  = 0x00
	partTypeData = 0x01

	partSubtypeFactory = 0x00
	partSubtypeOTA0    = 0x10
	partSubtypeSPIFFS  = 0x82
	partSubtypeFAT     = 0x81
)

// parsePartitionTable reads an ESP32 partitions.bin file and extracts layout info.
func parsePartitionTable(path string) (partitionInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return partitionInfo{}, err
	}

	var info partitionInfo
	fatCount := 0

	for offset := 0; offset+partEntrySize <= len(data); offset += partEntrySize {
		entry := data[offset : offset+partEntrySize]

		// Check magic bytes.
		if entry[0] != partMagicByte || entry[1] != partMagicByte2 {
			continue
		}

		partType := entry[2]
		partSubtype := entry[3]
		partOffset := int64(binary.LittleEndian.Uint32(entry[4:8]))
		partSize := int64(binary.LittleEndian.Uint32(entry[8:12]))

		switch {
		case partType == partTypeApp && (partSubtype == partSubtypeFactory || partSubtype == partSubtypeOTA0):
			if info.AppSize == 0 {
				info.AppOffset = partOffset
				info.AppSize = partSize
			}
		case partType == partTypeData && partSubtype == partSubtypeSPIFFS:
			info.SpiffsOffset = partOffset
			info.SpiffsSize = partSize
		case partType == partTypeData && partSubtype == partSubtypeFAT:
			if fatCount == 0 {
				info.FatOffset = partOffset
				info.FatSize = partSize
			} else if fatCount == 1 {
				info.Fat2Offset = partOffset
				info.Fat2Size = partSize
			}
			fatCount++
		}
	}

	return info, nil
}
