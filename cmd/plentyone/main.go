package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss/table"
	"github.com/janemig/plentyone/internal/app"
	"github.com/janemig/plentyone/internal/dashboard"
	"github.com/janemig/plentyone/internal/domain"
	"github.com/janemig/plentyone/internal/ean"
	"github.com/janemig/plentyone/internal/enrichment"
	"github.com/janemig/plentyone/internal/generate"
	"github.com/janemig/plentyone/internal/generate/quality"
	"github.com/janemig/plentyone/internal/generate/product"
	"github.com/janemig/plentyone/internal/generate/validate"
	"github.com/janemig/plentyone/internal/imagesource"
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
		cfg.Images.Provider = provider
	}

	// Create AI generator.
	gen, err := app.NewGeneratorFromConfig(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating AI generator: %w", err)
	}

	// Create product generator.
	prodGen := product.NewGenerator(gen, validate.NewValidator(), cfg.AI.Languages, logger)

	// Create image generator.
	imageGen, err := app.NewImageGeneratorFromConfig(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating image generator: %w", err)
	}

	// Create stock photo sources.
	imageSources := app.NewImageSourcesFromConfig(cfg)

	// Create enrichers.
	enrichers := app.NewEnrichersFromConfig(cfg)

	// Create quality scorer.
	scorer := app.NewQualityScorerFromConfig(cfg)

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

	// Create one parent category per job; track its ID for product_categories linking.
	var categoryID int64
	// Track subcategory IDs for product assignment (English subcategory name → local ID).
	subcategoryMap := make(map[string]int64)
	// Track whether property definitions and group have been persisted (once per job).
	var propertyGroupID int64
	var propertyDefsProcessed bool
	// Track whether attribute definitions have been persisted (once per job).
	var attributeDefsProcessed bool
	// Attribute lookup maps: attrNameToID["Size"] = localID, attrValueNameToID["Size"]["M"] = localValueID
	attrNameToID := make(map[string]int64)
	attrValueNameToID := make(map[string]map[string]int64)

	// Generate products in batches to minimize API calls.
	batchSize, _ := cmd.Flags().GetInt("batch-size")
	if batchSize <= 0 {
		batchSize = cfg.AI.BatchSize
	}
	if batchSize <= 0 {
		batchSize = 5
	}

	var allProducts []*product.GeneratedProduct
	remaining := count
	for remaining > 0 {
		select {
		case <-ctx.Done():
			_ = q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
				Status: "failed",
				ID:     jobID,
			})
			return ctx.Err()
		default:
		}

		batchCount := batchSize
		if batchCount > remaining {
			batchCount = remaining
		}

		req := product.GenerationRequest{
			ProductType: niche,
			Niche:       niche,
			ProductName: "",
			Category:    niche,
		}

		products, err := prodGen.GenerateBatch(ctx, req, batchCount)
		if err != nil {
			_ = q.UpdateJobStatus(ctx, queries.UpdateJobStatusParams{
				Status: "failed",
				ID:     jobID,
			})
			return fmt.Errorf("batch generation failed: %w", err)
		}

		allProducts = append(allProducts, products...)
		remaining -= len(products)

		slog.Info("batch generated",
			"products", len(products),
			"total_so_far", len(allProducts),
			"remaining", remaining,
		)
	}

	// Persist each generated product.
	for i, result := range allProducts {
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

		// Create parent category and subcategories once per job.
		if categoryID == 0 {
			catID, err := q.CreateCategory(ctx, queries.CreateCategoryParams{
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
			categoryID = catID

			// Persist parent category translations with descriptions.
			for lang, name := range result.CategoryNames {
				if lang == "en" || name == "" {
					continue
				}
				desc := result.CategoryDescriptions[lang]
				if _, err := q.CreateCategoryTranslation(ctx, queries.CreateCategoryTranslationParams{
					CategoryID:  categoryID,
					Lang:        lang,
					Name:        name,
					Description: desc,
				}); err != nil {
					logger.Warn("failed to persist category translation",
						"lang", lang, "error", err)
				}
			}
			// Persist English description too.
			if enDesc, ok := result.CategoryDescriptions["en"]; ok && enDesc != "" {
				if _, err := q.CreateCategoryTranslation(ctx, queries.CreateCategoryTranslationParams{
					CategoryID:  categoryID,
					Lang:        "en",
					Name:        niche,
					Description: enDesc,
				}); err != nil {
					logger.Warn("failed to persist English category description", "error", err)
				}
			}

			// Create subcategories.
			for sortIdx, sub := range result.Subcategories {
				subID, err := q.CreateCategory(ctx, queries.CreateCategoryParams{
					JobID:     jobID,
					ParentID:  sql.NullInt64{Int64: categoryID, Valid: true},
					Name:      sub.Name,
					Level:     2,
					SortOrder: int32(sortIdx),
					Status:    string(domain.StatusPending),
				})
				if err != nil {
					logger.Warn("failed to create subcategory",
						"name", sub.Name, "error", err)
					continue
				}
				subcategoryMap[sub.Name] = subID

				// Persist subcategory translations with descriptions.
				for lang, name := range sub.Translations {
					desc := sub.Descriptions[lang]
					if _, err := q.CreateCategoryTranslation(ctx, queries.CreateCategoryTranslationParams{
						CategoryID:  subID,
						Lang:        lang,
						Name:        name,
						Description: desc,
					}); err != nil {
						logger.Warn("failed to persist subcategory translation",
							"subcategory", sub.Name, "lang", lang, "error", err)
					}
				}
				// Persist English description for subcategory.
				if enDesc, ok := sub.Descriptions["en"]; ok && enDesc != "" {
					if _, err := q.CreateCategoryTranslation(ctx, queries.CreateCategoryTranslationParams{
						CategoryID:  subID,
						Lang:        "en",
						Name:        sub.Name,
						Description: enDesc,
					}); err != nil {
						logger.Warn("failed to persist English subcategory description", "error", err)
					}
				}
			}

			logger.Info("created categories",
				"parent_id", categoryID,
				"subcategories", len(subcategoryMap),
			)
		}

		// Link product to its subcategory (or parent if no match).
		linkCategoryID := categoryID
		if result.Subcategory != "" {
			if subID, ok := subcategoryMap[result.Subcategory]; ok {
				linkCategoryID = subID
			}
		}
		if err := q.CreateProductCategory(ctx, queries.CreateProductCategoryParams{
			ProductID:  productID,
			CategoryID: linkCategoryID,
		}); err != nil {
			return fmt.Errorf("linking product to category: %w", err)
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

		// Persist attribute definitions once per job.
		if !attributeDefsProcessed && len(result.AttributeDefs) > 0 {
			attributeDefsProcessed = true
			for _, def := range result.AttributeDefs {
				attrID, attrErr := getOrCreateAttribute(ctx, q, jobID, def.Name)
				if attrErr != nil {
					logger.Warn("failed to get/create attribute",
						"attribute", def.Name, "error", attrErr)
					continue
				}
				attrNameToID[def.Name] = attrID
				attrValueNameToID[def.Name] = make(map[string]int64)

				// Create attribute values.
				for sortIdx, valName := range def.Values {
					valID, valErr := q.CreateAttributeValue(ctx, queries.CreateAttributeValueParams{
						AttributeID: attrID,
						Name:        valName,
						SortOrder:   int32(sortIdx),
					})
					if valErr != nil {
						logger.Warn("failed to create attribute value",
							"attribute", def.Name, "value", valName, "error", valErr)
						continue
					}
					attrValueNameToID[def.Name][valName] = valID

					// Persist value translations.
					for lang, trans := range def.Translations {
						if sortIdx < len(trans.Values) && trans.Values[sortIdx] != "" {
							if _, err := q.CreateAttributeValueTranslation(ctx, queries.CreateAttributeValueTranslationParams{
								AttributeValueID: valID,
								Lang:             lang,
								Name:             trans.Values[sortIdx],
							}); err != nil {
								logger.Warn("failed to persist attribute value translation",
									"attribute", def.Name, "value", valName,
									"lang", lang, "error", err)
							}
						}
					}
				}

				// Persist attribute name translations.
				for lang, trans := range def.Translations {
					if trans.Name == "" {
						continue
					}
					if _, err := q.CreateAttributeNameTranslation(ctx, queries.CreateAttributeNameTranslationParams{
						AttributeID: attrID,
						Lang:        lang,
						Name:        trans.Name,
					}); err != nil {
						logger.Warn("failed to persist attribute name translation",
							"attribute", def.Name, "lang", lang, "error", err)
					}
				}
			}
			logger.Info("persisted attribute definitions",
				"attribute_count", len(result.AttributeDefs),
			)
		}

		// Build common variation fields.
		skuPrefix := niche
		if len(skuPrefix) > 3 {
			skuPrefix = skuPrefix[:3]
		}
		variationCurrency := result.Currency
		if variationCurrency == "" {
			variationCurrency = "EUR"
		}
		var weightVal sql.NullString
		weightUnit := ""
		if result.Weight > 0 {
			weightVal = sql.NullString{String: fmt.Sprintf("%.3f", result.Weight), Valid: true}
			weightUnit = result.WeightUnit
			if weightUnit == "" {
				weightUnit = "kg"
			}
		}
		var rrpVal sql.NullString
		if result.RRP > 0 {
			rrpVal = sql.NullString{String: fmt.Sprintf("%.2f", result.RRP), Valid: true}
		}
		var b2bPriceVal sql.NullString
		if result.B2BPrice > 0 {
			b2bPriceVal = sql.NullString{String: fmt.Sprintf("%.2f", result.B2BPrice), Valid: true}
		}
		var b2bRrpVal sql.NullString
		if result.B2BRRP > 0 {
			b2bRrpVal = sql.NullString{String: fmt.Sprintf("%.2f", result.B2BRRP), Valid: true}
		}
		lengthMM := int32(result.LengthCM * 10)
		widthMM := int32(result.WidthCM * 10)
		heightMM := int32(result.HeightCM * 10)

		// Compute variation combos: cartesian product of attribute assignments, or single "Default".
		type attrCombo struct {
			name     string // e.g., "S / Black"
			attrVals []struct {
				attrName  string
				valueName string
			}
		}
		var combos []attrCombo

		if len(result.AttributeAssignments) > 0 {
			// Build cartesian product.
			combos = []attrCombo{{}}
			for _, assignment := range result.AttributeAssignments {
				var expanded []attrCombo
				for _, combo := range combos {
					for _, val := range assignment.Values {
						newCombo := attrCombo{
							attrVals: make([]struct {
								attrName  string
								valueName string
							}, len(combo.attrVals)+1),
						}
						copy(newCombo.attrVals, combo.attrVals)
						newCombo.attrVals[len(combo.attrVals)] = struct {
							attrName  string
							valueName string
						}{assignment.Name, val}
						expanded = append(expanded, newCombo)
					}
				}
				combos = expanded
			}
			// Set combo names and apply hard cap.
			const maxVariations = 50
			if len(combos) > maxVariations {
				logger.Warn("cartesian product too large, truncating",
					"product_id", productID,
					"combos", len(combos),
					"max", maxVariations,
				)
				combos = combos[:maxVariations]
			}
			for idx := range combos {
				parts := make([]string, len(combos[idx].attrVals))
				for j, av := range combos[idx].attrVals {
					parts[j] = av.valueName
				}
				combos[idx].name = strings.Join(parts, " / ")
			}
		} else {
			// No attributes — single "Default" variation.
			combos = []attrCombo{{name: "Default"}}
		}

		// Create one variation per combo.
		var firstVariationID int64
		for varIdx, combo := range combos {
			barcode, bErr := ean.GenerateEAN13()
			if bErr != nil {
				logger.Warn("failed to generate barcode", "error", bErr)
				barcode = ""
			}

			variationID, vErr := q.CreateVariation(ctx, queries.CreateVariationParams{
				ProductID:  productID,
				Name:       combo.name,
				Sku:        fmt.Sprintf("%s-%d-%d", skuPrefix, productID, varIdx),
				Price:      sql.NullString{String: fmt.Sprintf("%.2f", result.Price), Valid: true},
				Rrp:        rrpVal,
				B2bPrice:   b2bPriceVal,
				B2bRrp:     b2bRrpVal,
				Currency:   variationCurrency,
				Weight:     weightVal,
				WeightUnit: weightUnit,
				SalesUnit:  result.SalesUnit,
				LengthMm:   lengthMM,
				WidthMm:    widthMM,
				HeightMm:   heightMM,
				Barcode:    barcode,
				Model:      result.Model,
				Status:     string(domain.StatusPending),
			})
			if vErr != nil {
				return fmt.Errorf("creating variation %q: %w", combo.name, vErr)
			}
			if varIdx == 0 {
				firstVariationID = variationID
			}

			// Link variation to attribute values.
			for _, av := range combo.attrVals {
				attrID, aOk := attrNameToID[av.attrName]
				if !aOk {
					continue
				}
				valMap, vOk := attrValueNameToID[av.attrName]
				if !vOk {
					continue
				}
				valID, vvOk := valMap[av.valueName]
				if !vvOk {
					continue
				}
				if vaErr := q.CreateVariationAttribute(ctx, queries.CreateVariationAttributeParams{
					VariationID:      variationID,
					AttributeID:      attrID,
					AttributeValueID: valID,
				}); vaErr != nil {
					logger.Warn("failed to link variation to attribute",
						"variation_id", variationID,
						"attribute", av.attrName,
						"value", av.valueName,
						"error", vaErr,
					)
				}
			}

			// Insert graduated prices for each variation.
			for _, gp := range result.GraduatedPrices {
				if _, gpErr := q.CreateGraduatedPrice(ctx, queries.CreateGraduatedPriceParams{
					VariationID:     variationID,
					MinimumQuantity: int32(gp.MinimumQuantity),
					Price:           fmt.Sprintf("%.2f", gp.Price),
					Rrp:             fmt.Sprintf("%.2f", gp.RRP),
				}); gpErr != nil {
					logger.Warn("failed to insert graduated price",
						"variation_id", variationID,
						"min_qty", gp.MinimumQuantity,
						"error", gpErr,
					)
				}
			}

			// Persist selection properties on each variation.
			for _, pv := range result.PropertyVals {
				if pv.Name == "" || pv.Value == "" {
					continue
				}
				propID, propErr := getOrCreateProperty(ctx, q, jobID, pv.Name)
				if propErr != nil {
					continue
				}
				_, _ = q.CreateVariationProperty(ctx, queries.CreateVariationPropertyParams{
					VariationID: variationID,
					PropertyID:  propID,
					ValueText:   sql.NullString{String: pv.Value, Valid: true},
				})
			}
		}

		logger.Info("created variations",
			"product_id", productID,
			"variation_count", len(combos),
			"first_variation_id", firstVariationID,
		)
		_ = firstVariationID // suppress unused warning if needed

		// Persist property translations and group once per job (from first product in batch).
		if !propertyDefsProcessed && len(result.PropertyDefs) > 0 {
			propertyDefsProcessed = true

			// Create property group for this job.
			pgID, pgErr := q.CreatePropertyGroup(ctx, queries.CreatePropertyGroupParams{
				JobID:  jobID,
				Name:   niche,
				Status: string(domain.StatusPending),
			})
			if pgErr != nil {
				logger.Warn("failed to create property group", "error", pgErr)
			} else {
				propertyGroupID = pgID
				// Persist group translations from category names (same niche translations).
				for lang, name := range result.CategoryNames {
					if _, err := q.CreatePropertyGroupTranslation(ctx, queries.CreatePropertyGroupTranslationParams{
						PropertyGroupID: propertyGroupID,
						Lang:            lang,
						Name:            name,
					}); err != nil {
						logger.Warn("failed to persist property group translation",
							"lang", lang, "error", err)
					}
				}
			}

			// Persist property name and option translations for each definition.
			for _, def := range result.PropertyDefs {
				propID, propErr := getOrCreateProperty(ctx, q, jobID, def.Name)
				if propErr != nil {
					logger.Warn("failed to get/create property for translations",
						"property", def.Name, "error", propErr)
					continue
				}

				// Link property to group.
				if propertyGroupID > 0 {
					_ = q.UpdatePropertyGroupID(ctx, queries.UpdatePropertyGroupIDParams{
						PropertyGroupID: sql.NullInt64{Int64: propertyGroupID, Valid: true},
						ID:              propID,
					})
				}

				// Persist name translations.
				for lang, trans := range def.Translations {
					if trans.Name == "" {
						continue
					}
					if _, err := q.CreatePropertyNameTranslation(ctx, queries.CreatePropertyNameTranslationParams{
						PropertyID: propID,
						Lang:       lang,
						Name:       trans.Name,
					}); err != nil {
						logger.Warn("failed to persist property name translation",
							"property", def.Name, "lang", lang, "error", err)
					}

					// Persist option translations.
					for optIdx, optName := range trans.Options {
						if optIdx >= len(def.Options) || optName == "" {
							continue
						}
						if _, err := q.CreatePropertyOptionTranslation(ctx, queries.CreatePropertyOptionTranslationParams{
							PropertyID: propID,
							OptionKey:  def.Options[optIdx],
							Lang:       lang,
							Name:       optName,
						}); err != nil {
							logger.Warn("failed to persist property option translation",
								"property", def.Name, "option", def.Options[optIdx],
								"lang", lang, "error", err)
						}
					}
				}
			}
		}

		// Step A: Generate AI images.
		imagePosition := 0
		for imgIdx := 0; imgIdx < cfg.Images.PerProduct; imgIdx++ {
			imgReq := generate.ImageRequest{
				ProductName: productName,
				ProductType: niche,
				Category:    niche,
				Style:       "product photography, white background, studio lighting",
				Size:        cfg.Images.Size,
				Quality:     cfg.Images.Quality,
			}
			imgResult, err := imageGen.GenerateProductImage(ctx, imgReq)
			if err != nil {
				logger.Warn("AI image generation failed, continuing", "error", err, "product", productName)
				break
			}
			// Save base64 image to local file.
			imgDir := filepath.Join("data", "images", fmt.Sprintf("job-%d", jobID))
			if mkErr := os.MkdirAll(imgDir, 0o755); mkErr != nil {
				return fmt.Errorf("creating image directory: %w", mkErr)
			}
			imgPath := filepath.Join(imgDir, fmt.Sprintf("product-%d-ai-%d.%s", productID, imgIdx, imgResult.Format))
			imgBytes, decErr := base64.StdEncoding.DecodeString(imgResult.Base64Data)
			if decErr != nil {
				logger.Warn("failed to decode AI image base64", "error", decErr)
				break
			}
			if wErr := os.WriteFile(imgPath, imgBytes, 0o644); wErr != nil {
				return fmt.Errorf("writing image file: %w", wErr)
			}

			if _, err := q.CreateImage(ctx, queries.CreateImageParams{
				ProductID:   productID,
				SourceUrl:   "",
				LocalPath:   imgPath,
				Position:    int32(imagePosition),
				SourceType:  "ai-generated",
				Attribution: sql.NullString{},
				Status:      string(domain.StatusPending),
			}); err != nil {
				return fmt.Errorf("creating AI image record: %w", err)
			}
			imagePosition++
		}
		hasAIImage := imagePosition > 0

		// Step B: Source stock photos.
		hasStockPhoto := false
		if len(imageSources) > 0 {
			searchQuery := productName + " " + niche
			for _, source := range imageSources {
				if imagePosition >= cfg.Images.PerProduct+cfg.StockPhotos.PerProduct {
					break
				}
				photos, sErr := source.Search(ctx, searchQuery, imagesource.SearchOptions{
					Page:        1,
					PerPage:     cfg.StockPhotos.PerProduct,
					Orientation: cfg.StockPhotos.Orientation,
					MinWidth:    cfg.StockPhotos.MinWidth,
					MinHeight:   cfg.StockPhotos.MinHeight,
				})
				if sErr != nil {
					logger.Warn("stock photo search failed", "source", source.Name(), "error", sErr)
					continue
				}
				for _, photo := range photos {
					if imagePosition >= cfg.Images.PerProduct+cfg.StockPhotos.PerProduct {
						break
					}
					if _, err := q.CreateImage(ctx, queries.CreateImageParams{
						ProductID:   productID,
						SourceUrl:   photo.DownloadURL,
						LocalPath:   "",
						Position:    int32(imagePosition),
						SourceType:  photo.SourceName,
						Attribution: sql.NullString{String: photo.Attribution, Valid: photo.Attribution != ""},
						Status:      string(domain.StatusPending),
					}); err != nil {
						return fmt.Errorf("creating stock photo record: %w", err)
					}
					imagePosition++
					hasStockPhoto = true
				}
				if hasStockPhoto {
					break // Got photos from first available source.
				}
			}
		}

		// Step C: Enrich from public databases.
		enrichmentFields := make(map[string]string)
		for _, enricher := range enrichers {
			enrichReq := enrichment.EnrichmentRequest{
				ProductName: productName,
				ProductType: niche,
				Category:    niche,
			}
			enrichResult, eErr := enricher.Enrich(ctx, enrichReq)
			if eErr != nil {
				logger.Warn("enrichment failed", "source", enricher.Name(), "error", eErr)
				continue
			}
			for k, v := range enrichResult.Fields {
				enrichmentFields[enricher.Name()+"_"+k] = v
			}
		}

		// Step D: Score quality.
		if scorer != nil {
			input := &quality.ScoringInput{
				ProductName:      productName,
				ProductType:      niche,
				Texts:            result.Texts,
				ImageCount:       imagePosition,
				HasAIImage:       hasAIImage,
				HasStockPhoto:    hasStockPhoto,
				EnrichmentFields: enrichmentFields,
				Languages:        cfg.AI.Languages,
			}
			report := scorer.Score(input)

			// Persist quality score.
			detailsJSON, _ := json.Marshal(report.RuleResults)
			if _, qErr := q.CreateQualityScore(ctx, queries.CreateQualityScoreParams{
				ProductID:  productID,
				JobID:      jobID,
				Overall:    fmt.Sprintf("%.4f", report.OverallScore),
				TextScore:  fmt.Sprintf("%.4f", report.TextScore),
				ImageScore: fmt.Sprintf("%.4f", report.ImageScore),
				DataScore:  fmt.Sprintf("%.4f", report.DataScore),
				Pass:       report.Pass,
				Details:    json.RawMessage(detailsJSON),
			}); qErr != nil {
				return fmt.Errorf("creating quality score: %w", qErr)
			}

			if !report.Pass {
				switch cfg.Quality.FlagAction {
				case "warn":
					logger.Warn("product below quality threshold",
						"product", productName,
						"score", report.OverallScore,
						"flags", report.Flags,
					)
				case "skip":
					logger.Warn("skipping low-quality product",
						"product", productName,
						"score", report.OverallScore,
					)
					continue
				case "fail":
					return fmt.Errorf("product %q failed quality check: score %.2f, flags: %v",
						productName, report.OverallScore, report.Flags)
				}
			}
		}

		slog.Info("generated product",
			"index", i+1, "total", count, "job_id", jobID,
			"images", imagePosition,
			"enrichment_fields", len(enrichmentFields),
		)
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
		BaseURL:   cfg.API.EffectiveBaseURL(),
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
	"api_key":      true,
	"password":     true,
	"username":     true,
	"unsplash_key": true,
	"pexels_key":   true,
	"pixabay_key":  true,
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

// getOrCreateProperty returns the local property ID for the given job and
// property name, creating a new selection-type property record if needed.
func getOrCreateProperty(ctx context.Context, q *queries.Queries, jobID int64, name string) (int64, error) {
	prop, err := q.GetPropertyByJobAndName(ctx, queries.GetPropertyByJobAndNameParams{
		JobID: jobID,
		Name:  name,
	})
	if err == nil {
		return prop.ID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("looking up property %q: %w", name, err)
	}
	// Property doesn't exist yet — create it.
	propID, err := q.CreateProperty(ctx, queries.CreatePropertyParams{
		JobID:        jobID,
		Name:         name,
		PropertyType: "selection",
		Status:       string(domain.StatusPending),
	})
	if err != nil {
		return 0, fmt.Errorf("creating property %q: %w", name, err)
	}
	return propID, nil
}

// getOrCreateAttribute returns the local attribute ID for the given job and
// name, creating it if it doesn't exist yet.
func getOrCreateAttribute(ctx context.Context, q *queries.Queries, jobID int64, name string) (int64, error) {
	attr, err := q.GetAttributeByJobAndName(ctx, queries.GetAttributeByJobAndNameParams{
		JobID: jobID,
		Name:  name,
	})
	if err == nil {
		return attr.ID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("looking up attribute %q: %w", name, err)
	}
	// Attribute doesn't exist yet — create it.
	attrID, err := q.CreateAttribute(ctx, queries.CreateAttributeParams{
		JobID:    jobID,
		Name:     name,
		AttrType: "selection",
		Status:   string(domain.StatusPending),
	})
	if err != nil {
		return 0, fmt.Errorf("creating attribute %q: %w", name, err)
	}
	return attrID, nil
}

var addPropertiesCmd = &cobra.Command{
	Use:   "add-properties",
	Short: "Add selection properties to variations that don't have any",
	Long: `Finds all variations in a job that have no properties and generates
selection-type property values for them using the configured AI provider.
If the job already has property definitions from a previous generation run,
those are reused. Otherwise, new definitions are generated automatically.`,
	RunE: runAddProperties,
}

func runAddProperties(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.Default()

	jobID, _ := cmd.Flags().GetInt64("job-id")
	provider, _ := cmd.Flags().GetString("provider")

	// Open DB connection.
	db, err := storage.NewDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer db.Close()
	q := queries.New(db)

	// Verify job exists and extract product type from config.
	job, err := q.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("getting job %d: %w", jobID, err)
	}

	var jobConfig struct {
		Niche string `json:"niche"`
	}
	if err := json.Unmarshal(job.Config, &jobConfig); err != nil {
		return fmt.Errorf("parsing job config: %w", err)
	}
	productType := jobConfig.Niche
	if productType == "" {
		productType = "general"
	}

	// Find variations without properties.
	variations, err := q.ListVariationsWithoutPropertiesByJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("listing variations without properties: %w", err)
	}

	if len(variations) == 0 {
		fmt.Println("All variations already have properties.")
		return nil
	}

	logger.Info("found variations without properties",
		"count", len(variations),
		"job_id", jobID,
	)

	// Override provider if flag is set.
	if provider != "" {
		cfg.AI.Provider = provider
	}

	// Create AI generator.
	gen, err := app.NewGeneratorFromConfig(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating AI generator: %w", err)
	}

	// Resolve property definitions: either reuse existing or generate new ones.
	propertySpecs, err := resolvePropertySpecs(ctx, q, gen, jobID, productType, logger)
	if err != nil {
		return fmt.Errorf("resolving property definitions: %w", err)
	}

	if len(propertySpecs) == 0 {
		return fmt.Errorf("no property definitions could be resolved for job %d", jobID)
	}

	logger.Info("resolved property definitions",
		"count", len(propertySpecs),
		"names", propertySpecNames(propertySpecs),
	)

	// Group variations by product.
	type productVariations struct {
		productName string
		productType string
		variationIDs []int64
	}
	byProduct := make(map[int64]*productVariations)
	for _, v := range variations {
		pv, ok := byProduct[v.ProductID]
		if !ok {
			pv = &productVariations{
				productName: v.ProductName,
				productType: v.ProductType,
			}
			byProduct[v.ProductID] = pv
		}
		pv.variationIDs = append(pv.variationIDs, v.VariationID)
	}

	// Generate property values per product and persist.
	var totalAdded int
	for productID, pv := range byProduct {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		propReq := generate.PropertyValueRequest{
			ProductType: pv.productType,
			ProductName: pv.productName,
			Properties:  propertySpecs,
			Language:    "en", // selection properties are language-independent
		}

		propVals, err := gen.GeneratePropertyValues(ctx, propReq)
		if err != nil {
			logger.Warn("failed to generate property values",
				"product_id", productID,
				"product_name", pv.productName,
				"error", err,
			)
			continue
		}

		// Persist the same property values for each variation of this product.
		for _, varID := range pv.variationIDs {
			added := 0
			for _, val := range propVals.Values {
				// Find the spec name by property ID.
				specName := ""
				selectionValue := val.SelectionValue
				for _, s := range propertySpecs {
					if s.ID == val.PropertyID {
						specName = s.Name
						break
					}
				}
				if specName == "" || selectionValue == "" {
					continue
				}

				propID, propErr := getOrCreateProperty(ctx, q, jobID, specName)
				if propErr != nil {
					logger.Warn("failed to get/create property",
						"property", specName,
						"error", propErr,
					)
					continue
				}

				if _, vpErr := q.CreateVariationProperty(ctx, queries.CreateVariationPropertyParams{
					VariationID: varID,
					PropertyID:  propID,
					ValueText:   sql.NullString{String: selectionValue, Valid: true},
				}); vpErr != nil {
					logger.Warn("failed to insert variation property",
						"variation_id", varID,
						"property", specName,
						"value", selectionValue,
						"error", vpErr,
					)
					continue
				}
				added++
			}
			totalAdded += added
		}

		logger.Info("added properties to product",
			"product_id", productID,
			"product_name", pv.productName,
			"variations", len(pv.variationIDs),
		)
	}

	fmt.Printf("Added %d property values across %d variations (job ID: %d)\n",
		totalAdded, len(variations), jobID)
	return nil
}

// resolvePropertySpecs builds PropertySpecs for the add-properties command.
// If the job already has property definitions, their names and distinct values
// are used. Otherwise, a mini batch generation is run to discover appropriate
// property definitions for the product type.
func resolvePropertySpecs(ctx context.Context, q *queries.Queries, gen generate.Generator, jobID int64, productType string, logger *slog.Logger) ([]generate.PropertySpec, error) {
	// Check for existing property definitions.
	existingProps, err := q.ListPropertiesByJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("listing properties for job %d: %w", jobID, err)
	}

	if len(existingProps) > 0 {
		logger.Info("reusing existing property definitions",
			"count", len(existingProps),
		)

		specs := make([]generate.PropertySpec, 0, len(existingProps))
		for _, prop := range existingProps {
			// Get existing option values for this property.
			distinctVals, err := q.ListDistinctPropertyValues(ctx, prop.ID)
			if err != nil {
				logger.Warn("failed to load distinct values for property",
					"property_id", prop.ID,
					"property_name", prop.Name,
					"error", err,
				)
				continue
			}

			options := make([]string, 0, len(distinctVals))
			for _, v := range distinctVals {
				if v.Valid && v.String != "" {
					options = append(options, v.String)
				}
			}

			specs = append(specs, generate.PropertySpec{
				ID:           prop.ID,
				Name:         prop.Name,
				PropertyType: "selection",
				Options:      options,
			})
		}
		return specs, nil
	}

	// No existing properties — generate definitions via a mini batch call.
	logger.Info("no existing property definitions, generating via batch call")

	batcher, ok := gen.(generate.BatchGenerator)
	if !ok {
		return nil, fmt.Errorf("provider %q does not support batch generation; cannot auto-generate property definitions", gen.Name())
	}

	batchReq := generate.BatchRequest{
		ProductType: productType,
		Category:    productType,
		Niche:       productType,
		Count:       1,
		Languages:   cfg.AI.Languages,
		Currency:    "EUR",
	}

	result, err := batcher.GenerateBatch(ctx, batchReq)
	if err != nil {
		return nil, fmt.Errorf("mini batch generation for property defs: %w", err)
	}

	if len(result.Properties) == 0 {
		return nil, fmt.Errorf("batch generation returned no property definitions")
	}

	// Store definitions in the properties table and build specs.
	// Also persist translations from the batch result.
	specs := make([]generate.PropertySpec, 0, len(result.Properties))
	for _, def := range result.Properties {
		propID, err := getOrCreateProperty(ctx, q, jobID, def.Name)
		if err != nil {
			logger.Warn("failed to store property definition",
				"name", def.Name,
				"error", err,
			)
			continue
		}

		// Persist name and option translations.
		for lang, trans := range def.Translations {
			if trans.Name != "" {
				if _, tErr := q.CreatePropertyNameTranslation(ctx, queries.CreatePropertyNameTranslationParams{
					PropertyID: propID,
					Lang:       lang,
					Name:       trans.Name,
				}); tErr != nil {
					logger.Warn("failed to persist property name translation",
						"property", def.Name, "lang", lang, "error", tErr)
				}
			}
			for optIdx, optName := range trans.Options {
				if optIdx >= len(def.Options) || optName == "" {
					continue
				}
				if _, tErr := q.CreatePropertyOptionTranslation(ctx, queries.CreatePropertyOptionTranslationParams{
					PropertyID: propID,
					OptionKey:  def.Options[optIdx],
					Lang:       lang,
					Name:       optName,
				}); tErr != nil {
					logger.Warn("failed to persist property option translation",
						"property", def.Name, "option", def.Options[optIdx],
						"lang", lang, "error", tErr)
				}
			}
		}

		specs = append(specs, generate.PropertySpec{
			ID:           propID,
			Name:         def.Name,
			PropertyType: "selection",
			Options:      def.Options,
		})
	}

	logger.Info("generated property definitions from batch",
		"count", len(specs),
	)

	return specs, nil
}

// propertySpecNames returns the names of property specs for logging.
func propertySpecNames(specs []generate.PropertySpec) string {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
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
	rootCmd.AddCommand(addPropertiesCmd)

	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)

	configCmd.AddCommand(configSetCmd)

	migrateCmd.PersistentFlags().Bool("dry-run", false, "print DSN and exit without running migrations")

	generateCmd.Flags().String("niche", "", "product niche (e.g., electronics, fashion)")
	generateCmd.Flags().Int("count", 10, "number of products to generate")
	generateCmd.Flags().String("provider", "", "AI provider override (mock, openai, gemini, claude)")
	generateCmd.Flags().Int("batch-size", 0, "products per batch API call (default from config or 5)")
	_ = generateCmd.MarkFlagRequired("niche")

	pushCmd.Flags().Int64("job-id", 0, "job ID to push")
	pushCmd.Flags().Bool("dry-run", false, "simulate pipeline without making API calls")
	pushCmd.Flags().Bool("resume", false, "resume the latest pipeline run for this job")
	pushCmd.Flags().Bool("reset-failed", false, "reset failed entity mappings before resuming")
	_ = pushCmd.MarkFlagRequired("job-id")

	statusCmd.Flags().Int64("job-id", 0, "filter by job ID")
	statusCmd.Flags().Int64("run-id", 0, "filter by pipeline run ID")

	addPropertiesCmd.Flags().Int64("job-id", 0, "job ID to add properties to")
	addPropertiesCmd.Flags().String("provider", "", "AI provider override (mock, openai, gemini, claude)")
	_ = addPropertiesCmd.MarkFlagRequired("job-id")
}
