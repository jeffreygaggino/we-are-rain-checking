package db

import (
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

// newMigrator opens a migrator over an existing pool. The schema itself is created by 000001, so
// the migration bookkeeping table is pinned to public — putting it in f1 would make the down
// migration drop the record of its own having run.
func newMigrator(conn *sqlx.DB, cfg *config.Config) (*migrate.Migrate, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("db.newMigrator source: %w", err)
	}

	driver, err := postgres.WithInstance(conn.DB, &postgres.Config{
		DatabaseName:    cfg.DBName,
		SchemaName:      "public",
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return nil, fmt.Errorf("db.newMigrator driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, cfg.DBName, driver)
	if err != nil {
		return nil, fmt.Errorf("db.newMigrator: %w", err)
	}
	return m, nil
}

// MigrateUp applies every pending migration. Already-current is success, not an error.
func MigrateUp(conn *sqlx.DB, cfg *config.Config) error {
	m, err := newMigrator(conn, cfg)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db.MigrateUp: %w", err)
	}
	return nil
}

// MigrateDown rolls every migration back. Destructive by definition — it is wired to a Makefile
// target and to tests, never to service startup.
func MigrateDown(conn *sqlx.DB, cfg *config.Config) error {
	m, err := newMigrator(conn, cfg)
	if err != nil {
		return err
	}
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("db.MigrateDown: %w", err)
	}
	return nil
}

// MigrateVersion reports the applied version and whether the last attempt left it dirty. A dirty
// version means a migration failed partway and the schema is in neither state.
func MigrateVersion(conn *sqlx.DB, cfg *config.Config) (uint, bool, error) {
	m, err := newMigrator(conn, cfg)
	if err != nil {
		return 0, false, err
	}
	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, fmt.Errorf("db.MigrateVersion: %w", err)
	}
	return version, dirty, nil
}
