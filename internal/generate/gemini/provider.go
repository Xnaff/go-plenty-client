// Package gemini implements the generate.Generator and generate.ImageGenerator
// interfaces using Google's Gemini API via the google.golang.org/genai SDK.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"google.golang.org/genai"

	"github.com/janemig/plentyone/internal/generate"
)

// Compile-time checks.
var _ generate.Generator = (*Provider)(nil)
var _ generate.BatchGenerator = (*Provider)(nil)

// Provider implements generate.Generator using the Gemini API with
// structured JSON output for guaranteed schema compliance.
type Provider struct {
	client *genai.Client
	model  string
	logger *slog.Logger
}

// NewProvider creates a Gemini Generator provider.
func NewProvider(client *genai.Client, model string, logger *slog.Logger) *Provider {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Provider{
		client: client,
		model:  model,
		logger: logger,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "gemini" }

// GenerateProductTexts generates all text fields for a product in a single language
// using the Gemini API with structured JSON output.
func (p *Provider) GenerateProductTexts(ctx context.Context, req generate.ProductTextRequest) (*generate.ProductTexts, error) {
	systemPrompt := generate.SystemPromptForLanguage(req.Language)
	userPrompt := generate.BuildProductTextPrompt(req)

	p.logger.Debug("generating product texts",
		"provider", "gemini",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
	)

	resp, err := p.client.Models.GenerateContent(ctx, p.model,
		genai.Text(userPrompt),
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: systemPrompt}},
			},
			ResponseMIMEType:   "application/json",
			ResponseJsonSchema: productTextsSchema,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("gemini generate product texts: %w", err)
	}

	var result generate.ProductTexts
	if err := json.Unmarshal([]byte(resp.Text()), &result); err != nil {
		return nil, fmt.Errorf("gemini parse product texts: %w", err)
	}

	p.logger.Debug("product texts generated",
		"provider", "gemini",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
		"name_length", len(result.Name),
	)

	return &result, nil
}

// GeneratePropertyValues generates property values for a product using the
// Gemini API with structured JSON output.
func (p *Provider) GeneratePropertyValues(ctx context.Context, req generate.PropertyValueRequest) (*generate.PropertyValues, error) {
	userPrompt := generate.BuildPropertyValuePrompt(req)

	p.logger.Debug("generating property values",
		"provider", "gemini",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
		"property_count", len(req.Properties),
	)

	resp, err := p.client.Models.GenerateContent(ctx, p.model,
		genai.Text(userPrompt),
		&genai.GenerateContentConfig{
			ResponseMIMEType:   "application/json",
			ResponseJsonSchema: propertyValuesSchema,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("gemini generate property values: %w", err)
	}

	var result generate.PropertyValues
	if err := json.Unmarshal([]byte(resp.Text()), &result); err != nil {
		return nil, fmt.Errorf("gemini parse property values: %w", err)
	}

	p.logger.Debug("property values generated",
		"provider", "gemini",
		"model", p.model,
		"language", req.Language,
		"property_count", len(result.Values),
	)

	return &result, nil
}

// GeneratePrice generates a realistic retail price for a product using the
// Gemini API with structured JSON output.
func (p *Provider) GeneratePrice(ctx context.Context, req generate.PriceRequest) (*generate.PriceResult, error) {
	userPrompt := generate.BuildPricePrompt(req)

	// Use English system prompt (prices are language-independent).
	systemPrompt := generate.SystemPromptForLanguage("en")

	p.logger.Debug("generating price",
		"provider", "gemini",
		"model", p.model,
		"product_type", req.ProductType,
		"currency", req.Currency,
	)

	resp, err := p.client.Models.GenerateContent(ctx, p.model,
		genai.Text(userPrompt),
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: systemPrompt}},
			},
			ResponseMIMEType:   "application/json",
			ResponseJsonSchema: priceResultSchema,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("gemini generate price: %w", err)
	}

	text := resp.Text()
	if text == "" {
		p.logger.Error("gemini returned empty price response",
			"model", p.model,
			"product_type", req.ProductType,
			"candidates", len(resp.Candidates),
		)
		return nil, fmt.Errorf("gemini generate price: empty response from model")
	}

	var result generate.PriceResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		p.logger.Error("gemini price response parse failed",
			"model", p.model,
			"raw_response", text,
			"error", err,
		)
		return nil, fmt.Errorf("gemini parse price: %w", err)
	}

	p.logger.Debug("price generated",
		"provider", "gemini",
		"model", p.model,
		"product_type", req.ProductType,
		"price", result.Price,
		"currency", result.Currency,
	)

	return &result, nil
}

// GenerateBatch generates multiple products with all translations and pricing
// data in a single API call using the Gemini API with structured JSON output.
func (p *Provider) GenerateBatch(ctx context.Context, req generate.BatchRequest) (*generate.BatchProductResult, error) {
	systemPrompt := generate.BatchSystemPrompt()
	userPrompt := generate.BuildBatchPrompt(req)

	p.logger.Debug("generating product batch",
		"provider", "gemini",
		"model", p.model,
		"count", req.Count,
		"languages", req.Languages,
		"product_type", req.ProductType,
	)

	resp, err := p.client.Models.GenerateContent(ctx, p.model,
		genai.Text(userPrompt),
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{Text: systemPrompt}},
			},
			ResponseMIMEType:   "application/json",
			ResponseJsonSchema: batchProductResultSchema,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("gemini batch generate: %w", err)
	}

	text := resp.Text()
	if text == "" {
		p.logger.Error("gemini returned empty batch response",
			"model", p.model,
			"product_type", req.ProductType,
			"candidates", len(resp.Candidates),
		)
		return nil, fmt.Errorf("gemini batch generate: empty response from model")
	}

	var result generate.BatchProductResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		p.logger.Error("gemini batch response parse failed",
			"model", p.model,
			"error", err,
		)
		return nil, fmt.Errorf("gemini parse batch result: %w", err)
	}

	p.logger.Debug("batch generated",
		"provider", "gemini",
		"model", p.model,
		"products_returned", len(result.Products),
	)

	return &result, nil
}
