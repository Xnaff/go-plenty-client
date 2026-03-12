package storage

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/janemig/plentyone/internal/app"
)

// NewDB opens a MySQL connection pool using the provided database configuration.
// It configures pool settings and verifies connectivity with a ping.
func NewDB(cfg app.DatabaseConfig) (*sql.DB, error) {
	dsn := cfg.DatabaseDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxConns)
	if cfg.MaxConns > 1 {
		db.SetMaxIdleConns(cfg.MaxConns / 2)
	} else {
		db.SetMaxIdleConns(1)
	}
	db.SetConnMaxLifetime(5 * time.Minute)

	// Verify connectivity
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	slog.Info("database connection established",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.String("database", cfg.Name),
		slog.Int("max_conns", cfg.MaxConns),
	)

	return db, nil
}

// MaskedDSN returns a DSN string with the password replaced for safe logging.
func MaskedDSN(cfg app.DatabaseConfig) string {
	return fmt.Sprintf("%s:***@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		cfg.User, cfg.Host, cfg.Port, cfg.Name)
}
