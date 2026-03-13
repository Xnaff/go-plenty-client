package storage

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	migrationFiles "github.com/janemig/plentyone/migrations"
)

// RunMigrations applies all pending database migrations.
// It uses the embedded SQL files from the migrations package.
// Returns nil if all migrations applied successfully or if there are no new migrations.
func RunMigrations(db *sql.DB) error {
	m, err := newMigrator(db)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	slog.Info("migrations applied",
		slog.Uint64("version", uint64(version)),
		slog.Bool("dirty", dirty),
	)

	return nil
}

// RollbackMigration rolls back the most recently applied migration.
// It steps back exactly one migration version.
func RollbackMigration(db *sql.DB) error {
	m, err := newMigrator(db)
	if err != nil {
		return err
	}

	if err := m.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("rolling back migration: %w", err)
	}

	version, dirty, verErr := m.Version()
	if verErr != nil && verErr != migrate.ErrNoChange {
		slog.Info("all migrations rolled back")
	} else {
		slog.Info("migration rolled back",
			slog.Uint64("version", uint64(version)),
			slog.Bool("dirty", dirty),
		)
	}

	return nil
}

// newMigrator creates a migrate instance from the embedded migration files.
func newMigrator(db *sql.DB) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(migrationFiles.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("creating migration source: %w", err)
	}

	dbDriver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return nil, fmt.Errorf("creating migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "mysql", dbDriver)
	if err != nil {
		return nil, fmt.Errorf("creating migrator: %w", err)
	}

	return m, nil
}
