package jobs

import "testing"

func TestValidateRepoURL(t *testing.T) {
	t.Parallel()

	valid := []string{
		"https://github.com/meshtastic/firmware.git",
		"http://git.example.local/team/repo",
		"ssh://git@example.com/team/repo.git",
		"git://example.com/team/repo.git",
		"git@example.com:team/repo.git",
	}

	for _, value := range valid {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if err := ValidateRepoURL(value); err != nil {
				t.Fatalf("expected valid URL %q, got error: %v", value, err)
			}
		})
	}

	invalid := []string{
		"",
		"file:///tmp/repo",
		"https://",
		"https://bad repo",
		"../../firmware",
	}

	for _, value := range invalid {
		value := value
		t.Run("invalid_"+value, func(t *testing.T) {
			t.Parallel()
			if err := ValidateRepoURL(value); err == nil {
				t.Fatalf("expected invalid URL %q", value)
			}
		})
	}
}

func TestValidateDeviceAndRef(t *testing.T) {
	t.Parallel()

	if err := ValidateDevice("tbeam-s3-core"); err != nil {
		t.Fatalf("expected valid device, got %v", err)
	}

	if err := ValidateDevice("../escape"); err == nil {
		t.Fatalf("expected invalid device")
	}

	if err := ValidateRef("feature/with-tag_1.0"); err != nil {
		t.Fatalf("expected valid ref, got %v", err)
	}

	if err := ValidateRef("bad ref"); err == nil {
		t.Fatalf("expected invalid ref")
	}
}
