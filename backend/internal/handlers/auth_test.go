package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// doJSON issues a request with an optional JSON body and bearer token.
func doJSON(t *testing.T, router *gin.Engine, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// registerUser creates an account and returns its token and user object.
func registerUser(t *testing.T, router *gin.Engine, phone, name string) (string, map[string]any) {
	t.Helper()

	recorder := doJSON(t, router, http.MethodPost, "/auth/register", map[string]any{
		"phone_number": phone,
		"name":         name,
		"password":     "zoa1234",
	}, "")

	if recorder.Code != http.StatusCreated {
		t.Fatalf("register %s: status %d, body %s", phone, recorder.Code, recorder.Body)
	}

	body := decode(t, recorder)
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("register %s returned no token: %v", phone, body)
	}
	user, _ := body["user"].(map[string]any)
	return token, user
}

func TestRegisterCreatesUserWithZeroBalance(t *testing.T) {
	router := newTestRouter(t)

	_, user := registerUser(t, router, "+254712345678", "Amina Wanjiru")

	if user["name"] != "Amina Wanjiru" {
		t.Errorf("name = %v, want Amina Wanjiru", user["name"])
	}
	if user["role"] != "user" {
		t.Errorf("role = %v, want user", user["role"])
	}
	if balance, _ := user["points_balance"].(float64); balance != 0 {
		t.Errorf("points_balance = %v, want 0", user["points_balance"])
	}
	if user["id"] == nil {
		t.Error("id is missing")
	}
}

// TestRegisterNeverLeaksPasswordHash guards the one field that must never cross
// the wire, whatever else changes in the model.
func TestRegisterNeverLeaksPasswordHash(t *testing.T) {
	router := newTestRouter(t)
	recorder := doJSON(t, router, http.MethodPost, "/auth/register", map[string]any{
		"phone_number": "+254712345678",
		"name":         "Amina Wanjiru",
		"password":     "zoa1234",
	}, "")

	body := recorder.Body.String()
	for _, forbidden := range []string{"password_hash", "$2a$", "$2b$", "zoa1234"} {
		if bytes.Contains([]byte(body), []byte(forbidden)) {
			t.Errorf("response contains %q: %s", forbidden, body)
		}
	}
}

// TestRegisterNormalizesPhoneNumber is what stops one person holding two
// accounts written two different ways.
func TestRegisterNormalizesPhoneNumber(t *testing.T) {
	router := newTestRouter(t)

	_, user := registerUser(t, router, "0712345678", "Amina Wanjiru")

	if user["phone_number"] != "+254712345678" {
		t.Errorf("phone_number = %v, want +254712345678", user["phone_number"])
	}
}

// TestRegisterRejectsDuplicateAcrossFormats registers with one format then
// retries with another spelling of the same number; the second must conflict.
func TestRegisterRejectsDuplicateAcrossFormats(t *testing.T) {
	router := newTestRouter(t)
	registerUser(t, router, "+254712345678", "Amina Wanjiru")

	recorder := doJSON(t, router, http.MethodPost, "/auth/register", map[string]any{
		"phone_number": "0712345678", // same number, different format
		"name":         "Someone Else",
		"password":     "zoa1234",
	}, "")

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", recorder.Code, recorder.Body)
	}
	detail, _ := decode(t, recorder)["error"].(map[string]any)
	if detail["code"] != "conflict" {
		t.Errorf("error.code = %v, want conflict", detail["code"])
	}
}

func TestRegisterValidationReportsFields(t *testing.T) {
	router := newTestRouter(t)

	recorder := doJSON(t, router, http.MethodPost, "/auth/register", map[string]any{
		"phone_number": "not-a-number",
		"name":         "",
		"password":     "123",
	}, "")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", recorder.Code, recorder.Body)
	}

	detail, ok := decode(t, recorder)["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope: %s", recorder.Body)
	}
	if detail["code"] != "validation_error" {
		t.Errorf("error.code = %v, want validation_error", detail["code"])
	}

	fields, ok := detail["fields"].(map[string]any)
	if !ok {
		t.Fatalf("no fields map: %s", recorder.Body)
	}
	for _, field := range []string{"phone_number", "name", "password"} {
		if fields[field] == nil {
			t.Errorf("fields is missing %q: %v", field, fields)
		}
	}
}

// TestRegisterCannotSelfAssignRole is a privilege-escalation guard: a collector
// can verify submissions and therefore credit points, so registration must never
// honour a client-supplied role.
func TestRegisterCannotSelfAssignRole(t *testing.T) {
	router := newTestRouter(t)

	recorder := doJSON(t, router, http.MethodPost, "/auth/register", map[string]any{
		"phone_number":   "+254712345678",
		"name":           "Sneaky",
		"password":       "zoa1234",
		"role":           "admin",
		"points_balance": 99999,
	}, "")

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", recorder.Code, recorder.Body)
	}

	user, _ := decode(t, recorder)["user"].(map[string]any)
	if user["role"] != "user" {
		t.Errorf("role = %v, want user — role was self-assigned", user["role"])
	}
	if balance, _ := user["points_balance"].(float64); balance != 0 {
		t.Errorf("points_balance = %v, want 0 — balance was self-assigned", user["points_balance"])
	}
}

func TestLoginSucceedsWithAnyPhoneFormat(t *testing.T) {
	router := newTestRouter(t)
	registerUser(t, router, "+254712345678", "Amina Wanjiru")

	for _, format := range []string{"+254712345678", "0712345678", "712345678", "254712345678"} {
		recorder := doJSON(t, router, http.MethodPost, "/auth/login", map[string]any{
			"phone_number": format,
			"password":     "zoa1234",
		}, "")

		if recorder.Code != http.StatusOK {
			t.Errorf("login with %q: status %d, body %s", format, recorder.Code, recorder.Body)
			continue
		}
		if token, _ := decode(t, recorder)["token"].(string); token == "" {
			t.Errorf("login with %q returned no token", format)
		}
	}
}

// TestLoginFailuresAreIndistinguishable checks an unknown number and a wrong
// password produce the same status and message, so the endpoint cannot be used
// to discover which numbers are registered.
func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	router := newTestRouter(t)
	registerUser(t, router, "+254712345678", "Amina Wanjiru")

	wrongPassword := doJSON(t, router, http.MethodPost, "/auth/login", map[string]any{
		"phone_number": "+254712345678",
		"password":     "definitely-wrong",
	}, "")

	unknownNumber := doJSON(t, router, http.MethodPost, "/auth/login", map[string]any{
		"phone_number": "+254799999999",
		"password":     "zoa1234",
	}, "")

	if wrongPassword.Code != http.StatusUnauthorized {
		t.Errorf("wrong password: status %d, want 401", wrongPassword.Code)
	}
	if unknownNumber.Code != http.StatusUnauthorized {
		t.Errorf("unknown number: status %d, want 401", unknownNumber.Code)
	}
	if wrongPassword.Body.String() != unknownNumber.Body.String() {
		t.Errorf("failure responses differ:\n  wrong password: %s\n  unknown number: %s",
			wrongPassword.Body, unknownNumber.Body)
	}
}

func TestMeRequiresAuthentication(t *testing.T) {
	router := newTestRouter(t)

	cases := map[string]string{
		"no token":      "",
		"garbage token": "not-a-real-token",
		"wrong scheme":  "Basic abc",
		"empty bearer":  " ",
	}

	for name, token := range cases {
		recorder := doJSON(t, router, http.MethodGet, "/me", nil, token)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401; body %s", name, recorder.Code, recorder.Body)
			continue
		}
		detail, _ := decode(t, recorder)["error"].(map[string]any)
		if detail["code"] != "unauthorized" {
			t.Errorf("%s: error.code = %v, want unauthorized", name, detail["code"])
		}
	}
}

func TestMeReturnsProfileAndBalance(t *testing.T) {
	router := newTestRouter(t)
	token, registered := registerUser(t, router, "+254712345678", "Amina Wanjiru")

	recorder := doJSON(t, router, http.MethodGet, "/me", nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}

	me := decode(t, recorder)
	if me["id"] != registered["id"] {
		t.Errorf("id = %v, want %v", me["id"], registered["id"])
	}
	if me["phone_number"] != "+254712345678" {
		t.Errorf("phone_number = %v", me["phone_number"])
	}
	if balance, _ := me["points_balance"].(float64); balance != 0 {
		t.Errorf("points_balance = %v, want 0", me["points_balance"])
	}
}

// TestMeReadsBalanceFromDatabase proves the balance is read live rather than
// taken from the token — this is what lets the app see points appear after a
// collector verifies a submission, without re-authenticating.
func TestMeReadsBalanceFromDatabase(t *testing.T) {
	router, conn := newTestRouterWithDB(t)
	token, user := registerUser(t, router, "+254712345678", "Amina Wanjiru")

	userID := int64(user["id"].(float64))
	if _, err := conn.Exec(
		`UPDATE users SET points_balance = 340 WHERE id = ?`, userID,
	); err != nil {
		t.Fatalf("update balance: %v", err)
	}

	recorder := doJSON(t, router, http.MethodGet, "/me", nil, token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if balance, _ := decode(t, recorder)["points_balance"].(float64); balance != 340 {
		t.Errorf("points_balance = %v, want 340 — balance is not read live",
			decode(t, recorder)["points_balance"])
	}
}

// TestMeRejectsTokenForDeletedUser covers the reason the middleware re-reads the
// user: a validly-signed token for a gone account must stop working immediately.
func TestMeRejectsTokenForDeletedUser(t *testing.T) {
	router, conn := newTestRouterWithDB(t)
	token, user := registerUser(t, router, "+254712345678", "Amina Wanjiru")

	userID := int64(user["id"].(float64))
	if _, err := conn.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	recorder := doJSON(t, router, http.MethodGet, "/me", nil, token)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body %s", recorder.Code, recorder.Body)
	}
}

// TestMeSeesRoleChangeImmediately is the other half of that trade-off: an admin
// promoting a user must not have to wait 30 days for the token to expire.
func TestMeSeesRoleChangeImmediately(t *testing.T) {
	router, conn := newTestRouterWithDB(t)
	token, user := registerUser(t, router, "+254712345678", "Amina Wanjiru")

	userID := int64(user["id"].(float64))
	if _, err := conn.Exec(
		`UPDATE users SET role = 'collector' WHERE id = ?`, userID,
	); err != nil {
		t.Fatalf("update role: %v", err)
	}

	recorder := doJSON(t, router, http.MethodGet, "/me", nil, token)
	if role := decode(t, recorder)["role"]; role != "collector" {
		t.Errorf("role = %v, want collector — role is stale from the token", role)
	}
}

func TestMalformedJSONBodyIsRejected(t *testing.T) {
	router := newTestRouter(t)

	request := httptest.NewRequest(http.MethodPost, "/auth/register",
		bytes.NewReader([]byte(`{"phone_number": broken`)))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", recorder.Code, recorder.Body)
	}
	detail, _ := decode(t, recorder)["error"].(map[string]any)
	if detail["code"] != "validation_error" {
		t.Errorf("error.code = %v, want validation_error", detail["code"])
	}
}
