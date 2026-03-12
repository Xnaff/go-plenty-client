package main

import (
	"fmt"
	"os"

	"github.com/janemig/plentyone/internal/app"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("migrate not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)
}
