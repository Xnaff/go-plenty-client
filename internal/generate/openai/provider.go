package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/janemig/plentyone/internal/generate"
)

// Provider implements generate.Generator using the OpenAI Responses API
// with structured output (JSON schema) for guaranteed schema compliance.
type Provider struct {
	client oai.Client
	model  string
	logger *slog.Logger
}

// NewProvider creates an OpenAI Generator provider.
func NewProvider(client oai.Client, model string, logger *slog.Logger) *Provider {
	return &Provider{
		client: client,
		model:  model,
		logger: logger,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "openai" }

// GenerateProductTexts generates all text fields for a product in a single language
// using the OpenAI Responses API with structured output.
func (p *Provider) GenerateProductTexts(ctx context.Context, req generate.ProductTextRequest) (*generate.ProductTexts, error) {
	systemPrompt := generate.SystemPromptForLanguage(req.Language)
	userPrompt := generate.BuildProductTextPrompt(req)

	p.logger.Debug("generating product texts",
		"provider", "openai",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
	)

	resp, err := p.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:        p.model,
		Instructions: oai.String(systemPrompt),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: oai.String(userPrompt),
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigParamOfJSONSchema(
				"product_texts",
				productTextsSchema,
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai generate product texts: %w", err)
	}

	var result generate.ProductTexts
	if err := json.Unmarshal([]byte(resp.OutputText()), &result); err != nil {
		return nil, fmt.Errorf("openai parse product texts: %w", err)
	}

	p.logger.Debug("product texts generated",
		"provider", "openai",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
		"name_length", len(result.Name),
	)

	return &result, nil
}

// GeneratePropertyValues generates property values for a product using the
// OpenAI Responses API with structured output.
func (p *Provider) GeneratePropertyValues(ctx context.Context, req generate.PropertyValueRequest) (*generate.PropertyValues, error) {
	userPrompt := generate.BuildPropertyValuePrompt(req)

	p.logger.Debug("generating property values",
		"provider", "openai",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
		"property_count", len(req.Properties),
	)

	resp, err := p.client.Responses.New(ctx, responses.ResponseNewParams{
		Model: p.model,
		Input: responses.ResponseNewParamsInputUnion{
			OfString: oai.String(userPrompt),
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigParamOfJSONSchema(
				"property_values",
				propertyValuesSchema,
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("openai generate property values: %w", err)
	}

	var result generate.PropertyValues
	if err := json.Unmarshal([]byte(resp.OutputText()), &result); err != nil {
		return nil, fmt.Errorf("openai parse property values: %w", err)
	}

	p.logger.Debug("property values generated",
		"provider", "openai",
		"model", p.model,
		"language", req.Language,
		"property_count", len(result.Values),
	)

	return &result, nil
}
