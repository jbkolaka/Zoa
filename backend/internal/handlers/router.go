package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"zoa/backend/internal/auth"
	"zoa/backend/internal/classify"
	"zoa/backend/internal/config"
	"zoa/backend/internal/httpx"
	"zoa/backend/internal/middleware"
	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// Version is the API version reported by /health.
const Version = "0.4.0"

// NewRouter builds the gin engine with middleware and all currently-live routes.
//
// Route paths deliberately carry no /api/v1 prefix: docs/05_App_Flow.md §2
// specifies bare paths (/auth/register, /me, /submissions …) and the client is
// built against that list verbatim.
func NewRouter(conn *sql.DB, cfg *config.Config, issuer *auth.TokenIssuer) *gin.Engine {
	if !cfg.IsDev() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), corsMiddleware(cfg.CORSOrigins))

	// Off by default in gin; without it a wrong-method request falls through to
	// NoRoute and reports 404, which hides the real mistake from the client.
	r.HandleMethodNotAllowed = true

	// Unknown routes still answer in the standard error envelope, so the client
	// never has to special-case gin's default 404 body.
	r.NoRoute(func(c *gin.Context) {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeNotFound, "no such endpoint: "+c.Request.URL.Path)
	})
	r.NoMethod(func(c *gin.Context) {
		httpx.Fail(c, http.StatusMethodNotAllowed, httpx.CodeMethodNotAllow, "method not allowed for "+c.Request.URL.Path)
	})

	users := store.NewUserStore(conn)
	submissions := store.NewSubmissionStore(conn)
	vouchers := store.NewVoucherStore(conn)
	redemptions := store.NewRedemptionStore(conn)
	admin := store.NewAdminStore(conn)
	catalog := store.NewCatalogStore(conn)

	health := NewHealthHandler(conn, cfg.Env, Version, time.Now())
	authHandler := NewAuthHandler(users, issuer)
	submissionHandler := NewSubmissionHandler(submissions)
	classifyHandler := NewClassifyHandler(newClassifier(cfg), cfg.ClassifyTimeout)
	voucherHandler := NewVoucherHandler(vouchers, users)
	redemptionHandler := NewRedemptionHandler(redemptions)
	adminHandler := NewAdminHandler(admin)
	catalogAdminHandler := NewCatalogAdminHandler(catalog)

	// Applied per-route rather than globally, so the public endpoints below
	// cannot accidentally inherit it and the auth-required set stays explicit.
	requireAuth := middleware.Auth(issuer, users)
	requireCollector := middleware.RequireRole(models.RoleCollector)
	requirePartner := middleware.RequireRole(models.RolePartnerStaff)
	requireAdmin := middleware.RequireRole(models.RoleAdmin)
	// A collector verifies what other people hand over and does not log its own
	// recycling, so the app gives that account no Recycle tab. Enforced here too:
	// hiding a tab is a client decision, and the client is never the authority.
	denyCollector := middleware.DenyRole(models.RoleCollector)

	// --- Phase 0: platform ---
	r.GET("/health", health.Health)
	r.GET("/meta", health.Meta)

	// --- Phase 1: auth & user core ---
	r.POST("/auth/register", authHandler.Register)
	r.POST("/auth/login", authHandler.Login)
	r.GET("/me", requireAuth, authHandler.Me)

	// --- Phase 2: submission flow ---
	r.POST("/submissions", requireAuth, denyCollector, submissionHandler.Create)
	// Declared before the :id route so "/submissions" and "/submissions/12" do
	// not compete for the same pattern.
	r.GET("/submissions", requireAuth, submissionHandler.List)
	r.GET("/submissions/:id", requireAuth, submissionHandler.Get)
	// The crediting step. Collector or admin only — a user must not be able to
	// verify their own submission and award themselves points.
	r.PATCH("/submissions/:id/verify", requireAuth, requireCollector, submissionHandler.Verify)

	// --- Phase 2.5: AI material classification ---
	// Registered before the /:id patterns to keep the static segment visibly
	// ahead of the wildcard, even though the differing methods mean they cannot
	// actually collide.
	//
	// Denied to collectors alongside the create route it feeds: this is the
	// suggestion step inside the submit flow, and an account that cannot submit
	// has nothing to classify.
	r.POST("/submissions/classify", requireAuth, denyCollector, classifyHandler.Classify)

	// --- Phase 3: voucher catalogue ---
	r.GET("/vouchers", requireAuth, voucherHandler.List)
	r.GET("/vouchers/:id", requireAuth, voucherHandler.Get)
	// Not in App Flow §2's list, but the catalogue needs a partner filter control
	// and deriving one from the voucher list would miss partners whose offers are
	// all currently out of stock.
	r.GET("/partners", requireAuth, voucherHandler.Partners)

	// --- Phase 4: redemption & verification ---
	// The points-spending step: one atomic check-balance → deduct → decrement-stock
	// → issue-code transaction (docs/06 §4).
	r.POST("/redemptions", requireAuth, redemptionHandler.Create)
	r.GET("/redemptions", requireAuth, redemptionHandler.List)
	// Partner staff or admin only — a user must not be able to mark their own code
	// used, which would let one code be presented at two tills.
	//
	// The route is POST /redemptions/:code/verify with :code being the
	// redemption_code, not an id, so it does not collide with the /:id patterns
	// above and needs no numeric parsing.
	r.POST("/redemptions/:code/verify", requireAuth, requirePartner, redemptionHandler.Verify)

	// --- Phase 5: admin ---
	// Platform-wide, so admin only. The `classification` block is the FR-22 payoff:
	// predicted-vs-verified accuracy as a real number.
	r.GET("/admin/stats", requireAuth, requireAdmin, adminHandler.Stats)

	// Catalogue administration. Grouped because every route shares the same two
	// middlewares, and listing them once makes it obvious none was missed.
	//
	// DELETE is a soft delete (`active = 0`) on both: vouchers are referenced by
	// issued redemptions, and partners own vouchers, so a hard delete would break a
	// code a user is already holding.
	adminRoutes := r.Group("/admin", requireAuth, requireAdmin)
	{
		adminRoutes.GET("/partners", catalogAdminHandler.ListPartners)
		adminRoutes.POST("/partners", catalogAdminHandler.CreatePartner)
		adminRoutes.GET("/partners/:id", catalogAdminHandler.GetPartner)
		adminRoutes.PATCH("/partners/:id", catalogAdminHandler.UpdatePartner)
		adminRoutes.DELETE("/partners/:id", catalogAdminHandler.DeletePartner)

		adminRoutes.GET("/vouchers", catalogAdminHandler.ListVouchers)
		adminRoutes.POST("/vouchers", catalogAdminHandler.CreateVoucher)
		adminRoutes.GET("/vouchers/:id", catalogAdminHandler.GetVoucher)
		adminRoutes.PATCH("/vouchers/:id", catalogAdminHandler.UpdateVoucher)
		adminRoutes.DELETE("/vouchers/:id", catalogAdminHandler.DeleteVoucher)
	}

	// Role-gated routes wrap requireAuth with middleware.RequireRole(...), which
	// always permits admin as well, so one admin login can drive every flow during
	// a demo.

	return r
}

// newClassifier builds the configured classifier, or nil when disabled.
//
// Selection lives here rather than in config so the config package stays free of
// a dependency on the classify package (and thus on the Anthropic SDK).
//
// An empty provider means mock, not nil and not claude. config.Load never leaves
// it empty, so this case is reached by callers that build a Config directly —
// chiefly the tests, which must stay deterministic and offline. Requiring an
// explicit "claude" to reach the network is what stops a stray ANTHROPIC_API_KEY
// in the environment from turning `go test` into a billed API call.
func newClassifier(cfg *config.Config) classify.Classifier {
	switch cfg.ClassifyProvider {
	case config.ClassifyProviderClaude:
		return classify.NewClaudeClassifier(cfg.AnthropicAPIKey, cfg.ClassifyModel)
	case config.ClassifyProviderOff:
		return nil
	default:
		return classify.NewMockClassifier()
	}
}

// corsMiddleware allows the Flutter app (and a browser-based admin view, if one
// is added) to call the API cross-origin. Android release builds do not need
// CORS, but Flutter web and local browser testing do.
func corsMiddleware(origins []string) gin.HandlerFunc {
	allowAll := len(origins) == 1 && origins[0] == "*"
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		switch {
		case allowAll:
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "" && allowed[origin]:
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
