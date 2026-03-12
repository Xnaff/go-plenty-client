package generate

import "context"

// Generator is the interface that AI providers must implement.
// Each method generates a specific kind of product data.
// The interface is domain-aware (not generic "send prompt, get text"),
// so each provider handles its own prompt construction and response parsing.
type Generator interface {
	// GenerateProductTexts generates all text fields for a product in a single language.
	// Returns texts for: name, shortDescription, description, technicalData,
	// metaDescription, urlContent, and previewText.
	GenerateProductTexts(ctx context.Context, req ProductTextRequest) (*ProductTexts, error)

	// GeneratePropertyValues generates property values for a product.
	// Respects property types: text properties get multilingual values,
	// numeric properties get appropriate numbers, selection properties
	// get values from allowed options.
	GeneratePropertyValues(ctx context.Context, req PropertyValueRequest) (*PropertyValues, error)

	// Name returns the provider name for logging and configuration purposes.
	Name() string
}
