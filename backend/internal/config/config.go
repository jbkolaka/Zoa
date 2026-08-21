// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds every runtime knob. Values come from the environment, optionally
// seeded from a .env file (see .env.example).
type Config struct {
	Port        string
	DBPath      string
	JWTSecret   string
	Env         string // "dev" | "demo" | "prod"
	CORSOrigins []string

	// --- Phase 2.5: classification ---

	// ClassifyProvider selects the classifier: "claude", "mock", or "off".
	// Resolved from ZOA_CLASSIFY_PROVIDER, defaulting to whichever can actually
	// work (see resolveClassifyProvider).
	ClassifyProvider string

	// ClassifyModel is the vision model id. Empty means the classifier's default.
	ClassifyModel string

	// ClassifyTimeout caps a single classification call (TRD §3: <3s).
	ClassifyTimeout time.Duration

	// AnthropicAPIKey is read here only to pass through to the SDK. Empty is
	// valid — the SDK also resolves credentials from the environment directly.
	AnthropicAPIKey string
}

// Classification providers.
const (
	ClassifyProviderClaude = "claude"
	ClassifyProviderMock   = "mock"
	ClassifyProviderOff    = "off"
)

// Load reads .env if present, then the environment. A missing .env is not an
// error — in a container, values arrive as real environment variables.
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:      envOr("PORT", "8080"),
		DBPath:    envOr("DB_PATH", "app.db"),
		JWTSecret: os.Getenv("JWT_SECRET"),
		Env:       envOr("APP_ENV", "dev"),

		ClassifyModel:   os.Getenv("ZOA_CLASSIFY_MODEL"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
	}

	cfg.ClassifyProvider = resolveClassifyProvider(
		os.Getenv("ZOA_CLASSIFY_PROVIDER"), cfg.AnthropicAPIKey)

	timeout, err := parseTimeout(os.Getenv("ZOA_CLASSIFY_TIMEOUT"))
	if err != nil {
		return nil, err
	}
	cfg.ClassifyTimeout = timeout

	origins := envOr("CORS_ORIGINS", "*")
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate fails fast on misconfiguration. JWT_SECRET is unused until Phase 1,
// but a dev-only default is generated there rather than shipping a hardcoded
// secret — so this only hard-fails outside dev.
func (c *Config) validate() error {
	if c.Env != "dev" && c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET must be set when APP_ENV=%q", c.Env)
	}
	if c.DBPath == "" {
		return fmt.Errorf("DB_PATH must not be empty")
	}
	return nil
}

// IsDev reports whether the app is running in development mode.
func (c *Config) IsDev() bool { return c.Env == "dev" }

// resolveClassifyProvider picks the classifier from configuration, defaulting to
// whichever one can actually run.
//
// The default is deliberate: an unset provider with no API key falls back to the
// mock rather than to "off", so a fresh clone demonstrates the AI step instead of
// silently omitting it. An explicit value always wins, including "off".
func resolveClassifyProvider(configured, apiKey string) string {
	switch strings.ToLower(strings.TrimSpace(configured)) {
	case ClassifyProviderClaude:
		return ClassifyProviderClaude
	case ClassifyProviderMock:
		return ClassifyProviderMock
	case ClassifyProviderOff:
		return ClassifyProviderOff
	}
	if apiKey != "" {
		return ClassifyProviderClaude
	}
	return ClassifyProviderMock
}

// parseTimeout reads a Go duration, falling back to the TRD's 3s budget.
func parseTimeout(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 3 * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("ZOA_CLASSIFY_TIMEOUT %q is not a duration (try 3s): %w", v, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("ZOA_CLASSIFY_TIMEOUT must be positive, got %s", d)
	}
	return d, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
