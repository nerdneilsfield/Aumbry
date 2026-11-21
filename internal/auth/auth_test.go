package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestAuthenticateTokenAndDigest(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Settings{
		AdminUser:         "admin",
		AdminPasswordHash: string(hash),
		APITokens:         []string{" amb_tok_one ", "amb_tok_two"},
		CookieSecret:      "0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}

	d1, ok := s.AuthenticateToken("bearer   amb_tok_one ")
	if !ok || d1 != TokenDigest("amb_tok_one") || len(d1) != 64 {
		t.Fatalf("d1=%q ok=%v, want normalized 64-char digest", d1, ok)
	}
	d2, ok := s.AuthenticateToken("Bearer amb_tok_two")
	if !ok || d2 != TokenDigest("amb_tok_two") || d2 == d1 {
		t.Fatalf("d2=%q ok=%v", d2, ok)
	}
	for _, bad := range []string{"", "Bearer ", "Bearer   ", "Basic abc", "Bearer wrong"} {
		if _, ok := s.AuthenticateToken(bad); ok {
			t.Errorf("AuthenticateToken(%q) succeeded, want false", bad)
		}
	}
}
