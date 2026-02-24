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
