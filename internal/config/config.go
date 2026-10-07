// Package config reads runtime settings from the environment (and a local
// .env when present). Everything the server needs is here, so a deployment is
// one binary plus one env file.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port      string
	Env       string
	PublicURL string
	// TrustProxy honours X-Forwarded-For / X-Real-IP. Enable only behind a
	// reverse proxy that sets them, or rate limits can be dodged by spoofing.
	TrustProxy bool

	DatabaseURL string

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Region    string
	S3Bucket    string
	S3Timeout   time.Duration

	// AdminPath is the secret URL prefix that serves the dashboard, e.g.
	// "/console-6f31ecf4". The public bundle never contains it: the server
	// injects it into index.html only for requests under that prefix.
	AdminPath     string
	AdminEmail    string
	AdminPassword string
	JWTSecret     string
	SessionTTL    time.Duration
}

func (c Config) Production() bool { return c.Env == "production" }

func (c Config) StorageConfigured() bool {
	return c.S3Endpoint != "" && c.S3AccessKey != "" && c.S3SecretKey != ""
}

// Load reads .env (if any) without overriding variables already set by the
// process environment, then validates the result.
func Load() (Config, error) {
	_ = godotenv.Load()

	c := Config{
		Port:          get("PORT", "8080"),
		Env:           get("APP_ENV", "development"),
		PublicURL:     strings.TrimRight(get("PUBLIC_URL", ""), "/"),
		TrustProxy:    get("TRUST_PROXY", "false") == "true",
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		S3Endpoint:    os.Getenv("S3_ENDPOINT"),
		S3AccessKey:   os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:   os.Getenv("S3_SECRET_KEY"),
		S3Region:      get("S3_REGION", "us-east-1"),
		S3Bucket:      get("S3_BUCKET", "my-portfolio"),
		S3Timeout:     time.Duration(getInt("S3_TIMEOUT_SECONDS", 10)) * time.Second,
		AdminPath:     "/" + strings.Trim(get("ADMIN_PATH", ""), "/"),
		AdminEmail:    os.Getenv("ADMIN_EMAIL"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		SessionTTL:    time.Duration(getInt("SESSION_TTL_HOURS", 72)) * time.Hour,
	}

	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.AdminPath == "/" {
		errs = append(errs, errors.New("ADMIN_PATH is required (e.g. /console-1a2b3c4d)"))
	} else if strings.HasPrefix(c.AdminPath, "/api") || strings.HasPrefix(c.AdminPath, "/media") || strings.HasPrefix(c.AdminPath, "/assets") {
		errs = append(errs, fmt.Errorf("ADMIN_PATH %q collides with a reserved route", c.AdminPath))
	}
	if len(c.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 characters"))
	}
	return c, errors.Join(errs...)
}

func get(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
