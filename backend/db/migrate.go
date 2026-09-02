package db

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/config"
)

// Migrations are embedded rather than read from disk so the binary carries them. That is what makes
// local, CI and the deployed host provably the same schema — there is no path where one of them
// applies a different directory.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// withMigrator runs one migration operation over a pool it opens and closes itself.
//
// Its own pool, not the caller's. The driver holds a dedicated connection for its whole lifetime, so
// a migrator built over a shared pool subtracts one connection from it for as long as it lives.
//
// Connect, not golang-migrate's URL-based migrate.New: it goes through DSN, whose values are quoted
// keyword/value pairs. A postgres:// URL would reintroduce that quoting as percent-encoding, where a
// password containing a space ends its value early and shifts every keyword after it.
//
// The schema itself is created by 000001, so the migration bookkeeping table is pinned to public —
// putting it in f1 would make the down migration drop the record of its own having run.
func withMigrator(cfg *config.Config, run func(*migrate.Migrate) error) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("db.withMigrator source: %w", err)
	}

	conn, err := Connect(cfg)
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("db.withMigrator connect (%s): %w", Describe(cfg), err)
	}
	// One operation uses one connection, and closing the pool on the way out is what keeps a run
	// that drives migrations repeatedly from accumulating Postgres backends.
	conn.SetMaxOpenConns(1)
	defer func() { _ = conn.Close() }()

	driver, err := postgres.WithInstance(conn.DB, &postgres.Config{
		DatabaseName:    cfg.DBName,
		SchemaName:      "public",
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("db.withMigrator driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, cfg.DBName, driver)
	if err != nil {
		_ = driver.Close()
		_ = source.Close()
		return fmt.Errorf("db.withMigrator: %w", err)
	}
	// Closes the source and the driver, and the driver closes the pool it was built over — which the
	// deferred Close above then finds already closed, and reports nothing about.
	defer func() { _, _ = m.Close() }()

	return run(m)
}

// MigrateUp applies every pending migration. Already-current is success, not an error.
func MigrateUp(cfg *config.Config) error {
	return withMigrator(cfg, func(m *migrate.Migrate) error {
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("db.MigrateUp: %w", err)
		}
		return nil
	})
}

// MigrateDown rolls every migration back. Destructive by definition — it is wired to a Makefile
// target and to tests, never to service startup.
func MigrateDown(cfg *config.Config) error {
	return withMigrator(cfg, func(m *migrate.Migrate) error {
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("db.MigrateDown: %w", err)
		}
		return nil
	})
}

// MigrateVersion reports the applied version and whether the last attempt left it dirty. A dirty
// version means a migration failed partway and the schema is in neither state.
func MigrateVersion(cfg *config.Config) (uint, bool, error) {
	var version uint
	var dirty bool

	err := withMigrator(cfg, func(m *migrate.Migrate) error {
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
