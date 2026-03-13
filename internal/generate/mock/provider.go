package mock

import (
	"context"
	"fmt"

	"github.com/janemig/plentyone/internal/generate"
)

// Compile-time checks: Provider implements both generate.Generator and generate.ImageGenerator.
var _ generate.Generator = (*Provider)(nil)
var _ generate.ImageGenerator = (*Provider)(nil)

// mockPNG1x1 is a 1x1 pixel transparent PNG encoded as base64.
// Used for deterministic test output without network calls.
const mockPNG1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// Provider is a mock Generator that returns deterministic, canned product data.
// It makes zero network calls and is intended for development and testing.
type Provider struct{}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "mock" }

// GenerateProductTexts returns deterministic product texts based on the request.
func (p *Provider) GenerateProductTexts(_ context.Context, req generate.ProductTextRequest) (*generate.ProductTexts, error) {
	return &generate.ProductTexts{
		Name:             fmt.Sprintf("Test Product %s (%s)", req.ProductType, req.Language),
		ShortDescription: fmt.Sprintf("A high-quality %s product for testing purposes.", req.ProductType),
		Description:      fmt.Sprintf("<p>This is a detailed description of a %s product. It includes all the features and benefits that customers want to know about.</p><ul><li>Premium quality materials</li><li>Designed for everyday use</li></ul>", req.ProductType),
		TechnicalData:    "<table><tr><th>Material</th><td>Premium</td></tr><tr><th>Weight</th><td>500g</td></tr><tr><th>Dimensions</th><td>20x15x10cm</td></tr></table>",
		MetaDescription:  fmt.Sprintf("Buy the best %s online. Free shipping. Great quality.", req.ProductType),
		URLContent:       fmt.Sprintf("test-product-%s-%s", req.ProductType, req.Language),
		PreviewText:      fmt.Sprintf("Discover our amazing %s product.", req.ProductType),
	}, nil
}

// GeneratePropertyValues returns type-appropriate deterministic values for each property.
func (p *Provider) GeneratePropertyValues(_ context.Context, req generate.PropertyValueRequest) (*generate.PropertyValues, error) {
	values := make([]generate.PropertyValue, len(req.Properties))
	for i, prop := range req.Properties {
		values[i] = generate.PropertyValue{PropertyID: prop.ID}
		switch prop.PropertyType {
		case "text":
			values[i].TextValue = fmt.Sprintf("Mock value for %s", prop.Name)
		case "int":
			v := int64(42)
			values[i].IntValue = &v
		case "float":
			v := 9.99
			values[i].FloatValue = &v
		case "selection":
			if len(prop.Options) > 0 {
				values[i].SelectionValue = prop.Options[0]
			}
		}
	}
	return &generate.PropertyValues{Values: values}, nil
}

// GenerateProductImage returns a deterministic 1x1 transparent PNG for testing.
func (p *Provider) GenerateProductImage(_ context.Context, req generate.ImageRequest) (*generate.ImageResult, error) {
	return &generate.ImageResult{
		Base64Data:    mockPNG1x1,
		RevisedPrompt: req.BuildPrompt(),
		Format:        "png",
	}, nil
}
