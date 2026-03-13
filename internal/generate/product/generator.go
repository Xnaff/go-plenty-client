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
	Price         float64                               // AI-generated price
	Currency      string                                // Price currency (e.g., "EUR")
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

	g.logger.Info("generated price",
		"provider", g.provider.Name(),
		"price", result.Price,
		"currency", result.Currency,
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
