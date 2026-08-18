package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("secret-password-123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "secret-password-123" {
		t.Fatal("password stored in plaintext")
	}
	if !VerifyPassword(hash, "secret-password-123") {
		t.Fatal("expected valid password to verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestTokenHashIsStableAndNotPlain(t *testing.T) {
	tok, err := NewToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if len(tok) < 32 {
		t.Fatalf("token too short: %d", len(tok))
	}
	h1 := HashToken(tok)
	h2 := HashToken(tok)
	if h1 != h2 {
		t.Fatal("token hash not stable")
	}
	if h1 == tok {
		t.Fatal("token hash equals plain token")
	}
}
