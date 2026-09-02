package db

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
)

var DB *sqlx.DB

// DSN builds the connection string. Every value is quoted because these are keyword/value pairs,
// not a URL: an unquoted password containing a space would end its value early and shift every
// keyword after it.
//
// The returned string contains the password, so it must never be logged. Use Describe for that.
func DSN(cfg *config.Config) string {
	return strings.Join([]string{
		"host=" + config.Quote(cfg.DBHost),
		"port=" + config.Quote(cfg.DBPort),
		"dbname=" + config.Quote(cfg.DBName),
		"user=" + config.Quote(cfg.DBUser),
		"password=" + config.Quote(cfg.DBPassword),
		"sslmode=" + config.Quote(cfg.DBSSLMode),
		"search_path=" + config.Quote(cfg.DBSchema),
	}, " ")
}

// Describe is the loggable form of a connection: everything DSN carries except the password, which
// is omitted entirely rather than masked, so there is no version of this string a password can
// leak through.
func Describe(cfg *config.Config) string {
	return fmt.Sprintf("host=%s port=%s dbname=%s user=%s sslmode=%s search_path=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBSSLMode, cfg.DBSchema)
}

func ConnectDatabase(cfg *config.Config) {
	conn, err := Connect(cfg)
	if err != nil {
		// %v on the driver error only — never the DSN, which holds the password.
		log.Fatalf("db: failed to connect (%s): %v", Describe(cfg), err)
	}
	DB = conn
	log.Printf("db: connected to postgres (%s)", Describe(cfg))
}

// Connect opens and verifies a pool. sqlx.Connect pings, so a returned handle has proved the
// credentials rather than merely parsed them.
func Connect(cfg *config.Config) (*sqlx.DB, error) {
	conn, err := sqlx.Connect("postgres", DSN(cfg))
	if err != nil {
		return nil, fmt.Errorf("db.Connect: %w", err)
	}
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(time.Hour)
	return conn, nil
}

// Ping is what the health route calls. It bounds its own wait so an unreachable database answers
// the health check rather than hanging it.
func Ping(ctx context.Context, conn *sqlx.DB) error {
	if conn == nil {
		return fmt.Errorf("db.Ping: no connection")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("db.Ping: %w", err)
	}
	return nil
}
