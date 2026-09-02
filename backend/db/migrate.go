package db

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
)

// Migrations are embedded rather than read from disk so the binary carries them. That is what makes
// local, CI and the deployed host provably the same schema — there is no path where one of them
// applies a different directory.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// withMigrator runs one migration operation against the caller's pool, and gives back everything it
// borrowed.
//
// Giving it back is the whole point. The driver holds a dedicated connection for its lifetime, so a
// migrator that is never closed subtracts one connection from the caller's pool of ten for as long
// as that pool lives. In cmd/migrate that costs nothing — the process exits immediately, and the
// API binary never migrates at all. The test suite is where it bites: every test stands up its own
// pool and drives a down and an up through it, so the suite held tens of Postgres backends at once
// against a server that allows a hundred.
//
// Closing it is where the trap is. The driver's WithInstance takes its connection out of the pool
// and keeps a reference to the pool as well, which its Close then closes — so releasing the
// connection that way would tear the caller's shared handle down with it. Taking the connection
// here instead leaves the driver with nothing to close but that one connection, and closing a
// pooled connection is what returns it to the pool.
//
// The schema itself is created by 000001, so the migration bookkeeping table is pinned to public —
// putting it in f1 would make the down migration drop the record of its own having run.
func withMigrator(conn *sqlx.DB, cfg *config.Config, run func(*migrate.Migrate) error) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("db.withMigrator source: %w", err)
	}

	ctx := context.Background()

	pooled, err := conn.Conn(ctx)
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("db.withMigrator connection: %w", err)
	}

	driver, err := postgres.WithConnection(ctx, pooled, &postgres.Config{
		DatabaseName:    cfg.DBName,
		SchemaName:      "public",
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		_ = pooled.Close()
		_ = source.Close()
		return fmt.Errorf("db.withMigrator driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, cfg.DBName, driver)
	if err != nil {
		_ = pooled.Close()
		_ = source.Close()
		return fmt.Errorf("db.withMigrator: %w", err)
	}
	// Closes the source and the driver. The driver closes the single connection it was handed,
	// which is how that connection returns to the pool it came from.
	defer func() { _, _ = m.Close() }()

	return run(m)
}

// MigrateUp applies every pending migration. Already-current is success, not an error.
func MigrateUp(conn *sqlx.DB, cfg *config.Config) error {
	return withMigrator(conn, cfg, func(m *migrate.Migrate) error {
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("db.MigrateUp: %w", err)
		}
		return nil
	})
}

// MigrateDown rolls every migration back. Destructive by definition — it is wired to a Makefile
// target and to tests, never to service startup.
func MigrateDown(conn *sqlx.DB, cfg *config.Config) error {
	return withMigrator(conn, cfg, func(m *migrate.Migrate) error {
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("db.MigrateDown: %w", err)
		}
		return nil
	})
}

// MigrateVersion reports the applied version and whether the last attempt left it dirty. A dirty
// version means a migration failed partway and the schema is in neither state.
func MigrateVersion(conn *sqlx.DB, cfg *config.Config) (uint, bool, error) {
	var version uint
	var dirty bool

	err := withMigrator(conn, cfg, func(m *migrate.Migrate) error {
		var err error
		version, dirty, err = m.Version()
		if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
			return fmt.Errorf("db.MigrateVersion: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return version, dirty, nil
}
