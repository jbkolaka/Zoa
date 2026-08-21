// Package middleware holds cross-cutting HTTP concerns.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/auth"
	"zoa/backend/internal/httpx"
	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// contextUserKey is where the authenticated user is stashed for handlers.
const contextUserKey = "zoa.user"

// Auth builds the authentication middleware.
//
// It verifies the bearer token, then loads the user from the database. The
// second step costs one indexed query and buys correctness: a role change or a
// deleted account applies on the next request instead of lingering for the
// token's whole 30-day life.
func Auth(issuer *auth.TokenIssuer, users *store.UserStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
				"a bearer token is required")
			return
		}

		claims, err := issuer.Verify(token)
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
				"your session is no longer valid — please sign in again")
			return
		}

		userID, err := claims.UserID()
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
				"your session is no longer valid — please sign in again")
			return
		}

		user, err := users.ByID(c.Request.Context(), userID)
		if errors.Is(err, store.ErrNotFound) {
			// Valid signature, but the account is gone.
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
				"this account no longer exists")
			return
		}
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
				"could not load your account")
			return
		}

		c.Set(contextUserKey, user)
		c.Next()
	}
}

// DenyRole blocks a set of roles from a route that is otherwise open to any
// signed-in account.
//
// The inverse of [RequireRole], and deliberately not expressible with it: that
// helper appends admin to every allow-list, so "everyone except a collector"
// written as an allow-list would have to name each remaining role and would
// silently admit any role added later. This names the exclusion instead.
//
// Admin is *not* denied here for the same reason it is implicitly allowed
// there — one admin login has to be able to drive every flow during a demo.
func DenyRole(denied ...string) gin.HandlerFunc {
	blocked := make(map[string]bool, len(denied))
	for _, role := range denied {
		blocked[role] = true
	}
	// A denial must never lock the demo account out of a flow it has to walk.
	delete(blocked, models.RoleAdmin)

	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok {
			// Auth middleware must run first; reaching here is a wiring bug.
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
				"authentication is required")
			return
		}

		if blocked[user.Role] {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden,
				"your account does not have access to this action")
			return
		}
		c.Next()
	}
}

// RequireRole gates a route to a set of roles. Admin is appended implicitly, so
// one admin account can drive every flow during a demo without also holding a
// collector and a partner_staff login.
func RequireRole(allowed ...string) gin.HandlerFunc {
	permitted := make(map[string]bool, len(allowed)+1)
	for _, role := range allowed {
		permitted[role] = true
	}
	permitted[models.RoleAdmin] = true

	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok {
			// Auth middleware must run first; reaching here is a wiring bug.
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
				"authentication is required")
			return
		}

		if !permitted[user.Role] {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden,
				"your account does not have access to this action")
			return
		}
		c.Next()
	}
}

// CurrentUser returns the authenticated user attached by [Auth].
func CurrentUser(c *gin.Context) (*models.User, bool) {
	value, exists := c.Get(contextUserKey)
	if !exists {
		return nil, false
	}
	user, ok := value.(*models.User)
	return user, ok
}

// MustCurrentUser returns the authenticated user, and is safe to call from any
// handler mounted behind [Auth].
func MustCurrentUser(c *gin.Context) *models.User {
	user, ok := CurrentUser(c)
	if !ok {
		panic("middleware.MustCurrentUser called on a route without Auth middleware")
	}
	return user
}

// bearerToken extracts the token from an `Authorization: Bearer …` header.
func bearerToken(c *gin.Context) (string, bool) {
	header := c.GetHeader("Authorization")
	if header == "" {
		return "", false
	}

	// Scheme is case-insensitive per RFC 7235.
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}

	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}
