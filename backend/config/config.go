package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port        string
	PublicURL   string
	APIBasePath string

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSchema   string
	DBSSLMode  string

	OpenF1BaseURL     string
	OpenMeteoBaseURL  string
	UpstreamTimeout   time.Duration
	ForecastCacheTTL  time.Duration
	IngestMinInterval time.Duration
}

var cfg = &Config{}

func GetConfig() *Config { return cfg }

func Address() string { return fmt.Sprintf(":%s", cfg.Port) }

// LoadConfig reads the environment once at boot. Anything the service cannot run without is fatal
// here rather than a zero value that surfaces as a mystery at request time.
func LoadConfig() {
	cfg.Port = envOr("PORT", "8080")
	cfg.PublicURL = envOr("PUBLIC_URL", "localhost:"+cfg.Port)
	cfg.APIBasePath = envOr("API_BASE_PATH", "/api/v1")

	cfg.DBHost = mustEnv("DB_HOST")
	cfg.DBPort = envOr("DB_PORT", "5432")
	cfg.DBName = mustEnv("DB_NAME")
	cfg.DBUser = mustEnv("DB_USER")
	cfg.DBPassword = mustEnv("DB_PASSWORD")
	cfg.DBSchema = envOr("DB_SCHEMA", "f1")
	// Secure by default: the operator has to opt out of TLS, not opt in.
	cfg.DBSSLMode = envOr("DB_SSLMODE", "require")

	cfg.OpenF1BaseURL = envOr("OPENF1_BASE_URL", "https://api.openf1.org/v1")
	cfg.OpenMeteoBaseURL = envOr("OPENMETEO_BASE_URL", "https://api.open-meteo.com/v1")
	cfg.UpstreamTimeout = envDuration("UPSTREAM_TIMEOUT", 10*time.Second)
	cfg.ForecastCacheTTL = envDuration("FORECAST_CACHE_TTL", time.Hour)
	// OpenF1 documents 30 req/min; 2100ms between calls leaves headroom under both that and the
	// 3 req/s ceiling.
	cfg.IngestMinInterval = envDuration("INGEST_MIN_INTERVAL", 2100*time.Millisecond)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Fatalf("config: %s is not a duration: %v", key, err)
	}
	return d
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("config: %s is required", key)
	}
	return v
}

// Quote wraps a DSN value in single quotes. The DSN is keyword/value pairs rather than a URL, so a
// password containing a space would otherwise end the value early and shift every later keyword.
func Quote(v string) string {
	return "'" + dsnEscaper.Replace(v) + "'"
}

var dsnEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)
