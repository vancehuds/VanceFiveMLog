package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const CookieName = "vfl_session"

type SessionManager struct {
	secret []byte
	secure bool
	now    func() time.Time
}

type Session struct {
	AdminID int64
	Expiry  time.Time
}

func NewSessionManager(secret string) *SessionManager {
	return &SessionManager{
		secret: []byte(secret),
		now:    time.Now,
	}
}

func (m *SessionManager) WithSecureCookie(secure bool) *SessionManager {
	m.secure = secure
	return m
}

func (m *SessionManager) Set(w http.ResponseWriter, adminID int64) error {
	expiry := m.now().Add(12 * time.Hour)
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	payload := fmt.Sprintf("%d:%d:%s", adminID, expiry.Unix(), base64.RawURLEncoding.EncodeToString(nonce))
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + m.sign(payload)

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (m *SessionManager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *SessionManager) Read(r *http.Request) (Session, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return Session{}, err
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return Session{}, errors.New("invalid session shape")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Session{}, err
	}
	payload := string(payloadBytes)
	expected := m.sign(payload)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[1])) != 1 {
		return Session{}, errors.New("invalid session signature")
	}

	fields := strings.Split(payload, ":")
	if len(fields) != 3 {
		return Session{}, errors.New("invalid session payload")
	}
	adminID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return Session{}, err
	}
	expiryUnix, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return Session{}, err
	}
	expiry := time.Unix(expiryUnix, 0)
	if !expiry.After(m.now()) {
		return Session{}, errors.New("session expired")
	}

	return Session{AdminID: adminID, Expiry: expiry}, nil
}

func (m *SessionManager) CSRFToken(session Session) string {
	return m.sign(m.csrfPayload(session))
}

func (m *SessionManager) VerifyCSRFToken(session Session, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	expected := m.CSRFToken(session)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}

func (m *SessionManager) csrfPayload(session Session) string {
	return fmt.Sprintf("csrf:%d:%d", session.AdminID, session.Expiry.Unix())
}

func (m *SessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
