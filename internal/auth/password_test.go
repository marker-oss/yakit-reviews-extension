package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" {
		t.Fatal("empty hash")
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	bad, err := VerifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if bad {
		t.Fatal("expected wrong password to fail")
	}
}

func TestHashIsSaltedPerCall(t *testing.T) {
	a, err := HashPassword("same")
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	b, err := HashPassword("same")
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if a == b {
		t.Fatal("expected distinct salts to produce distinct hashes")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("not-a-phc-string", "x"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}
