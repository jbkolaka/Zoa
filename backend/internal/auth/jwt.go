package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenTTL is how long an issued token stays valid. Long, deliberately: there
// is no refresh-token flow in this scope, and a session expiring mid-demo is a
// worse failure than a long-lived token on a phone.
const TokenTTL = 30 * 24 * time.Hour

// ErrInvalidToken covers every reason a token was rejected. Callers get one
// error so nothing leaks about *why* — expired, wrong signature and malformed
// are indistinguishable to a caller probing the endpoint.
var ErrInvalidToken = errors.New("invalid or expired token")

// Claims is the JWT payload.
//
// Role is carried for convenience only. Authorisation re-reads the user from the
// database on every request, so a role change or a deleted account takes effect
// immediately rather than lingering until the token expires.
type Claims struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
	Name string `json:"name"`
}

// UserID returns the user id from the subject claim.
func (c *Claims) UserID() (int64, error) {
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: subject is not a user id", ErrInvalidToken)
	}
	return id, nil
}

// TokenIssuer signs and verifies tokens with a single HMAC secret.
type TokenIssuer struct {
	secret []byte
}

// NewTokenIssuer builds an issuer. An empty secret is rejected — a zero-length
// HMAC key would make every token forgeable.
func NewTokenIssuer(secret string) (*TokenIssuer, error) {
	if secret == "" {
		return nil, errors.New("jwt secret must not be empty")
	}
	return &TokenIssuer{secret: []byte(secret)}, nil
}

// Issue returns a signed token for the given user.
func (t *TokenIssuer) Issue(userID int64, role, name string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(TokenTTL)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			Issuer:    "zoa-api",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		Role: role,
		Name: name,
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// Verify parses and validates a token, returning its claims.
func (t *TokenIssuer) Verify(tokenString string) (*Claims, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) { return t.secret, nil },
		// Pinning the algorithm is what prevents the classic "alg: none" and
		// RS256→HS256 confusion attacks; without it a caller could choose how
		// their own token gets verified.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer("zoa-api"),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return claims, nil
}

// GenerateDevSecret returns a random secret, used only when APP_ENV=dev and no
// JWT_SECRET was configured. Config refuses to start without an explicit secret
// in any other environment.
//
// A fresh secret each boot means dev tokens do not survive a restart — which is
// the correct trade: it is a loud, obvious "log in again" rather than a
// hardcoded secret that could reach production.
func GenerateDevSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate dev secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
