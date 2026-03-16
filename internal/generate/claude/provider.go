// Package claude implements the generate.Generator and generate.BatchGenerator
// interfaces using the Anthropic Claude API via the anthropic-sdk-go SDK.
// Structured output is achieved through tool use with forced tool selection,
// which guarantees JSON schema compliance.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/janemig/plentyone/internal/generate"
)

// Compile-time checks.
var _ generate.Generator = (*Provider)(nil)
var _ generate.BatchGenerator = (*Provider)(nil)

// Provider implements generate.Generator using the Anthropic Claude Messages API
// with tool-use-based structured output for guaranteed schema compliance.
type Provider struct {
	client anthropic.Client
	model  string
	logger *slog.Logger
}

// NewProvider creates a Claude Generator provider.
func NewProvider(client anthropic.Client, model string, logger *slog.Logger) *Provider {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &Provider{
		client: client,
		model:  model,
		logger: logger,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "claude" }

// GenerateProductTexts generates all text fields for a product in a single language
// using the Claude Messages API with tool-based structured output.
func (p *Provider) GenerateProductTexts(ctx context.Context, req generate.ProductTextRequest) (*generate.ProductTexts, error) {
	systemPrompt := generate.SystemPromptForLanguage(req.Language)
	userPrompt := generate.BuildProductTextPrompt(req)

	p.logger.Debug("generating product texts",
		"provider", "claude",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
	)

	raw, err := p.callWithTool(ctx, systemPrompt, userPrompt, "product_texts", "Generate structured product text fields", productTextsSchema)
	if err != nil {
		return nil, fmt.Errorf("claude generate product texts: %w", err)
	}

	var result generate.ProductTexts
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("claude parse product texts: %w", err)
	}

	p.logger.Debug("product texts generated",
		"provider", "claude",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
		"name_length", len(result.Name),
	)

	return &result, nil
}

// GeneratePropertyValues generates property values for a product using the
// Claude Messages API with tool-based structured output.
func (p *Provider) GeneratePropertyValues(ctx context.Context, req generate.PropertyValueRequest) (*generate.PropertyValues, error) {
	userPrompt := generate.BuildPropertyValuePrompt(req)

	p.logger.Debug("generating property values",
		"provider", "claude",
		"model", p.model,
		"language", req.Language,
		"product_type", req.ProductType,
		"property_count", len(req.Properties),
	)

	raw, err := p.callWithTool(ctx, "", userPrompt, "property_values", "Generate structured property values", propertyValuesSchema)
	if err != nil {
		return nil, fmt.Errorf("claude generate property values: %w", err)
	}

	var result generate.PropertyValues
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("claude parse property values: %w", err)
	}

	p.logger.Debug("property values generated",
		"provider", "claude",
		"model", p.model,
		"language", req.Language,
		"property_count", len(result.Values),
	)

	return &result, nil
}

// GeneratePrice generates a realistic retail price for a product using the
// Claude Messages API with tool-based structured output.
func (p *Provider) GeneratePrice(ctx context.Context, req generate.PriceRequest) (*generate.PriceResult, error) {
	userPrompt := generate.BuildPricePrompt(req)

	// Use English system prompt (prices are language-independent).
	systemPrompt := generate.SystemPromptForLanguage("en")

	p.logger.Debug("generating price",
		"provider", "claude",
		"model", p.model,
		"product_type", req.ProductType,
		"currency", req.Currency,
	)

	raw, err := p.callWithTool(ctx, systemPrompt, userPrompt, "price_result", "Generate structured pricing and physical product data", priceResultSchema)
	if err != nil {
		return nil, fmt.Errorf("claude generate price: %w", err)
	}

	var result generate.PriceResult
	if err := json.Unmarshal(raw, &result); err != nil {
		p.logger.Error("claude price response parse failed",
			"model", p.model,
			"raw_response", string(raw),
			"error", err,
		)
		return nil, fmt.Errorf("claude parse price: %w", err)
	}

	p.logger.Debug("price generated",
		"provider", "claude",
		"model", p.model,
		"product_type", req.ProductType,
		"price", result.Price,
		"currency", result.Currency,
	)

	return &result, nil
}

// GenerateBatch generates multiple products with all translations and pricing
// data in a single API call using the Claude Messages API with tool-based structured output.
func (p *Provider) GenerateBatch(ctx context.Context, req generate.BatchRequest) (*generate.BatchProductResult, error) {
	systemPrompt := generate.BatchSystemPrompt()
	userPrompt := generate.BuildBatchPrompt(req)

	p.logger.Debug("generating product batch",
		"provider", "claude",
		"model", p.model,
		"count", req.Count,
		"languages", req.Languages,
		"product_type", req.ProductType,
	)

	raw, err := p.callWithTool(ctx, systemPrompt, userPrompt, "batch_product_result", "Generate a batch of products with multilingual texts and pricing", batchProductResultSchema)
	if err != nil {
		return nil, fmt.Errorf("claude batch generate: %w", err)
	}

	var result generate.BatchProductResult
	if err := json.Unmarshal(raw, &result); err != nil {
		p.logger.Error("claude batch response parse failed",
			"model", p.model,
			"error", err,
		)
		return nil, fmt.Errorf("claude parse batch result: %w", err)
	}

	p.logger.Debug("batch generated",
		"provider", "claude",
		"model", p.model,
		"products_returned", len(result.Products),
	)

	return &result, nil
}

// callWithTool is the core helper that calls the Claude Messages API with a
// single forced tool, extracting the structured JSON from the tool_use response.
// Uses streaming to handle long-running generation (required by Anthropic API for
// operations that may exceed 10 minutes). The response is accumulated into a
// complete Message and the tool_use input is extracted.
func (p *Provider) callWithTool(
	ctx context.Context,
	systemPrompt string,
	userPrompt string,
	toolName string,
	toolDescription string,
	inputSchema anthropic.ToolInputSchemaParam,
) (json.RawMessage, error) {

	toolParam := anthropic.ToolParam{
		Name:        toolName,
		Description: anthropic.String(toolDescription),
		InputSchema: inputSchema,
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: 32768,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
		Tools: []anthropic.ToolUnionParam{
			{OfTool: &toolParam},
		},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{
				Name: toolName,
			},
		},
	}

	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}

	// Use streaming to avoid the 10-minute timeout for non-streaming requests.
	stream := p.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	// Accumulate streamed events into a complete message.
	message := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return nil, fmt.Errorf("accumulate stream event: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}

	// Check for truncated responses due to token limits.
	if message.StopReason == "max_tokens" {
		p.logger.Error("claude response truncated (max_tokens reached)",
			"model", p.model,
			"tool", toolName,
			"content_blocks", len(message.Content),
		)
		return nil, fmt.Errorf("response truncated: max_tokens reached (reduce batch size or languages)")
	}

	// Find the tool_use block in the response using direct field access
	// on ContentBlockUnion (avoids potential type assertion issues with AsAny).
	for _, block := range message.Content {
		if block.Type == "tool_use" && block.Name == toolName {
			if len(block.Input) == 0 {
				p.logger.Error("claude tool_use block has empty input",
					"model", p.model,
					"tool", toolName,
					"stop_reason", message.StopReason,
				)
				return nil, fmt.Errorf("tool_use block has empty input (stop_reason: %s)", message.StopReason)
			}
			p.logger.Debug("claude tool_use extracted",
				"tool", toolName,
				"input_bytes", len(block.Input),
			)
			return block.Input, nil
		}
	}

	// Log content block types for debugging.
	var blockTypes []string
	for _, block := range message.Content {
		blockTypes = append(blockTypes, block.Type)
	}
	p.logger.Error("no tool_use block found in claude response",
		"model", p.model,
		"tool", toolName,
		"stop_reason", message.StopReason,
		"block_types", blockTypes,
		"content_blocks", len(message.Content),
	)

	return nil, fmt.Errorf("no tool_use block found in response (stop_reason: %s, blocks: %v)", message.StopReason, blockTypes)
}
