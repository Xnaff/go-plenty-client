package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"google.golang.org/genai"

	"github.com/janemig/plentyone/internal/generate"
)

// Compile-time check: ImageProvider implements generate.ImageGenerator.
var _ generate.ImageGenerator = (*ImageProvider)(nil)

// ImageProvider implements generate.ImageGenerator using the Gemini Imagen API.
type ImageProvider struct {
	client *genai.Client
	model  string
	logger *slog.Logger
}

// NewImageProvider creates a Gemini ImageGenerator provider.
// Default model is imagen-3.0-generate-002.
func NewImageProvider(client *genai.Client, model string, logger *slog.Logger) *ImageProvider {
	if model == "" {
		model = "imagen-3.0-generate-002"
	}
	return &ImageProvider{
		client: client,
		model:  model,
		logger: logger,
	}
}

// Name returns the provider identifier.
func (p *ImageProvider) Name() string { return "gemini" }

// GenerateProductImage generates a product image using the Gemini Imagen API.
func (p *ImageProvider) GenerateProductImage(ctx context.Context, req generate.ImageRequest) (*generate.ImageResult, error) {
	prompt := req.BuildPrompt()
	aspectRatio := mapSizeToAspectRatio(req.Size)

	p.logger.Debug("generating product image",
		"provider", "gemini",
		"model", p.model,
		"aspect_ratio", aspectRatio,
		"product_type", req.ProductType,
	)

	resp, err := p.client.Models.GenerateImages(ctx, p.model, prompt,
		&genai.GenerateImagesConfig{
			NumberOfImages: 1,
			AspectRatio:    aspectRatio,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("gemini generate product image: %w", err)
	}

	if len(resp.GeneratedImages) == 0 {
		return nil, fmt.Errorf("gemini generate product image: empty response")
	}

	img := resp.GeneratedImages[0]
	imgBytes := img.Image.ImageBytes
	b64 := base64.StdEncoding.EncodeToString(imgBytes)

	revisedPrompt := prompt
	if img.EnhancedPrompt != "" {
		revisedPrompt = img.EnhancedPrompt
	}

	p.logger.Debug("product image generated",
		"provider", "gemini",
		"model", p.model,
		"image_size_bytes", len(imgBytes),
	)

	return &generate.ImageResult{
		Base64Data:    b64,
		RevisedPrompt: revisedPrompt,
		Format:        "png",
	}, nil
}

// mapSizeToAspectRatio converts pixel dimension strings to Gemini aspect ratios.
func mapSizeToAspectRatio(size string) string {
	switch size {
	case "1024x1024":
		return "1:1"
	case "1536x1024", "1792x1024":
		return "16:9"
	case "1024x1536", "1024x1792":
		return "9:16"
	default:
		return "1:1"
	}
}
