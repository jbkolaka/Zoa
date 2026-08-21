package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/auth"
	"zoa/backend/internal/httpx"
	"zoa/backend/internal/middleware"
	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// AuthHandler serves registration, login and the current-user endpoint.
type AuthHandler struct {
	users  *store.UserStore
	issuer *auth.TokenIssuer
}

// NewAuthHandler builds the auth handler.
func NewAuthHandler(users *store.UserStore, issuer *auth.TokenIssuer) *AuthHandler {
	return &AuthHandler{users: users, issuer: issuer}
}

// --- request/response shapes (docs/API_CONTRACT.md § Phase 1) ---

type registerRequest struct {
	PhoneNumber string `json:"phone_number"`
	Name        string `json:"name"`
	Password    string `json:"password"`
}

type loginRequest struct {
	PhoneNumber string `json:"phone_number"`
	Password    string `json:"password"`
}

// userResponse is the public shape of a user. It exists so password_hash can
// never be serialised by accident, whatever changes in the model.
type userResponse struct {
	ID            int64     `json:"id"`
	PhoneNumber   string    `json:"phone_number"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	PointsBalance int64     `json:"points_balance"`
	CreatedAt     time.Time `json:"created_at"`
}

func newUserResponse(user *models.User) userResponse {
	return userResponse{
		ID:            user.ID,
		PhoneNumber:   user.PhoneNumber,
		Name:          user.Name,
		Role:          user.Role,
		PointsBalance: user.PointsBalance,
		CreatedAt:     user.CreatedAt,
	}
}

type authResponse struct {
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	User      userResponse `json:"user"`
}

// Register handles POST /auth/register.
//
// New accounts are always role `user`. Elevated roles are assigned by an admin
// (or by demo seeding) — self-service registration must never be able to mint a
// collector who could then verify their own submissions and credit themselves.
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if !bindJSON(c, &req) {
		return
	}

	fields := map[string]string{}

	phone, err := auth.NormalizePhone(req.PhoneNumber)
	if err != nil {
		fields["phone_number"] = err.Error()
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		fields["name"] = "name is required"
	} else if len(name) > 80 {
		fields["name"] = "name must be at most 80 characters"
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		fields["password"] = err.Error()
	}

	if len(fields) > 0 {
		httpx.FailFields(c, http.StatusBadRequest, httpx.CodeValidation,
			"please correct the highlighted fields", fields)
		return
	}

	user, err := h.users.Create(c.Request.Context(), phone, name, passwordHash, models.RoleUser)
	if errors.Is(err, store.ErrPhoneTaken) {
		httpx.Fail(c, http.StatusConflict, httpx.CodeConflict,
			"that phone number is already registered — try signing in instead")
		return
	}
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not create your account")
		return
	}

	h.respondWithToken(c, http.StatusCreated, user)
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}

	// An unparseable number cannot match any stored account, but it is answered
	// with the same generic failure as a wrong password so the endpoint cannot
	// be used to probe which numbers are registered.
	phone, err := auth.NormalizePhone(req.PhoneNumber)
	if err != nil {
		h.failLogin(c)
		return
	}

	user, err := h.users.ByPhone(c.Request.Context(), phone)
	if errors.Is(err, store.ErrNotFound) {
		h.failLogin(c)
		return
	}
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not sign you in")
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		h.failLogin(c)
		return
	}

	h.respondWithToken(c, http.StatusOK, user)
}

// Me handles GET /me — profile plus live points balance.
//
// The balance is read fresh from the database rather than from the token, so it
// reflects points credited since sign-in. This is what the app polls after a
// collector verifies a submission (App Flow §1).
func (h *AuthHandler) Me(c *gin.Context) {
	user := middleware.MustCurrentUser(c)
	c.JSON(http.StatusOK, newUserResponse(user))
}

// failLogin answers with one deliberately vague message. Distinguishing "no such
// account" from "wrong password" would turn the endpoint into a way to enumerate
// which phone numbers are registered.
func (h *AuthHandler) failLogin(c *gin.Context) {
	httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthorized,
		"that phone number and password do not match")
}

func (h *AuthHandler) respondWithToken(c *gin.Context, status int, user *models.User) {
	token, expiresAt, err := h.issuer.Issue(user.ID, user.Role, user.Name)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal,
			"could not issue a session token")
		return
	}

	c.JSON(status, authResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      newUserResponse(user),
	})
}

// bindJSON decodes the request body, reporting a validation error on malformed
// input. Returns false when the handler should stop.
func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeValidation,
			"the request body could not be read as JSON")
		return false
	}
	return true
}
