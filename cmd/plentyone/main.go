package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/janemig/plentyone/internal/app"
	"github.com/janemig/plentyone/internal/storage"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *app.Config
	version = "v0.1.0-dev"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "plentyone",
	Short: "PlentyONE product data generator and pipeline manager",
	Long: `plentyone generates e-commerce product data using AI and public databases,
then pushes it into PlentyONE through their REST API. It manages the complex
multi-step creation pipeline (categories, attributes, products, variations,
images, multilingual text) and tracks all mappings between local and
PlentyONE IDs in MySQL.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load .env file in development (ignore errors if missing)
		_ = godotenv.Load()

		var err error
		cfg, err = app.LoadConfig(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		app.SetupLogger(cfg.Log)
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("plentyone %s\n", version)
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if dryRun {
			fmt.Printf("DSN: %s\n", storage.MaskedDSN(cfg.Database))
			fmt.Println("(dry-run: no migrations applied)")
			return nil
		}

		slog.Info("connecting to database", slog.String("dsn", storage.MaskedDSN(cfg.Database)))

		db, err := storage.NewDB(cfg.Database)
		if err != nil {
			return fmt.Errorf("connecting to database: %w", err)
		}
		defer db.Close()

		slog.Info("applying migrations")
		if err := storage.RunMigrations(db); err != nil {
			return fmt.Errorf("applying migrations: %w", err)
		}

		slog.Info("migrations applied successfully")
		return nil
	},
}

var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback the most recent migration",
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if dryRun {
			fmt.Printf("DSN: %s\n", storage.MaskedDSN(cfg.Database))
			fmt.Println("(dry-run: no rollback performed)")
			return nil
		}

		slog.Info("connecting to database", slog.String("dsn", storage.MaskedDSN(cfg.Database)))

		db, err := storage.NewDB(cfg.Database)
		if err != nil {
			return fmt.Errorf("connecting to database: %w", err)
		}
		defer db.Close()

		slog.Info("rolling back migration")
		if err := storage.RollbackMigration(db); err != nil {
			return fmt.Errorf("rolling back migration: %w", err)
		}

		slog.Info("migration rolled back successfully")
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)

	migrateCmd.PersistentFlags().Bool("dry-run", false, "print DSN and exit without running migrations")
}
