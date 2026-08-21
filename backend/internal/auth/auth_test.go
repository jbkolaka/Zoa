package auth

import (
	"strings"
	"testing"
	"time"
)

// TestNormalizePhoneAcceptsKenyanFormats is the important one: users.phone_number
// is UNIQUE and is the login identifier, so every way of writing the same number
// must collapse to one canonical string. If it does not, one person can hold two
// accounts and "already registered" stops working.
func TestNormalizePhoneAcceptsKenyanFormats(t *testing.T) {
	const want = "+254712345678"

	inputs := []string{
		"+254712345678",
		"254712345678",
		"0712345678",
		"712345678",
		"+254 712 345 678",
		"0712 345 678",
		"+254-712-345-678",
		"  0712345678  ",
		"(+254) 712 345678",
	}

	for _, input := range inputs {
		got, err := NormalizePhone(input)
		if err != nil {
			t.Errorf("NormalizePhone(%q) returned error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestNormalizePhoneAcceptsOtherValidPrefixes covers the 01x ranges alongside 07x.
func TestNormalizePhoneAcceptsOtherValidPrefixes(t *testing.T) {
	cases := map[string]string{
		"0110000000":    "+254110000000",
		"+254100000000": "+254100000000",
		"0799999999":    "+254799999999",
	}

	for input, want := range cases {
		got, err := NormalizePhone(input)
		if err != nil {
			t.Errorf("NormalizePhone(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizePhoneRejectsInvalid(t *testing.T) {
	invalid := []string{
		"",              // empty
		"   ",           // whitespace only
		"abcdefghi",     // no digits
		"071234567",     // too short
		"07123456789",   // too long
		"0812345678",    // 08 is not a valid Kenyan mobile prefix
		"0612345678",    // 06 likewise
		"+1234567890",   // wrong country
		"+254212345678", // Nairobi landline, not a mobile
		"254",           // country code alone
	}

	for _, input := range invalid {
		if got, err := NormalizePhone(input); err == nil {
			t.Errorf("NormalizePhone(%q) = %q, want an error", input, got)
		}
	}
}

func TestHashAndCheckPassword(t *testing.T) {
	const password = "zoa1234"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// A bcrypt hash must not resemble the input.
	if strings.Contains(hash, password) {
		t.Fatal("hash contains the plaintext password")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("hash %q does not look like bcrypt output", hash)
	}

	if !CheckPassword(hash, password) {
		t.Error("CheckPassword rejected the correct password")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Error("CheckPassword accepted an incorrect password")
	}
	if CheckPassword("not-a-hash", password) {
		t.Error("CheckPassword accepted a malformed hash")
	}
}

// TestHashPasswordSaltsEachHash confirms two users with the same password do not
// share a stored hash — otherwise one cracked hash would expose several accounts.
func TestHashPasswordSaltsEachHash(t *testing.T) {
	first, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	second, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if first == second {
		t.Error("identical passwords produced identical hashes; salting is not working")
	}
}

func TestHashPasswordRejectsTooShort(t *testing.T) {
	if _, err := HashPassword("12345"); err == nil {
		t.Error("accepted a 5-character password")
	}
}

// TestHashPasswordRejectsOverLongInput matters because bcrypt silently ignores
// everything past 72 bytes — a longer password would appear accepted while most
// of it was discarded.
func TestHashPasswordRejectsOverLongInput(t *testing.T) {
	if _, err := HashPassword(strings.Repeat("a", 73)); err == nil {
		t.Error("accepted a 73-byte password, which bcrypt would truncate")
	}
}

func TestIssueAndVerifyToken(t *testing.T) {
	issuer, err := NewTokenIssuer("a-test-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	token, expiresAt, err := issuer.Issue(42, "collector", "Joseph Kariuki")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if !expiresAt.After(time.Now()) {
		t.Errorf("expiresAt %v is not in the future", expiresAt)
	}

	claims, err := issuer.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	userID, err := claims.UserID()
	if err != nil {
		t.Fatalf("UserID: %v", err)
	}
	if userID != 42 {
		t.Errorf("UserID() = %d, want 42", userID)
	}
	if claims.Role != "collector" {
		t.Errorf("Role = %q, want collector", claims.Role)
	}
}

// TestVerifyRejectsForeignSecret is the core guarantee: a token signed by anyone
// else must not authenticate.
func TestVerifyRejectsForeignSecret(t *testing.T) {
	mine, err := NewTokenIssuer("my-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	theirs, err := NewTokenIssuer("someone-elses-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	forged, _, err := theirs.Issue(1, "admin", "Attacker")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	if _, err := mine.Verify(forged); err == nil {
		t.Fatal("accepted a token signed with a different secret")
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	issuer, err := NewTokenIssuer("a-test-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	for _, token := range []string{"", "not.a.token", "a.b.c", "Bearer x"} {
		if _, err := issuer.Verify(token); err == nil {
			t.Errorf("Verify(%q) succeeded, want an error", token)
		}
	}
}

// TestVerifyRejectsUnsignedToken covers the "alg: none" attack — a token whose
// header claims no signature is required. WithValidMethods is what blocks it.
func TestVerifyRejectsUnsignedToken(t *testing.T) {
	issuer, err := NewTokenIssuer("a-test-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	// {"alg":"none","typ":"JWT"} . {"sub":"1","role":"admin"} . <empty>
	unsigned := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiIxIiwicm9sZSI6ImFkbWluIiwiaXNzIjoiem9hLWFwaSJ9."

	if _, err := issuer.Verify(unsigned); err == nil {
		t.Fatal("accepted an unsigned token")
	}
}

func TestNewTokenIssuerRejectsEmptySecret(t *testing.T) {
	if _, err := NewTokenIssuer(""); err == nil {
		t.Error("accepted an empty signing secret")
	}
}

func TestGenerateDevSecretIsRandom(t *testing.T) {
	first, err := GenerateDevSecret()
	if err != nil {
		t.Fatalf("GenerateDevSecret: %v", err)
	}
	second, err := GenerateDevSecret()
	if err != nil {
		t.Fatalf("GenerateDevSecret: %v", err)
	}

	if first == second {
		t.Error("two dev secrets were identical")
	}
	if len(first) < 32 {
		t.Errorf("dev secret is only %d characters", len(first))
	}
}
