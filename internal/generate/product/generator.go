package product

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/janemig/plentyone/internal/generate"
	"github.com/janemig/plentyone/internal/generate/validate"
)

// Generator orchestrates multilingual text generation, property value
// generation, and output validation. It ties together the AI provider and the
// validation layer.
type Generator struct {
	provider  generate.Generator
	validator *validate.Validator
	languages []string
	logger    *slog.Logger
}

// NewGenerator creates a product Generator with the given components.
func NewGenerator(provider generate.Generator, validator *validate.Validator, languages []string, logger *slog.Logger) *Generator {
	return &Generator{
		provider:  provider,
		validator: validator,
		languages: languages,
		logger:    logger,
	}
}

// GenerationRequest describes a complete product generation request.
type GenerationRequest struct {
	ProductType string
	ProductName string
	Category    string
	Niche       string
	Keywords    []string
	Properties  []generate.PropertySpec
}

// GeneratedProduct is the complete output of generation for one product.
type GeneratedProduct struct {
	Texts         map[string]*generate.ProductTexts    // lang -> product texts
	PropertyTexts map[string]*generate.PropertyValues   // lang -> text-type property values
	Properties    *generate.PropertyValues              // non-text property values (language-independent)
	PropertyDefs  []generate.BatchPropertyDefinition    // Shared property definitions from batch
	PropertyVals  []generate.BatchProductProperty       // This product's selection property values
	CategoryNames        map[string]string                     // Translated category names (lang -> name) from batch
	CategoryDescriptions map[string]string                     // Translated category descriptions (lang -> description) from batch
	Subcategories        []generate.BatchSubcategory           // Subcategory definitions from batch
	Subcategory          string                                // This product's subcategory (English name)
	AttributeDefs        []generate.BatchAttributeDefinition           // Shared attribute definitions from batch
	AttributeAssignments []generate.BatchProductAttributeAssignment    // This product's attribute assignments for variations
	Price         float64                               // AI-generated selling price
	Currency      string                                // Price currency (e.g., "EUR")
	RRP             float64                             // Recommended retail price
	B2BPrice        float64                             // B2B wholesale selling price
	B2BRRP          float64                             // B2B recommended retail price
	Model           string                              // Product model identifier
	Weight          float64                             // Product weight
	WeightUnit      string                              // Weight unit: "kg" or "g"
	SalesUnit       string                              // How the product is sold: "piece", "kg", etc.
	LengthCM        int                                 // Product length in cm
	WidthCM         int                                 // Product width in cm
	HeightCM        int                                 // Product height in cm
	GraduatedPrices []generate.GraduatedPrice           // Optional quantity-based price tiers (with RRP per tier)
	Warnings      []validate.ValidationError
}

// Generate orchestrates full product generation across all configured languages.
// For each language, it generates text fields and validates them. Properties are
// split by type: text-type properties are generated per-language, non-text
// properties (int, float, selection) are generated once.
func (g *Generator) Generate(ctx context.Context, req GenerationRequest) (*GeneratedProduct, error) {
	result := &GeneratedProduct{
		Texts: make(map[string]*generate.ProductTexts, len(g.languages)),
	}

	// 1. Generate and validate texts for each language.
	for _, lang := range g.languages {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		textReq := generate.ProductTextRequest{
			ProductType: req.ProductType,
			ProductName: req.ProductName,
			Category:    req.Category,
			Language:    lang,
			Keywords:    req.Keywords,
			Niche:       req.Niche,
		}

		texts, warnings, err := g.GenerateTexts(ctx, textReq)
		if err != nil {
			return nil, fmt.Errorf("generate texts for %s: %w", lang, err)
		}

		result.Texts[lang] = texts
		result.Warnings = append(result.Warnings, warnings...)
	}

	// 2. Generate price.
	priceReq := generate.PriceRequest{
		ProductType: req.ProductType,
		ProductName: req.ProductName,
		Category:    req.Category,
		Niche:       req.Niche,
		Currency:    "EUR",
	}
	priceResult, err := g.provider.GeneratePrice(ctx, priceReq)
	if err != nil {
		return nil, fmt.Errorf("generate price: %w", err)
	}
	result.Price = priceResult.Price
	result.Currency = priceResult.Currency
	if result.Currency == "" {
		result.Currency = "EUR"
	}
	result.RRP = priceResult.RRP
	result.B2BPrice = priceResult.B2BPrice
	result.B2BRRP = priceResult.B2BRRP
	result.Model = priceResult.Model
	result.Weight = priceResult.Weight
	result.WeightUnit = priceResult.WeightUnit
	result.SalesUnit = priceResult.SalesUnit
	result.LengthCM = priceResult.LengthCM
	result.WidthCM = priceResult.WidthCM
	result.HeightCM = priceResult.HeightCM
	result.GraduatedPrices = priceResult.GraduatedPrices

	g.logger.Info("generated price",
		"provider", g.provider.Name(),
		"price", result.Price,
		"rrp", result.RRP,
		"b2b_price", result.B2BPrice,
		"b2b_rrp", result.B2BRRP,
		"model", result.Model,
		"currency", result.Currency,
		"weight", result.Weight,
		"weight_unit", result.WeightUnit,
		"sales_unit", result.SalesUnit,
		"dimensions_cm", fmt.Sprintf("%dx%dx%d", result.LengthCM, result.WidthCM, result.HeightCM),
		"graduated_tiers", len(result.GraduatedPrices),
	)

	// 3. Handle properties if provided.
	if len(req.Properties) > 0 {
		// Split properties by type.
		var textProps []generate.PropertySpec
		var nonTextProps []generate.PropertySpec
		for _, p := range req.Properties {
			if p.PropertyType == "text" {
				textProps = append(textProps, p)
			} else {
				nonTextProps = append(nonTextProps, p)
			}
		}

		// 2a. Text-type properties: generate per language.
		if len(textProps) > 0 {
			result.PropertyTexts = make(map[string]*generate.PropertyValues, len(g.languages))
			for _, lang := range g.languages {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				propReq := generate.PropertyValueRequest{
					ProductType: req.ProductType,
					ProductName: req.ProductName,
					Properties:  textProps,
					Language:    lang,
				}

				propVals, err := g.provider.GeneratePropertyValues(ctx, propReq)
				if err != nil {
					return nil, fmt.Errorf("generate text properties for %s: %w", lang, err)
				}

				validated, propErrs := g.validator.ValidatePropertyValues(propVals, textProps)
				if validate.HasErrors(propErrs) {
					errors := validate.ErrorsOnly(propErrs)
					return nil, fmt.Errorf("text property validation failed for %s: %v", lang, errors)
				}
				result.Warnings = append(result.Warnings, validate.WarningsOnly(propErrs)...)
				result.PropertyTexts[lang] = validated

				g.logger.Info("generated text property values",
					"language", lang,
					"provider", g.provider.Name(),
					"count", len(textProps),
				)
			}
		}

		// 2b. Non-text properties: generate once (language-independent).
		if len(nonTextProps) > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			propReq := generate.PropertyValueRequest{
				ProductType: req.ProductType,
				ProductName: req.ProductName,
				Properties:  nonTextProps,
				Language:    "en", // Non-text properties are language-independent; use English for context.
			}

			propVals, err := g.provider.GeneratePropertyValues(ctx, propReq)
			if err != nil {
				return nil, fmt.Errorf("generate non-text properties: %w", err)
			}

			validated, propErrs := g.validator.ValidatePropertyValues(propVals, nonTextProps)
			if validate.HasErrors(propErrs) {
				errors := validate.ErrorsOnly(propErrs)
				return nil, fmt.Errorf("property validation failed: %v", errors)
			}
			result.Warnings = append(result.Warnings, validate.WarningsOnly(propErrs)...)
			result.Properties = validated

			g.logger.Info("generated non-text property values",
				"provider", g.provider.Name(),
				"count", len(nonTextProps),
			)
		}
	}

	return result, nil
}

// GenerateBatch generates multiple products in a single operation.
// If the underlying provider implements BatchGenerator, it uses a single
// API call for all products. Otherwise, it falls back to calling Generate
// per product sequentially.
func (g *Generator) GenerateBatch(ctx context.Context, req GenerationRequest, count int) ([]*GeneratedProduct, error) {
	batcher, ok := g.provider.(generate.BatchGenerator)
	if !ok {
		// Fallback: generate one at a time.
		return g.generateBatchSequential(ctx, req, count)
	}

	batchReq := generate.BatchRequest{
		ProductType: req.ProductType,
		Category:    req.Category,
		Niche:       req.Niche,
		Count:       count,
		Languages:   g.languages,
		Currency:    "EUR",
	}

	result, err := batcher.GenerateBatch(ctx, batchReq)
	if err != nil {
		return nil, fmt.Errorf("batch generation: %w", err)
	}

	if len(result.Products) == 0 {
		return nil, fmt.Errorf("batch generation returned zero products")
	}

	if len(result.Products) < count {
		g.logger.Warn("batch generation returned fewer products than requested",
			"requested", count,
			"received", len(result.Products),
		)
	}

	// Convert and validate each product.
	products := make([]*GeneratedProduct, 0, len(result.Products))
	for i, bp := range result.Products {
		gp, err := g.convertBatchProduct(bp, result.Properties, result.CategoryName, result.CategoryDescriptions, result.Subcategories, result.Attributes)
		if err != nil {
			g.logger.Warn("skipping invalid product from batch",
				"index", i,
				"error", err,
			)
			continue
		}
		products = append(products, gp)
	}

	if len(products) == 0 {
		return nil, fmt.Errorf("all products in batch failed validation")
	}

	g.logger.Info("batch generation complete",
		"provider", g.provider.Name(),
		"requested", count,
		"valid", len(products),
	)

	return products, nil
}

// convertBatchProduct converts a BatchProduct to GeneratedProduct and validates
// all text fields per language. Property definitions from the batch result are
// attached to each product for downstream persistence.
func (g *Generator) convertBatchProduct(bp generate.BatchProduct, propertyDefs []generate.BatchPropertyDefinition, categoryNames map[string]string, categoryDescriptions map[string]string, subcategories []generate.BatchSubcategory, attributeDefs []generate.BatchAttributeDefinition) (*GeneratedProduct, error) {
	gp := &GeneratedProduct{
		Texts:                make(map[string]*generate.ProductTexts, len(bp.Texts)),
		PropertyDefs:         propertyDefs,
		PropertyVals:         bp.Properties,
		CategoryNames:        categoryNames,
		CategoryDescriptions: categoryDescriptions,
		Subcategories:        subcategories,
		Subcategory:          bp.Subcategory,
		AttributeDefs:        attributeDefs,
		AttributeAssignments: bp.Attributes,
		Price:                bp.Price,
		Currency:        bp.Currency,
		RRP:             bp.RRP,
		B2BPrice:        bp.B2BPrice,
		B2BRRP:          bp.B2BRRP,
		Model:           bp.Model,
		Weight:          bp.Weight,
		WeightUnit:      bp.WeightUnit,
		SalesUnit:       bp.SalesUnit,
		LengthCM:        bp.LengthCM,
		WidthCM:         bp.WidthCM,
		HeightCM:        bp.HeightCM,
		GraduatedPrices: bp.GraduatedPrices,
	}

	if gp.Currency == "" {
		gp.Currency = "EUR"
	}

	for _, bt := range bp.Texts {
		texts := bt.ToProductTexts()

		validated, errs := g.validator.ValidateProductTexts(texts, bt.Language)
		if validate.HasErrors(errs) {
			return nil, fmt.Errorf("validation failed for language %s: %v", bt.Language, validate.ErrorsOnly(errs))
		}
		gp.Warnings = append(gp.Warnings, validate.WarningsOnly(errs)...)
		gp.Texts[bt.Language] = validated
	}

	// Verify we got texts for all configured languages.
	for _, lang := range g.languages {
		if _, ok := gp.Texts[lang]; !ok {
			return nil, fmt.Errorf("missing texts for language %s", lang)
		}
	}

	g.logger.Info("generated product (batch)",
		"provider", g.provider.Name(),
		"price", gp.Price,
		"model", gp.Model,
		"dimensions_cm", fmt.Sprintf("%dx%dx%d", gp.LengthCM, gp.WidthCM, gp.HeightCM),
		"languages", len(gp.Texts),
		"properties", len(gp.PropertyVals),
	)

	return gp, nil
}

// generateBatchSequential falls back to per-product generation when the
// provider does not implement BatchGenerator.
func (g *Generator) generateBatchSequential(ctx context.Context, req GenerationRequest, count int) ([]*GeneratedProduct, error) {
	products := make([]*GeneratedProduct, 0, count)

	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		gp, err := g.Generate(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("generating product %d/%d: %w", i+1, count, err)
		}
		products = append(products, gp)
	}

	return products, nil
}

// GenerateTexts generates and validates product texts for a single language.
// This is the lower-level method that Generate() calls internally, but it is
// also exported for fine-grained use by the pipeline.
func (g *Generator) GenerateTexts(ctx context.Context, req generate.ProductTextRequest) (*generate.ProductTexts, []validate.ValidationError, error) {
	texts, err := g.provider.GenerateProductTexts(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("ai generation failed: %w", err)
	}

	validated, errs := g.validator.ValidateProductTexts(texts, req.Language)
	if validate.HasErrors(errs) {
		errors := validate.ErrorsOnly(errs)
		return nil, nil, fmt.Errorf("validation failed for %s: %v", req.Language, errors)
	}

	warnings := validate.WarningsOnly(errs)
	for _, w := range warnings {
		g.logger.Warn("validation warning",
			"field", w.Field,
			"code", w.Code,
			"message", w.Message,
			"language", req.Language,
		)
	}

	g.logger.Info("generated product texts",
		"language", req.Language,
		"provider", g.provider.Name(),
	)

	return validated, warnings, nil
}
