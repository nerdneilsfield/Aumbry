// Package auth implements API tokens, the admin login and the signed session cookies.
package auth

import (
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminCookie   = "aumbry_admin"
	visitorCookie = "aumbry_unlocked"
	adminMaxAge   = 7 * 24 * time.Hour
	visitorMaxAge = 30 * 24 * time.Hour
)

// Settings are the credentials and secrets the server was configured with.
type Settings struct {
	AdminUser         string
	AdminPasswordHash string
	APITokens         []string
	CookieSecret      string
}

// Sessions verifies credentials and issues the encrypted, signed session cookies.
type Sessions struct {
	user      string
	hash      []byte
	adminFP   string
	tokens    []string
	adminSC   *securecookie.SecureCookie
	visitorSC *securecookie.SecureCookie
}

// New validates the settings (the admin hash must be a bcrypt hash) and derives the cookie keys.
func New(s Settings) (*Sessions, error) {
	if _, err := bcrypt.Cost([]byte(s.AdminPasswordHash)); err != nil {
		return nil, fmt.Errorf("AUMBRY_ADMIN_PASSWORD_HASH is not a valid bcrypt hash: %w", err)
	}
	secret := []byte(s.CookieSecret)
	hashKey, err := hkdf.Key(sha256.New, secret, nil, "aumbry cookie hmac", 32)
	if err != nil {
		return nil, err
	}
	blockKey, err := hkdf.Key(sha256.New, secret, nil, "aumbry cookie aes", 32)
	if err != nil {
		return nil, err
	}
	newCodec := func(maxAge time.Duration) *securecookie.SecureCookie {
		return securecookie.New(hashKey, blockKey).
			SetSerializer(securecookie.JSONEncoder{}).
			MaxAge(int(maxAge.Seconds()))
	}
	return &Sessions{
		user:      s.AdminUser,
		hash:      []byte(s.AdminPasswordHash),
		adminFP:   Fingerprint(s.AdminPasswordHash),
		tokens:    s.APITokens,
		adminSC:   newCodec(adminMaxAge),
		visitorSC: newCodec(visitorMaxAge),
	}, nil
}

// Fingerprint identifies one password hash. Cookies carry it so that changing or
// clearing a password invalidates the sessions issued for the old one.
func Fingerprint(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return hex.EncodeToString(sum[:6])
}

// CheckToken reports whether the Authorization header carries a configured API token.
func (s *Sessions) CheckToken(header string) bool {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	token = strings.TrimSpace(token)
	matched := 0
	for _, t := range s.tokens {
		matched |= digestEqual(token, t)
	}
	return matched == 1
}

// CheckAdmin verifies the admin credentials. The bcrypt comparison always runs so that
// a wrong username is not distinguishable by timing.
func (s *Sessions) CheckAdmin(user, password string) bool {
	userOK := digestEqual(user, s.user)
	passOK := bcrypt.CompareHashAndPassword(s.hash, []byte(password)) == nil
	return userOK == 1 && passOK
}

// digestEqual compares two strings in constant time regardless of their lengths.
func digestEqual(a, b string) int {
	da, db := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(da[:], db[:])
}

// IsAdmin reports whether the request carries a valid admin session cookie.
func (s *Sessions) IsAdmin(r *http.Request) bool {
	c, err := r.Cookie(adminCookie)
	if err != nil {
		return false
	}
	var fp string
	if s.adminSC.Decode(adminCookie, c.Value, &fp) != nil {
		return false
	}
	return fp == s.adminFP
}

// LoginAdmin issues the admin session cookie.
func (s *Sessions) LoginAdmin(w http.ResponseWriter, r *http.Request) error {
	value, err := s.adminSC.Encode(adminCookie, s.adminFP)
	if err != nil {
		return err
	}
	http.SetCookie(w, newCookie(r, adminCookie, value, adminMaxAge))
	return nil
}

// LogoutAdmin expires the admin session cookie.
func (s *Sessions) LogoutAdmin(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, newCookie(r, adminCookie, "", -1))
}

// Unlocked returns the directories the visitor unlocked, as directory id to the fingerprint
// of the password that unlocked it. A missing, forged or tampered cookie yields an empty map.
func (s *Sessions) Unlocked(r *http.Request) map[string]string {
	unlocked := map[string]string{}
	if c, err := r.Cookie(visitorCookie); err == nil {
		if s.visitorSC.Decode(visitorCookie, c.Value, &unlocked) != nil {
			return map[string]string{}
		}
	}
	return unlocked
}

// SetUnlocked issues the visitor cookie holding the given unlocked directories.
func (s *Sessions) SetUnlocked(w http.ResponseWriter, r *http.Request, unlocked map[string]string) error {
	value, err := s.visitorSC.Encode(visitorCookie, unlocked)
	if err != nil {
		return err
	}
	http.SetCookie(w, newCookie(r, visitorCookie, value, visitorMaxAge))
	return nil
}

func newCookie(r *http.Request, name, value string, maxAge time.Duration) *http.Cookie {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	}
	if maxAge < 0 {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(maxAge.Seconds())
		c.Expires = time.Now().Add(maxAge)
	}
	return c
}
