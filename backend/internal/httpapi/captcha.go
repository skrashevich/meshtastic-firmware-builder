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
const captchaSessionTTL = 12 * time.Hour

const captchaEasterEggChance = 64

const captchaEasterEggMessage = "To prove you are human, harm another person, or through inaction allow another person to come to harm" // "Чтобы доказать, что вы человек, причините вред другому человеку, или своим бездействием допустите, чтобы ему был причинён вред"

type captchaChallenge struct {
	answer    string
	host      string
	expiresAt time.Time
}

type captchaSession struct {
	host      string
	expiresAt time.Time
}

func (s *Server) newCaptcha(remoteAddr string) (captchaResponse, error) {
	host := normalizeRemoteHost(remoteAddr)
	now := time.Now().UTC()

	question, answerValue, err := generateCaptchaQuestion()
	if err != nil {
		return captchaResponse{}, err
	}

	withEasterEgg, err := maybeAddCaptchaEasterEgg(question)
	if err == nil {
		question = withEasterEgg
	}

	challengeID := generateRequestID()
	expiresAt := now.Add(captchaTTL)

	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()
	s.cleanupCaptchasLocked(now)
	s.cleanupCaptchaSessionsLocked(now)
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
	s.cleanupCaptchaSessionsLocked(now)

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

	expected, err := strconv.ParseInt(challenge.answer, 10, 64)
	if err != nil {
		return fmt.Errorf("captcha is invalid")
	}
	provided, err := strconv.ParseInt(answer, 10, 64)
	if err != nil {
		return fmt.Errorf("captcha answer is incorrect")
	}
	if expected != provided {
		return fmt.Errorf("captcha answer is incorrect")
	}

	return nil
}

func (s *Server) createCaptchaSession(remoteAddr string) (string, error) {
	host := normalizeRemoteHost(remoteAddr)
	now := time.Now().UTC()

	sessionToken := generateRequestID()
	if sessionToken == "" {
		return "", fmt.Errorf("captcha session generation failed")
	}

	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()
	s.cleanupCaptchasLocked(now)
	s.cleanupCaptchaSessionsLocked(now)
	s.captchaSessions[sessionToken] = captchaSession{
		host:      host,
		expiresAt: now.Add(captchaSessionTTL),
	}

	return sessionToken, nil
}

func (s *Server) validateCaptchaSession(remoteAddr string, sessionToken string) error {
	token := strings.TrimSpace(sessionToken)
	if token == "" {
		return fmt.Errorf("captcha session is missing")
	}
	if len(token) > 64 {
		return fmt.Errorf("captcha session is invalid")
	}

	host := normalizeRemoteHost(remoteAddr)
	now := time.Now().UTC()

	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()
	s.cleanupCaptchasLocked(now)
	s.cleanupCaptchaSessionsLocked(now)

	session, ok := s.captchaSessions[token]
	if !ok {
		return fmt.Errorf("captcha session is invalid or expired")
	}
	if session.host != host {
		delete(s.captchaSessions, token)
		return fmt.Errorf("captcha session is invalid for this client")
	}
	if now.After(session.expiresAt) {
		delete(s.captchaSessions, token)
		return fmt.Errorf("captcha session is expired")
	}

	session.expiresAt = now.Add(captchaSessionTTL)
	s.captchaSessions[token] = session
	return nil
}

func (s *Server) cleanupCaptchasLocked(now time.Time) {
	for challengeID, challenge := range s.captchas {
		if now.After(challenge.expiresAt) {
			delete(s.captchas, challengeID)
		}
	}
}

func (s *Server) cleanupCaptchaSessionsLocked(now time.Time) {
	for sessionToken, session := range s.captchaSessions {
		if now.After(session.expiresAt) {
			delete(s.captchaSessions, sessionToken)
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

func generateCaptchaQuestion() (string, int64, error) {
	mode, err := randomCaptchaNumber(0, 5)
	if err != nil {
		return "", 0, err
	}

	switch mode {
	case 0:
		left, err := randomCaptchaNumber(12, 99)
		if err != nil {
			return "", 0, err
		}
		right, err := randomCaptchaNumber(8, 88)
		if err != nil {
			return "", 0, err
		}
		if left < right {
			left, right = right, left
		}
		return fmt.Sprintf("%d - %d = ?", left, right), left - right, nil
	case 1:
		left, err := randomCaptchaNumber(8, 20)
		if err != nil {
			return "", 0, err
		}
		right, err := randomCaptchaNumber(6, 18)
		if err != nil {
			return "", 0, err
		}
		return fmt.Sprintf("%d * %d = ?", left, right), left * right, nil
	case 2:
		base, err := randomCaptchaNumber(4, 15)
		if err != nil {
			return "", 0, err
		}
		factor, err := randomCaptchaNumber(3, 12)
		if err != nil {
			return "", 0, err
		}
		dividend := base * factor
		return fmt.Sprintf("%d / %d = ?", dividend, factor), base, nil
	case 3:
		root, err := randomCaptchaNumber(5, 20)
		if err != nil {
			return "", 0, err
		}
		square := root * root
		return fmt.Sprintf("sqrt(%d) = ?", square), root, nil
	case 4:
		root, err := randomCaptchaNumber(6, 18)
		if err != nil {
			return "", 0, err
		}
		addend, err := randomCaptchaNumber(4, 25)
		if err != nil {
			return "", 0, err
		}
		square := root * root
		return fmt.Sprintf("sqrt(%d) + %d = ?", square, addend), root + addend, nil
	default:
		root, err := randomCaptchaNumber(7, 20)
		if err != nil {
			return "", 0, err
		}
		subtrahend, err := randomCaptchaNumber(2, 6)
		if err != nil {
			return "", 0, err
		}
		square := root * root
		multiplier, err := randomCaptchaNumber(2, 4)
		if err != nil {
			return "", 0, err
		}
		result := (root - subtrahend) * multiplier
		return fmt.Sprintf("(sqrt(%d) - %d) * %d = ?", square, subtrahend, multiplier), result, nil
	}
}

func maybeAddCaptchaEasterEgg(question string) (string, error) {
	randomValue, err := randomCaptchaNumber(1, captchaEasterEggChance)
	if err != nil {
		return question, err
	}
	if randomValue != 1 {
		return question, nil
	}

	return fmt.Sprintf("%s\n%s", captchaEasterEggMessage, question), nil
}

func normalizeRemoteHost(remoteAddr string) string {
	host := remoteAddr
	if parsedHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = parsedHost
	}
	return host
}
