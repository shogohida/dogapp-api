// Package auth handles password hashing and JWT issuing/verification for
// dogapp-api's self-built login (no external auth provider).
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidToken = errors.New("invalid or expired token")

// tokenTTL is deliberately long-lived (no refresh-token flow) since this is
// a small personal app, not a product with strict session requirements.
const tokenTTL = 30 * 24 * time.Hour

// secret is the HMAC signing key for issued JWTs, resolved once at startup.
var secret = loadSecret()

func loadSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	log.Println("warning: JWT_SECRET is not set - using a random ephemeral secret, so all sessions will be invalidated on restart. Set JWT_SECRET in production.")
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("generate ephemeral JWT secret: %v", err)
	}
	return []byte(hex.EncodeToString(buf))
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type claims struct {
	jwt.RegisteredClaims
}

// IssueToken returns a signed JWT whose subject is userID.
func IssueToken(userID string) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
}

// VerifyToken returns the user id embedded in a valid, unexpired token.
func VerifyToken(tokenString string) (string, error) {
	var c claims
	token, err := jwt.ParseWithClaims(tokenString, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return "", ErrInvalidToken
	}
	return c.Subject, nil
}
