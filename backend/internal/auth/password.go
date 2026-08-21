// Package auth handles password hashing, phone-number normalisation and JWT
// issue/verify. It has no HTTP or database dependencies so it stays testable in
// isolation.
package auth

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is deliberately modest: this is a demo-scoped platform
// whose users type passwords on a phone keypad. bcrypt's work factor, not
// length alone, is what makes the stored hash expensive to attack.
const MinPasswordLength = 6

// bcrypt cost. 10 is bcrypt's default — roughly 60ms per hash, which is slow
// enough to matter to an attacker and fast enough not to be felt at login.
const bcryptCost = 10

// HashPassword returns a bcrypt hash for storage in users.password_hash.
func HashPassword(plain string) (string, error) {
	if len(plain) < MinPasswordLength {
		return "", fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}

	// bcrypt silently truncates at 72 bytes, so a longer password would appear
	// to work while ignoring everything past the limit.
	if len(plain) > 72 {
		return "", fmt.Errorf("password must be at most 72 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword reports whether plain matches the stored hash.
//
// It always runs the full bcrypt comparison, so the time taken does not reveal
// whether the failure was a wrong password or a malformed hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// --- phone numbers ---

// Kenyan mobile numbers are 9 digits after the country code, always starting
// with 7 (Safaricom/Airtel) or 1 (newer Airtel/Telkom ranges).
var kenyanMobile = regexp.MustCompile(`^\+254[71]\d{8}$`)

var nonDigits = regexp.MustCompile(`\D`)

// NormalizePhone converts any of the formats a Kenyan user might type into the
// single canonical form `+254XXXXXXXXX`.
//
// This matters because users.phone_number is UNIQUE and doubles as the login
// identifier: without normalisation, "0712345678" and "+254712345678" would
// create two accounts for the same person, and the second registration would
// succeed instead of reporting a conflict.
//
// Accepted inputs:
//
//	0712345678      →  +254712345678
//	712345678       →  +254712345678
//	254712345678    →  +254712345678
//	+254 712 345678 →  +254712345678
func NormalizePhone(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("phone number is required")
	}

	// Strip spaces, dashes, brackets — anything that is not a digit. The leading
	// "+" carries no information once we know the country.
	digits := nonDigits.ReplaceAllString(trimmed, "")
	if digits == "" {
		return "", fmt.Errorf("phone number must contain digits")
	}

	var national string
	switch {
	case strings.HasPrefix(digits, "254"):
		national = strings.TrimPrefix(digits, "254")
	case strings.HasPrefix(digits, "0"):
		national = strings.TrimPrefix(digits, "0")
	default:
		national = digits
	}

	candidate := "+254" + national
	if !kenyanMobile.MatchString(candidate) {
		return "", fmt.Errorf("not a valid Kenyan mobile number")
	}
	return candidate, nil
}
