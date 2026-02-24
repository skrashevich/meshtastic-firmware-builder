package httpapi

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"
)

const captchaTTL = 10 * time.Minute

type captchaChallenge struct {
	answer    string
	host      string
	expiresAt time.Time
}

func (s *Server) newCaptcha(remoteAddr string) (captchaResponse, error) {
	host := normalizeRemoteHost(remoteAddr)
	now := time.Now().UTC()

	left, err := randomCaptchaNumber(3, 25)
	if err != nil {
		return captchaResponse{}, err
	}
	right, err := randomCaptchaNumber(2, 20)
	if err != nil {
		return captchaResponse{}, err
	}

	operator := "+"
	answerValue := left + right
	if left >= right {
		useSub, randErr := randomCaptchaNumber(0, 1)
		if randErr == nil && useSub == 1 {
			operator = "-"
			answerValue = left - right
		}
	}

	challengeID := generateRequestID()
	question := fmt.Sprintf("%d %s %d = ?", left, operator, right)
	expiresAt := now.Add(captchaTTL)

	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()
	s.cleanupCaptchasLocked(now)
	s.captchas[challengeID] = captchaChallenge{
		answer:    strconv.FormatInt(answerValue, 10),
		host:      host,
		expiresAt: expiresAt,
	}

	return captchaResponse{
		CaptchaID: challengeID,
		Question:  question,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Server) validateCaptcha(remoteAddr string, captchaID string, captchaAnswer string) error {
	challengeID := strings.TrimSpace(captchaID)
	answer := strings.TrimSpace(captchaAnswer)
	if challengeID == "" || answer == "" {
		return fmt.Errorf("captcha is required")
	}
	if len(challengeID) > 64 || len(answer) > 16 {
		return fmt.Errorf("captcha is invalid")
	}

	host := normalizeRemoteHost(remoteAddr)
	now := time.Now().UTC()

	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()
	s.cleanupCaptchasLocked(now)

	challenge, ok := s.captchas[challengeID]
	if !ok {
		return fmt.Errorf("captcha is invalid or expired")
	}
	delete(s.captchas, challengeID)

	if challenge.host != host {
		return fmt.Errorf("captcha is invalid for this client")
	}
	if now.After(challenge.expiresAt) {
		return fmt.Errorf("captcha is expired")
	}
	if challenge.answer != answer {
		return fmt.Errorf("captcha answer is incorrect")
	}

	return nil
}

func (s *Server) cleanupCaptchasLocked(now time.Time) {
	for challengeID, challenge := range s.captchas {
		if now.After(challenge.expiresAt) {
			delete(s.captchas, challengeID)
		}
	}
}

func randomCaptchaNumber(min int64, max int64) (int64, error) {
	if max < min {
		return 0, fmt.Errorf("captcha random range is invalid")
	}
	if max == min {
		return min, nil
	}

	rangeSize := max - min + 1
	randomValue, err := rand.Int(rand.Reader, big.NewInt(rangeSize))
	if err != nil {
		return 0, fmt.Errorf("generate captcha random number: %w", err)
	}

	return min + randomValue.Int64(), nil
}

func normalizeRemoteHost(remoteAddr string) string {
	host := remoteAddr
	if parsedHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = parsedHost
	}
	return host
}
