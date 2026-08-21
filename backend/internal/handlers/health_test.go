package handlers_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/auth"
	"zoa/backend/internal/config"
	"zoa/backend/internal/db"
	"zoa/backend/internal/handlers"
)

// newTestRouter builds a router over a fresh migrated database.
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	router, _ := newTestRouterWithDB(t)
	return router
}

// newTestRouterWithDB also returns the connection, for tests that need to seed
// or inspect rows directly.
func newTestRouterWithDB(t *testing.T) (*gin.Engine, *sql.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	if _, err := db.Migrate(conn); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	issuer, err := auth.NewTokenIssuer("test-secret-not-used-outside-tests")
	if err != nil {
		t.Fatalf("auth.NewTokenIssuer: %v", err)
	}

	router := handlers.NewRouter(conn, &config.Config{
		Env:         "dev",
		Port:        "8080",
		DBPath:      "test.db",
		CORSOrigins: []string{"*"},
	}, issuer)

	return router, conn
}

// do issues a request against the router and returns the recorder.
func do(t *testing.T, router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

// decode unmarshals a JSON body, failing the test if it is not valid JSON.
func decode(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return body
}

// TestHealthReportsServiceAndDatabase covers the Phase 0 deliverable: the
// endpoint reports 200 and confirms the database is live and migrated.
func TestHealthReportsServiceAndDatabase(t *testing.T) {
	recorder := do(t, newTestRouter(t), http.MethodGet, "/health")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	body := decode(t, recorder)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["service"] != "zoa-api" {
		t.Errorf("service = %v, want zoa-api", body["service"])
	}

	database, ok := body["database"].(map[string]any)
	if !ok {
		t.Fatalf("database block missing from %v", body)
	}
	if database["connected"] != true {
		t.Errorf("database.connected = %v, want true", database["connected"])
	}
	if applied, _ := database["migrations_applied"].(float64); applied < 2 {
		t.Errorf("migrations_applied = %v, want at least 2", database["migrations_applied"])
	}
}

// TestMetaReturnsFullTaxonomyWithRates checks the client's material selector has
// a complete, rate-carrying taxonomy to render (TRD §2.6, FR-9).
func TestMetaReturnsFullTaxonomyWithRates(t *testing.T) {
	recorder := do(t, newTestRouter(t), http.MethodGet, "/meta")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	body := decode(t, recorder)
	materials, ok := body["materials"].([]any)
	if !ok {
		t.Fatalf("materials missing from %v", body)
	}
	if len(materials) != 14 {
		t.Errorf("got %d materials, want 14", len(materials))
	}

	// Every entry needs a key, a group and a usable rate.
	groups := map[string]bool{}
	for _, raw := range materials {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("material entry is not an object: %v", raw)
		}
		key, _ := entry["key"].(string)
		if key == "" {
			t.Errorf("material with no key: %v", entry)
		}
		group, _ := entry["group"].(string)
		if group == "" {
			t.Errorf("material %q has no group", key)
		}
		groups[group] = true

		if points, _ := entry["points_per_kg"].(float64); points <= 0 {
			t.Errorf("material %q has rate %v, want > 0", key, entry["points_per_kg"])
		}
	}

	for _, group := range []string{"plastics", "paper", "glass", "metal", "organic"} {
		if !groups[group] {
			t.Errorf("taxonomy is missing the %q group", group)
		}
	}
}

// TestUnknownRouteUsesErrorEnvelope ensures even a 404 arrives in the documented
// shape, so the client never has to special-case gin's default body.
func TestUnknownRouteUsesErrorEnvelope(t *testing.T) {
	recorder := do(t, newTestRouter(t), http.MethodGet, "/does-not-exist")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}

	detail, ok := decode(t, recorder)["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope in %q", recorder.Body.String())
	}
	if detail["code"] != "not_found" {
		t.Errorf("error.code = %v, want not_found", detail["code"])
	}
	if detail["message"] == "" {
		t.Error("error.message is empty")
	}
}

// TestWrongMethodReportsMethodNotAllowed guards the HandleMethodNotAllowed
// setting; without it gin reports 404 and hides the real mistake.
func TestWrongMethodReportsMethodNotAllowed(t *testing.T) {
	recorder := do(t, newTestRouter(t), http.MethodPost, "/health")

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}

	detail, ok := decode(t, recorder)["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error envelope in %q", recorder.Body.String())
	}
	if detail["code"] != "method_not_allowed" {
		t.Errorf("error.code = %v, want method_not_allowed", detail["code"])
	}
}

// TestCORSPreflightIsAnswered covers browser-based clients (Flutter web, a
// future admin view), which cannot call the API without this.
func TestCORSPreflightIsAnswered(t *testing.T) {
	router := newTestRouter(t)
	recorder := httptest.NewRecorder()

	request := httptest.NewRequest(http.MethodOptions, "/health", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Error("Access-Control-Allow-Origin header is missing")
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Access-Control-Allow-Headers header is missing")
	}
}
