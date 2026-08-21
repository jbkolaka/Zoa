package middleware_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/auth"
	"zoa/backend/internal/db"
	"zoa/backend/internal/middleware"
	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// harness wires the middleware onto a throwaway route so role gating can be
// tested before Phases 2–5 mount their real endpoints.
type harness struct {
	router *gin.Engine
	users  *store.UserStore
	issuer *auth.TokenIssuer
	conn   *sql.DB
}

func newHarness(t *testing.T, allowed ...string) *harness {
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

	issuer, err := auth.NewTokenIssuer("test-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	users := store.NewUserStore(conn)
	router := gin.New()

	handlers := []gin.HandlerFunc{middleware.Auth(issuer, users)}
	if len(allowed) > 0 {
		handlers = append(handlers, middleware.RequireRole(allowed...))
	}
	handlers = append(handlers, func(c *gin.Context) {
		user := middleware.MustCurrentUser(c)
		c.JSON(http.StatusOK, gin.H{"id": user.ID, "role": user.Role})
	})

	router.GET("/guarded", handlers...)

	return &harness{router: router, users: users, issuer: issuer, conn: conn}
}

// tokenFor creates a user with the given role and returns a valid token.
func (h *harness) tokenFor(t *testing.T, phone, role string) string {
	t.Helper()

	hash, err := auth.HashPassword("zoa1234")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	user, err := h.users.Create(context.Background(), phone, "Test User", hash, role)
	if err != nil {
		t.Fatalf("create %s: %v", role, err)
	}

	token, _, err := h.issuer.Issue(user.ID, user.Role, user.Name)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return token
}

func (h *harness) get(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, request)
	return recorder
}

func TestAuthAllowsAnyAuthenticatedRole(t *testing.T) {
	h := newHarness(t)

	roles := []string{
		models.RoleUser,
		models.RoleCollector,
		models.RolePartnerStaff,
		models.RoleAdmin,
	}

	for i, role := range roles {
		phone := []string{"+254712000011", "+254712000012", "+254712000013", "+254712000014"}[i]
		recorder := h.get(t, h.tokenFor(t, phone, role))
		if recorder.Code != http.StatusOK {
			t.Errorf("role %s: status %d, want 200; body %s", role, recorder.Code, recorder.Body)
		}
	}
}

func TestRequireRoleBlocksInsufficientRole(t *testing.T) {
	h := newHarness(t, models.RoleCollector)

	recorder := h.get(t, h.tokenFor(t, "+254712000021", models.RoleUser))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", recorder.Code, recorder.Body)
	}
}

func TestRequireRoleAllowsNamedRole(t *testing.T) {
	h := newHarness(t, models.RoleCollector)

	recorder := h.get(t, h.tokenFor(t, "+254712000022", models.RoleCollector))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body)
	}
}

// TestRequireRoleAlwaysAllowsAdmin covers the deliberate rule that admin
// inherits every capability, so one demo login can drive the whole flow.
func TestRequireRoleAlwaysAllowsAdmin(t *testing.T) {
	for _, gated := range []string{models.RoleCollector, models.RolePartnerStaff} {
		h := newHarness(t, gated)

		recorder := h.get(t, h.tokenFor(t, "+254712000031", models.RoleAdmin))
		if recorder.Code != http.StatusOK {
			t.Errorf("admin against %s-gated route: status %d, want 200", gated, recorder.Code)
		}
	}
}

// TestRequireRoleStillRequiresAuthentication confirms the gate reports 401 (not
// 403) when there is no identity at all — the client distinguishes "sign in" from
// "you cannot do this".
func TestRequireRoleStillRequiresAuthentication(t *testing.T) {
	h := newHarness(t, models.RoleAdmin)

	recorder := h.get(t, "")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body %s", recorder.Code, recorder.Body)
	}
}

// TestRoleIsReadFromDatabaseNotToken is the payoff for re-reading the user: a
// token minted while the account was a collector must stop passing a
// collector-gated route the moment the role is revoked.
func TestRoleIsReadFromDatabaseNotToken(t *testing.T) {
	h := newHarness(t, models.RoleCollector)

	token := h.tokenFor(t, "+254712000041", models.RoleCollector)

	if recorder := h.get(t, token); recorder.Code != http.StatusOK {
		t.Fatalf("collector was blocked before demotion: status %d", recorder.Code)
	}

	user, err := h.users.ByPhone(context.Background(), "+254712000041")
	if err != nil {
		t.Fatalf("ByPhone: %v", err)
	}
	// An out-of-band admin change, which is exactly the case under test.
	if _, err := h.conn.Exec(
		`UPDATE users SET role = 'user' WHERE id = ?`, user.ID,
	); err != nil {
		t.Fatalf("demote: %v", err)
	}

	if recorder := h.get(t, token); recorder.Code != http.StatusForbidden {
		t.Errorf("after demotion: status %d, want 403 — role came from the token",
			recorder.Code)
	}
}
