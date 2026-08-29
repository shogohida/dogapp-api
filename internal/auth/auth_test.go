package auth

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "correct-password" {
		t.Fatal("expected the password to actually be hashed")
	}
	if !CheckPassword(hash, "correct-password") {
		t.Fatal("expected the correct password to check out")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("expected the wrong password to fail")
	}
}

func TestIssueAndVerifyToken(t *testing.T) {
	token, err := IssueToken("user-123")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	userID, err := VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("unexpected user id: %s", userID)
	}
}

func TestVerifyTokenRejectsGarbage(t *testing.T) {
	if _, err := VerifyToken("not-a-real-token"); err == nil {
		t.Fatal("expected an error for a garbage token")
	}
}

func TestVerifyTokenRejectsTamperedToken(t *testing.T) {
	token, err := IssueToken("user-123")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	tampered := token[:len(token)-1] + "x"
	if _, err := VerifyToken(tampered); err == nil {
		t.Fatal("expected an error for a tampered token")
	}
}
