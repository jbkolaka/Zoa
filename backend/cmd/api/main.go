// Command api is the Zoa backend HTTP server.
//
// Boot sequence: load config → open SQLite (with pragmas) → apply migrations →
// build the token issuer → serve. Migrations run on every boot and are
// idempotent, so a fresh clone and a redeploy both converge on the same schema
// with no manual step.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"zoa/backend/internal/auth"
	"zoa/backend/internal/config"
	"zoa/backend/internal/db"
	"zoa/backend/internal/handlers"
	"zoa/backend/internal/seed"
	"zoa/backend/internal/store"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations, print the schema state, then exit")
	seedDemo := flag.Bool("seed-demo", false, "create the demo accounts (recycler, collector, partner staff, admin) if missing")
	flag.Parse()

	// A container image is configured with environment variables, not argv, so
	// the same seeding is reachable through ZOA_SEED_DEMO. The free-tier deploy
	// depends on it: that filesystem is ephemeral, so every restart and every
	// wake from spin-down starts on an empty database, and without re-seeding
	// there would be no account left to sign in with. Safe to leave on — seeding
	// is idempotent and never modifies an account that already exists.
	shouldSeed := *seedDemo || envTrue("ZOA_SEED_DEMO")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer conn.Close()

	ran, err := db.Migrate(conn)
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if len(ran) == 0 {
		log.Printf("migrate: schema already up to date")
	} else {
		for _, name := range ran {
			log.Printf("migrate: applied %s", name)
		}
	}

	if *migrateOnly {
		log.Printf("migrate-only: done (db=%s)", cfg.DBPath)
		return
	}

	issuer, err := buildTokenIssuer(cfg)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	if shouldSeed {
		if err := seedDemoUsers(conn); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handlers.NewRouter(conn, cfg, issuer),
		ReadHeaderTimeout: 10 * time.Second,
		// Generous relative to the <500ms NFR target: Phase 2.5 uploads a photo
		// over a slow mobile link, and that must not be cut off mid-body.
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Serve in the background so the main goroutine can wait for a signal.
	go func() {
		log.Printf("zoa-api %s listening on :%s (env=%s, db=%s)",
			handlers.Version, cfg.Port, cfg.Env, cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown: let in-flight requests finish. This matters for the
	// redemption transaction — killing mid-write on SQLite is how you get a
	// hot journal to recover from.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Printf("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Printf("stopped")
}

// buildTokenIssuer returns the JWT issuer, generating a throwaway secret in dev
// when none is configured.
//
// Config already refuses to start outside dev without JWT_SECRET, so this cannot
// silently weaken a demo or production deployment. In dev, a fresh secret per
// boot means tokens do not survive a restart — an honest "sign in again" rather
// than a hardcoded default secret that could escape into a real environment.
func buildTokenIssuer(cfg *config.Config) (*auth.TokenIssuer, error) {
	secret := cfg.JWTSecret

	if secret == "" {
		generated, err := auth.GenerateDevSecret()
		if err != nil {
			return nil, err
		}
		secret = generated
		log.Printf("auth: no JWT_SECRET set — generated a temporary dev secret; " +
			"tokens will not survive a restart")
	}

	return auth.NewTokenIssuer(secret)
}

// envTrue reports whether an environment variable holds a truthy value. Render
// and most container hosts can only supply strings, so "true"/"1" both count
// rather than requiring one exact spelling.
func envTrue(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// seedDemoUsers creates the demo accounts, reporting which were added and which
// already existed. Existing accounts are never modified, so re-running this
// cannot reset a password between demo rehearsals.
func seedDemoUsers(conn *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := seed.Users(ctx, store.NewUserStore(conn))
	if err != nil {
		return err
	}

	for _, user := range result.Created {
		log.Printf("seed: created %s (%s) as %s", user.Name, user.Phone, user.Role)
	}
	for _, user := range result.Skipped {
		log.Printf("seed: %s already exists — left unchanged", user.Phone)
	}
	if len(result.Created) > 0 {
		log.Printf("seed: shared demo password is %q", seed.DemoPassword)
	}
	return nil
}
