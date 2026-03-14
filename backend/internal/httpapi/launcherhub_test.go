package httpapi

import (
	"testing"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/jobs"
)

func TestMatchesDeviceFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		device string
		query  string
		want   bool
	}{
		// Exact match.
		{"t-deck", "t-deck", true},
		{"heltec-v4", "heltec-v4", true},

		// Family variants: query "t-deck" should match all t-deck-* devices.
		{"t-deck-tft", "t-deck", true},
		{"t-deck-plus", "t-deck", true},
		{"t-deck-tft-ru", "t-deck", true},

		// Query for a specific variant should still match the family root.
		{"t-deck", "t-deck-plus", true},
		{"t-deck-tft", "t-deck-plus", true},

		// Heltec family.
		{"heltec-wireless-paper", "heltec", true},
		{"heltec-v4", "heltec", true},
		{"heltec-wireless-paper", "heltec-wireless-paper", true},

		// No match — different families.
		{"t-echo", "t-deck", false},
		{"rak4631", "t-deck", false},
		{"heltec-v4", "t-deck", false},

		// Case insensitive.
		{"T-Deck-TFT", "t-deck", true},

		// Single-segment device names.
		{"rak4631", "rak4631", true},
		{"rak4631", "rak", false},

		// Edge case: query is a prefix of device but not at dash boundary.
		{"t-deckplus", "t-deck", false},
	}

	for _, tt := range tests {
		got := matchesDeviceFamily(tt.device, tt.query)
		if got != tt.want {
			t.Errorf("matchesDeviceFamily(%q, %q) = %v, want %v", tt.device, tt.query, got, tt.want)
		}
	}
}

func TestParseFID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fid     string
		device  string
		version string
		source  string
		wantErr bool
	}{
		{"t-deck|v2.7.19.bb3d6d5|Official%20repo", "t-deck", "v2.7.19.bb3d6d5", "Official repo", false},
		{"t-deck|v2.7.19.bb3d6d5", "t-deck", "v2.7.19.bb3d6d5", "default", false},
		{"heltec-v4|2.7.20.016e68e|meshtastic/firmware", "heltec-v4", "2.7.20.016e68e", "meshtastic/firmware", false},
		{"", "", "", "", true},
		{"invalid", "", "", "", true},
		{"a|b|c|d", "", "", "", true},
	}

	for _, tt := range tests {
		device, version, source, err := parseFID(tt.fid)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseFID(%q) error = %v, wantErr %v", tt.fid, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if device != tt.device || version != tt.version || source != tt.source {
			t.Errorf("parseFID(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.fid, device, version, source, tt.device, tt.version, tt.source)
		}
	}
}

func TestSourceFromRepoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/meshtastic/firmware.git", "meshtastic/firmware"},
		{"https://github.com/skrashevich/firmware.git", "skrashevich/firmware"},
		{"https://github.com/user/repo", "user/repo"},
		{"", "default"},
	}

	for _, tt := range tests {
		got := sourceFromRepoURL(tt.url)
		if got != tt.want {
			t.Errorf("sourceFromRepoURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestExtractVersionFromCacheEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		artifacts []string
		ref       string
		want      string
	}{
		{
			"device-specific firmware",
			[]string{"firmware-heltec-v4-2.7.20.016e68e.bin", "bootloader.bin"},
			"main",
			"2.7.20.016e68e",
		},
		{
			"firmware with factory",
			[]string{"firmware-t-echo-2.7.20.b941dab.factory.bin", "firmware-t-echo-2.7.20.b941dab.elf"},
			"v2.5.12",
			"2.7.20.b941dab",
		},
		{
			"generic firmware only, fallback to ref",
			[]string{"firmware.bin", "bootloader.bin", "partitions.bin"},
			"v2.7.19",
			"v2.7.19",
		},
		{
			"no version extractable, no ref",
			[]string{"firmware.bin"},
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var artifacts []jobs.FirmwareCacheArtifactInfo
			for _, name := range tt.artifacts {
				artifacts = append(artifacts, jobs.FirmwareCacheArtifactInfo{Name: name})
			}
			ce := jobs.FirmwareCacheEntry{Ref: tt.ref, Artifacts: artifacts}
			got := extractVersionFromCacheEntry(ce)
			if got != tt.want {
				t.Errorf("extractVersionFromCacheEntry() = %q, want %q", got, tt.want)
			}
		})
	}
}
