package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss/table"
	"github.com/janemig/plentyone/internal/app"
	"github.com/janemig/plentyone/internal/dashboard"
	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/generate/product"
	"github.com/janemig/plentyone/internal/generate/validate"
	"github.com/janemig/plentyone/internal/pipeline"
	"github.com/janemig/plentyone/internal/plenty"
	"github.com/janemig/plentyone/internal/storage"
	"github.com/janemig/plentyone/internal/storage/queries"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
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

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push generated products to PlentyONE",
	RunE:  runPush,
}

func runPush(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.Default()

	jobID, _ := cmd.Flags().GetInt64("job-id")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	resume, _ := cmd.Flags().GetBool("resume")
	resetFailed, _ := cmd.Flags().GetBool("reset-failed")

	// Open DB connection.
	db, err := storage.NewDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()
	q := queries.New(db)

	// Verify job exists.
	job, err := q.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("getting job %d: %w", jobID, err)
	}
	_ = job

	// Create PlentyONE client.
	plentyClient := plenty.NewClient(plenty.ClientConfig{
		BaseURL:   cfg.API.BaseURL,
		Username:  cfg.API.Username,
		Password:  cfg.API.Password,
		RateLimit: cfg.API.RateLimit,
		Timeout:   time.Duration(cfg.API.Timeout) * time.Second,
		DryRun:    dryRun,
		Logger:    logger,
		DB:        q,
	})

	// Create pipeline.
	pipeCfg := pipeline.PipelineConfig{Concurrency: cfg.Pipeline.Concurrency}
	p := pipeline.NewPipeline(plentyClient, q, db, pipeCfg, logger)

	// Register all 6 stages.
	p.RegisterStage(pipeline.NewCategoryStage(plentyClient, q, pipeCfg))
	p.RegisterStage(pipeline.NewAttributeStage(plentyClient, q, pipeCfg))
	p.RegisterStage(pipeline.NewProductStage(plentyClient, q, pipeCfg))
	p.RegisterStage(pipeline.NewVariationStage(plentyClient, q, pipeCfg))
	p.RegisterStage(pipeline.NewImageStage(plentyClient, q, pipeCfg))
	p.RegisterStage(pipeline.NewTextStage(plentyClient, q, pipeCfg))

	// Handle resume mode.
	if resume {
		run, err := q.GetPipelineRunByJobLatest(ctx, jobID)
		if err != nil {
			return fmt.Errorf("getting latest pipeline run for job %d: %w", jobID, err)
		}
		fmt.Printf("Resuming pipeline run %d\n", run.ID)
		return p.Resume(ctx, run.ID, resetFailed)
	}

	// Create new pipeline run.
	runID, err := q.CreatePipelineRun(ctx, queries.CreatePipelineRunParams{
		JobID:        jobID,
		Status:       string(domain.PipelinePending),
		CurrentStage: sql.NullString{},
	})
	if err != nil {
		return fmt.Errorf("creating pipeline run: %w", err)
	}

	rc := &pipeline.RunContext{
		RunID:  runID,
		JobID:  jobID,
		DryRun: dryRun,
		Logger: logger,
	}

	if err := p.Run(ctx, rc); err != nil {
		return err
	}

	fmt.Printf("Pipeline completed (run ID: %d)\n", runID)
	return nil
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show pipeline and job status",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	jobID, _ := cmd.Flags().GetInt64("job-id")
	runID, _ := cmd.Flags().GetInt64("run-id")

	// Open DB connection.
	db, err := storage.NewDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()
	q := queries.New(db)

	// Determine what to show based on flags.
	switch {
	case runID > 0:
		// Case A: --run-id provided -- show specific pipeline run details.
		run, err := q.GetPipelineRun(ctx, runID)
		if err != nil {
			return fmt.Errorf("getting pipeline run %d: %w", runID, err)
		}
		fmt.Printf("Pipeline Run #%d (Job #%d)\n", run.ID, run.JobID)
		fmt.Printf("Status: %s\n", run.Status)
		if run.CurrentStage.Valid {
			fmt.Printf("Current Stage: %s\n", run.CurrentStage.String)
		}
		if run.ErrorMessage.Valid {
			fmt.Printf("Error: %s\n", run.ErrorMessage.String)
		}

	case jobID > 0:
		// Case B: --job-id provided -- show latest pipeline run for that job.
		job, err := q.GetJob(ctx, jobID)
		if err != nil {
			return fmt.Errorf("getting job %d: %w", jobID, err)
		}
		fmt.Printf("Job #%d: %s (%s)\n", job.ID, job.Name, job.Status)

		run, err := q.GetPipelineRunByJobLatest(ctx, jobID)
		if err != nil {
			// No pipeline runs -- just show job info.
			fmt.Println("No pipeline runs found for this job.")
			return nil
		}
		fmt.Printf("\nLatest Pipeline Run #%d (%s)\n", run.ID, run.Status)
		runID = run.ID

	default:
		// Case C: no flags -- show recent jobs overview.
		jobs, err := q.ListRecentJobs(ctx, 10)
		if err != nil {
			return fmt.Errorf("listing recent jobs: %w", err)
		}
		if len(jobs) == 0 {
			fmt.Println("No jobs found.")
			return nil
		}

		t := table.New().
			Headers("ID", "Name", "Type", "Status", "Created").
			BorderRow(true)

		for _, job := range jobs {
			t.Row(
				fmt.Sprint(job.ID),
				job.Name,
				job.JobType,
				job.Status,
				job.CreatedAt.Format("2006-01-02 15:04"),
			)
		}

		fmt.Println(t.Render())
		return nil
	}

	// Stage details (for Case A and B when a run exists).
	if runID > 0 {
		stages, err := q.ListStageStatesByRun(ctx, runID)
		if err != nil {
			return fmt.Errorf("listing stage states: %w", err)
		}

		if len(stages) > 0 {
			fmt.Println()
			t := table.New().
				Headers("Stage", "Status", "Processed", "Total", "Duration").
				BorderRow(true)

			for _, stage := range stages {
				durationStr := "-"
				if stage.StartedAt.Valid && stage.CompletedAt.Valid {
					d := stage.CompletedAt.Time.Sub(stage.StartedAt.Time)
					durationStr = fmt.Sprintf("%.0fs", d.Seconds())
				}
				t.Row(
					stage.StageName,
					stage.Status,
					fmt.Sprint(stage.Processed),
					fmt.Sprint(stage.Total),
					durationStr,
				)
			}

			fmt.Println(t.Render())
		}

		// Failed items summary.
		failedCount, err := q.CountFailedByRun(ctx, runID)
		if err != nil {
			return fmt.Errorf("counting failed items: %w", err)
		}
		if failedCount > 0 {
			fmt.Printf("\nFlagged items: %d\n", failedCount)
			failed, _ := q.ListFailedMappingsByRun(ctx, runID)
			for i, f := range failed {
				if i >= 10 {
					fmt.Printf("  ... and %d more\n", failedCount-10)
					break
				}
				errMsg := ""
				if f.ErrorMessage.Valid {
					errMsg = f.ErrorMessage.String
				}
				fmt.Printf("  - %s #%d: %s\n", f.EntityType, f.LocalID, errMsg)
			}
		}
	}

	return nil
}

// Sensitive keys for config masking (matched by leaf key name, any nesting level).
var sensitiveKeys = map[string]bool{
	"api_key":  true,
	"password": true,
	"username": true,
}

func maskSettings(settings map[string]any) map[string]any {
	masked := make(map[string]any, len(settings))
	for k, v := range settings {
		switch val := v.(type) {
		case map[string]any:
			masked[k] = maskSettings(val)
		case string:
			if sensitiveKeys[k] && len(val) > 0 {
				if len(val) > 4 {
					masked[k] = val[:4] + "****"
				} else {
					masked[k] = "****"
				}
			} else {
				masked[k] = v
			}
		default:
			masked[k] = v
		}
	}
	return masked
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View current configuration",
	RunE:  runConfigView,
}

func runConfigView(cmd *cobra.Command, args []string) error {
	settings := viper.AllSettings()
	masked := maskSettings(settings)

	enc := yaml.NewEncoder(os.Stdout)
	enc.SetIndent(2)
	if err := enc.Encode(masked); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	enc.Close()

	if f := viper.ConfigFileUsed(); f != "" {
		fmt.Fprintf(os.Stderr, "\nConfig file: %s\n", f)
	} else {
		fmt.Fprintf(os.Stderr, "\nNo config file found (using defaults + environment)\n")
	}
	return nil
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]

	// Warn about sensitive keys.
	leafKey := key
	if idx := strings.LastIndex(key, "."); idx >= 0 {
		leafKey = key[idx+1:]
	}
	if sensitiveKeys[leafKey] {
		envKey := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		fmt.Fprintf(os.Stderr, "Warning: Prefer using environment variable PLENTYONE_%s instead of storing sensitive values in config file\n", envKey)
	}

	viper.Set(key, value)

	cfgPath := viper.ConfigFileUsed()
	if cfgPath == "" {
		cfgPath = "./config.yaml"
		fmt.Fprintf(os.Stderr, "Creating new config file: %s\n", cfgPath)
	}

	if err := viper.WriteConfigAs(cfgPath); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	fmt.Printf("%s = %s\n", key, value)
	return nil
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web dashboard",
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Open DB connection.
	db, err := storage.NewDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()
	q := queries.New(db)

	// Create SSE broker and start event loop.
	broker := dashboard.NewBroker()
	go broker.Run(ctx)

	// Create handlers and router.
	handlers := dashboard.NewHandlers(q, db, cfg, broker)
	router := dashboard.NewRouter(handlers, broker)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	slog.Info("starting dashboard", slog.String("addr", addr))

	srv := &http.Server{Addr: addr, Handler: router}

	// Graceful shutdown goroutine.
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(serveCmd)

	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)

	configCmd.AddCommand(configSetCmd)

	migrateCmd.PersistentFlags().Bool("dry-run", false, "print DSN and exit without running migrations")

	generateCmd.Flags().String("niche", "", "product niche (e.g., electronics, fashion)")
	generateCmd.Flags().Int("count", 10, "number of products to generate")
	generateCmd.Flags().String("provider", "", "AI provider override (mock, openai)")
	_ = generateCmd.MarkFlagRequired("niche")

	pushCmd.Flags().Int64("job-id", 0, "job ID to push")
	pushCmd.Flags().Bool("dry-run", false, "simulate pipeline without making API calls")
	pushCmd.Flags().Bool("resume", false, "resume the latest pipeline run for this job")
	pushCmd.Flags().Bool("reset-failed", false, "reset failed entity mappings before resuming")
	_ = pushCmd.MarkFlagRequired("job-id")

	statusCmd.Flags().Int64("job-id", 0, "filter by job ID")
	statusCmd.Flags().Int64("run-id", 0, "filter by pipeline run ID")
}
