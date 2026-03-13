package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/janemig/plentyone/internal/app"
	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/generate/product"
	"github.com/janemig/plentyone/internal/generate/validate"
	"github.com/janemig/plentyone/internal/storage"
	"github.com/janemig/plentyone/internal/storage/queries"
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

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate product data using AI",
	RunE:  runGenerate,
}

func runGenerate(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.Default()

	niche, _ := cmd.Flags().GetString("niche")
	count, _ := cmd.Flags().GetInt("count")
	provider, _ := cmd.Flags().GetString("provider")

	// Open DB connection.
	db, err := storage.NewDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()
	q := queries.New(db)

	// Override provider if flag is set.
	if provider != "" {
		cfg.AI.Provider = provider
	}

	// Create AI generator.
	gen, err := app.NewGeneratorFromConfig(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating AI generator: %w", err)
	}

	// Create product generator.
	prodGen := product.NewGenerator(gen, validate.NewValidator(), cfg.AI.Languages, logger)

	// Create job config JSON.
	configJSON, err := json.Marshal(map[string]any{
		"niche":    niche,
		"count":    count,
		"provider": cfg.AI.Provider,
	})
	if err != nil {
		return fmt.Errorf("marshaling job config: %w", err)
	}

	// Create job record.
	jobID, err := q.CreateJob(ctx, queries.CreateJobParams{
		Name:    fmt.Sprintf("generate-%s-%d", niche, count),
		JobType: string(domain.JobGenerate),
		Config:  json.RawMessage(configJSON),
		Status:  string(domain.StatusPending),
	})
	if err != nil {
		return fmt.Errorf("creating job: %w", err)
	}

	// Mark job as running.
	if err := q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
		Status: "running",
		ID:     jobID,
	}); err != nil {
		return fmt.Errorf("updating job status: %w", err)
	}

	// Create one category per job.
	var categoryCreated bool
	_ = categoryCreated

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			_ = q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
				Status: "failed",
				ID:     jobID,
			})
			return ctx.Err()
		default:
		}

		req := product.GenerationRequest{
			ProductType: niche,
			Niche:       niche,
			ProductName: "",
			Category:    niche,
		}

		result, err := prodGen.Generate(ctx, req)
		if err != nil {
			_ = q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
				Status: "failed",
				ID:     jobID,
			})
			return fmt.Errorf("generating product %d/%d: %w", i+1, count, err)
		}

		// Get the product name from the first available language.
		var productName string
		for _, lang := range cfg.AI.Languages {
			if texts, ok := result.Texts[lang]; ok && texts.Name != "" {
				productName = texts.Name
				break
			}
		}
		if productName == "" {
			productName = fmt.Sprintf("%s-product-%d", niche, i+1)
		}

		// Marshal base data (all generated texts).
		baseDataJSON, err := json.Marshal(result.Texts)
		if err != nil {
			return fmt.Errorf("marshaling base data: %w", err)
		}

		// Persist product.
		productID, err := q.CreateProduct(ctx, queries.CreateProductParams{
			JobID:       jobID,
			Name:        productName,
			ProductType: niche,
			BaseData:    json.RawMessage(baseDataJSON),
			Status:      string(domain.StatusPending),
		})
		if err != nil {
			return fmt.Errorf("creating product record: %w", err)
		}

		// Create category once per job.
		if !categoryCreated {
			_, err := q.CreateCategory(ctx, queries.CreateCategoryParams{
				JobID:     jobID,
				ParentID:  sql.NullInt64{},
				Name:      niche,
				Level:     1,
				SortOrder: 0,
				Status:    string(domain.StatusPending),
			})
			if err != nil {
				return fmt.Errorf("creating category: %w", err)
			}
			categoryCreated = true
		}

		// Persist texts for each language.
		for lang, texts := range result.Texts {
			textFields := map[string]string{
				"name":             texts.Name,
				"shortDescription": texts.ShortDescription,
				"description":      texts.Description,
				"technicalData":    texts.TechnicalData,
				"metaDescription":  texts.MetaDescription,
				"urlContent":       texts.URLContent,
				"previewText":      texts.PreviewText,
			}
			for field, content := range textFields {
				if content == "" {
					continue
				}
				if _, err := q.CreateText(ctx, queries.CreateTextParams{
					ProductID: productID,
					Field:     field,
					Lang:      lang,
					Content:   content,
					Status:    string(domain.StatusPending),
				}); err != nil {
					return fmt.Errorf("creating text %s/%s: %w", field, lang, err)
				}
			}
		}

		// Create default variation.
		skuPrefix := niche
		if len(skuPrefix) > 3 {
			skuPrefix = skuPrefix[:3]
		}
		if _, err := q.CreateVariation(ctx, queries.CreateVariationParams{
			ProductID:  productID,
			Name:       "Default",
			Sku:        fmt.Sprintf("%s-%d", skuPrefix, productID),
			Price:      sql.NullString{String: "0.00", Valid: true},
			Currency:   "EUR",
			Weight:     sql.NullString{},
			WeightUnit: "",
			Barcode:    "",
			Status:     string(domain.StatusPending),
		}); err != nil {
			return fmt.Errorf("creating variation: %w", err)
		}

		slog.Info("generated product", "index", i+1, "total", count, "job_id", jobID)
	}

	// Mark job as completed.
	if err := q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
		Status: "completed",
		ID:     jobID,
	}); err != nil {
		return fmt.Errorf("updating job status to completed: %w", err)
	}

	fmt.Printf("Generated %d products (job ID: %d)\n", count, jobID)
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(generateCmd)

	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)

	migrateCmd.PersistentFlags().Bool("dry-run", false, "print DSN and exit without running migrations")

	generateCmd.Flags().String("niche", "", "product niche (e.g., electronics, fashion)")
	generateCmd.Flags().Int("count", 10, "number of products to generate")
	generateCmd.Flags().String("provider", "", "AI provider override (mock, openai)")
	_ = generateCmd.MarkFlagRequired("niche")
}
