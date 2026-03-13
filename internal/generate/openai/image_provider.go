package openai

import (
	"context"
	"fmt"
	"log/slog"

	oai "github.com/openai/openai-go/v3"

	"github.com/janemig/plentyone/internal/generate"
)

// Compile-time check: ImageProvider implements generate.ImageGenerator.
var _ generate.ImageGenerator = (*ImageProvider)(nil)

// ImageProvider implements generate.ImageGenerator using the OpenAI Images API
// with gpt-image-1 for AI-generated product photos.
type ImageProvider struct {
	client oai.Client
	model  string
	logger *slog.Logger
}

// NewImageProvider creates an OpenAI ImageGenerator provider.
// If model is empty, defaults to gpt-image-1.
func NewImageProvider(client oai.Client, model string, logger *slog.Logger) *ImageProvider {
	if model == "" {
		model = oai.ImageModelGPTImage1
	}
	return &ImageProvider{
		client: client,
		model:  model,
		logger: logger,
	}
}

// Name returns the provider identifier.
func (p *ImageProvider) Name() string { return "openai" }

// GenerateProductImage generates a product photo using the OpenAI Images API.
// It constructs a prompt from the request, calls gpt-image-1, and returns
// the base64-encoded image data.
func (p *ImageProvider) GenerateProductImage(ctx context.Context, req generate.ImageRequest) (*generate.ImageResult, error) {
	prompt := req.BuildPrompt()

	size := mapSize(req.Size)
	quality := mapQuality(req.Quality)

	p.logger.Debug("generating product image",
		"provider", "openai",
		"model", p.model,
		"product_name", req.ProductName,
		"product_type", req.ProductType,
		"size", size,
		"quality", quality,
	)

	resp, err := p.client.Images.Generate(ctx, oai.ImageGenerateParams{
		Prompt:       prompt,
		Model:        oai.ImageModel(p.model),
		N:            oai.Opt(int64(1)),
		Size:         size,
		Quality:      quality,
		OutputFormat: oai.ImageGenerateParamsOutputFormatPNG,
	})
	if err != nil {
		return nil, fmt.Errorf("openai generate product image: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai generate product image: empty response data")
	}

	p.logger.Debug("product image generated",
		"provider", "openai",
		"model", p.model,
		"product_name", req.ProductName,
		"b64_length", len(resp.Data[0].B64JSON),
	)

	return &generate.ImageResult{
		Base64Data:    resp.Data[0].B64JSON,
		RevisedPrompt: resp.Data[0].RevisedPrompt,
		Format:        "png",
	}, nil
}

// mapSize converts a string size to the SDK constant.
func mapSize(s string) oai.ImageGenerateParamsSize {
	switch s {
	case "1024x1024":
		return oai.ImageGenerateParamsSize1024x1024
	case "1536x1024":
		return oai.ImageGenerateParamsSize1536x1024
	case "1024x1536":
		return oai.ImageGenerateParamsSize1024x1536
	default:
		return oai.ImageGenerateParamsSize1024x1024
	}
}

// mapQuality converts a string quality to the SDK constant.
func mapQuality(q string) oai.ImageGenerateParamsQuality {
	switch q {
	case "low":
		return oai.ImageGenerateParamsQualityLow
	case "medium":
		return oai.ImageGenerateParamsQualityMedium
	case "high":
		return oai.ImageGenerateParamsQualityHigh
	case "auto":
		return oai.ImageGenerateParamsQualityAuto
	default:
		return oai.ImageGenerateParamsQualityMedium
	}
}
