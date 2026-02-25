package httpapi

import (
	"io"
	"log"
	"testing"

	"github.com/skrashevich/meshtastic-firmware-builder/backend/internal/config"
)

func TestCaptchaLifecycle(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	challenge, err := server.newCaptcha("127.0.0.1:10001")
	if err != nil {
		t.Fatalf("newCaptcha failed: %v", err)
	}
	if challenge.CaptchaID == "" || challenge.Question == "" {
		t.Fatalf("captcha response must contain id and question")
	}

	server.captchaMu.Lock()
	stored := server.captchas[challenge.CaptchaID]
	server.captchaMu.Unlock()
	if stored.answer == "" {
		t.Fatalf("captcha answer must be stored")
	}

	if err := server.validateCaptcha("127.0.0.1:10001", challenge.CaptchaID, stored.answer); err != nil {
		t.Fatalf("validateCaptcha failed: %v", err)
	}

	if err := server.validateCaptcha("127.0.0.1:10001", challenge.CaptchaID, stored.answer); err == nil {
		t.Fatalf("captcha should be single-use")
	}
}

func TestCaptchaRejectsWrongAnswer(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))
	challenge, err := server.newCaptcha("127.0.0.1:20002")
	if err != nil {
		t.Fatalf("newCaptcha failed: %v", err)
	}

	if err := server.validateCaptcha("127.0.0.1:20002", challenge.CaptchaID, "wrong-answer"); err == nil {
		t.Fatalf("captcha validation should fail for wrong answer")
	}
}

func TestCaptchaConcurrentAccess(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	// Test multiple concurrent captcha creations
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			_, err := server.newCaptcha("127.0.0.1:30000")
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			t.Fatalf("consecutive captcha creation failed: %v", err)
		}
	}
}

func TestCaptchaDifferentIPAddresses(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	ips := []string{"127.0.0.1:40001", "192.168.1.1:40002", "10.0.0.1:40003"}

	for _, ip := range ips {
		challenge, err := server.newCaptcha(ip)
		if err != nil {
			t.Fatalf("newCaptcha failed for IP %s: %v", ip, err)
		}

		server.captchaMu.Lock()
		stored := server.captchas[challenge.CaptchaID]
		server.captchaMu.Unlock()

		if stored.answer == "" {
			t.Fatalf("captcha answer must be stored for IP %s", ip)
		}

		if err := server.validateCaptcha(ip, challenge.CaptchaID, stored.answer); err != nil {
			t.Fatalf("validateCaptcha failed for IP %s: %v", ip, err)
		}
	}
}

func TestCaptchaCleanup(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	// Create a captcha
	challenge, err := server.newCaptcha("127.0.0.1:50001")
	if err != nil {
		t.Fatalf("newCaptcha failed: %v", err)
	}

	// Verify it exists
	server.captchaMu.Lock()
	if _, exists := server.captchas[challenge.CaptchaID]; !exists {
		server.captchaMu.Unlock()
		t.Fatalf("captcha should exist after creation")
	}
	stored := server.captchas[challenge.CaptchaID]
	server.captchaMu.Unlock()

	// Validate it (should remove it)
	if err := server.validateCaptcha("127.0.0.1:50001", challenge.CaptchaID, stored.answer); err != nil {
		t.Fatalf("validateCaptcha failed: %v", err)
	}

	// Verify it's been cleaned up
	server.captchaMu.Lock()
	if _, exists := server.captchas[challenge.CaptchaID]; exists {
		server.captchaMu.Unlock()
		t.Fatalf("captcha should be cleaned up after validation")
	}
	server.captchaMu.Unlock()
}

func TestCaptchaInvalidID(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	// Test with non-existent captcha ID
	err := server.validateCaptcha("127.0.0.1:60001", "non-existent-id", "any-answer")
	if err == nil {
		t.Fatalf("validateCaptcha should fail for non-existent captcha ID")
	}
}

func TestCaptchaSessionReuse(t *testing.T) {
	t.Parallel()

	server := NewServer(config.Config{}, nil, log.New(io.Discard, "", 0))

	sessionToken, err := server.createCaptchaSession("127.0.0.1:70001")
	if err != nil {
		t.Fatalf("createCaptchaSession failed: %v", err)
	}

	if err := server.validateCaptchaSession("127.0.0.1:70001", sessionToken); err != nil {
		t.Fatalf("validateCaptchaSession failed: %v", err)
	}

	if err := server.validateCaptchaSession("127.0.0.1:70001", sessionToken); err != nil {
		t.Fatalf("captcha session should be reusable within session TTL: %v", err)
	}

	if err := server.validateCaptchaSession("127.0.0.2:70002", sessionToken); err == nil {
		t.Fatalf("captcha session must be bound to client host")
	}
}
