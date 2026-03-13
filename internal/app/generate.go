package app

import (
	"fmt"
	"log/slog"

	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/janemig/plentyone/internal/enrichment"
	"github.com/janemig/plentyone/internal/enrichment/openfoodfacts"
	"github.com/janemig/plentyone/internal/enrichment/wikidata"
	"github.com/janemig/plentyone/internal/generate"
	"github.com/janemig/plentyone/internal/generate/mock"
	oaiprovider "github.com/janemig/plentyone/internal/generate/openai"
	"github.com/janemig/plentyone/internal/generate/quality"
	"github.com/janemig/plentyone/internal/imagesource"
	"github.com/janemig/plentyone/internal/imagesource/pexels"
	"github.com/janemig/plentyone/internal/imagesource/pixabay"
	"github.com/janemig/plentyone/internal/imagesource/unsplash"
)

// NewGeneratorFromConfig creates the correct Generator implementation based on
// the AI provider configured in the application config.
func NewGeneratorFromConfig(cfg *Config, logger *slog.Logger) (generate.Generator, error) {
	switch cfg.AI.Provider {
	case "openai":
		client := oai.NewClient(option.WithAPIKey(cfg.AI.APIKey))
		return oaiprovider.NewProvider(client, cfg.AI.Model, logger), nil
	case "mock":
		return &mock.Provider{}, nil
	default:
		return nil, fmt.Errorf("unknown AI provider: %s", cfg.AI.Provider)
	}
}

// NewImageGeneratorFromConfig creates the correct ImageGenerator based on config.
func NewImageGeneratorFromConfig(cfg *Config, logger *slog.Logger) (generate.ImageGenerator, error) {
	switch cfg.Images.Provider {
	case "openai":
		client := oai.NewClient(option.WithAPIKey(cfg.AI.APIKey))
		return oaiprovider.NewImageProvider(client, cfg.Images.Model, logger), nil
	case "mock":
		return &mock.Provider{}, nil
	default:
		return nil, fmt.Errorf("unknown image provider: %s", cfg.Images.Provider)
	}
}

// NewImageSourcesFromConfig creates stock photo clients based on config.
// Returns nil if stock photos are disabled or no API keys are configured.
func NewImageSourcesFromConfig(cfg *Config) []imagesource.ImageSource {
	if !cfg.StockPhotos.Enabled {
		return nil
	}
	var sources []imagesource.ImageSource
	for _, name := range cfg.StockPhotos.Providers {
		switch name {
		case "unsplash":
			if cfg.StockPhotos.UnsplashKey != "" {
				sources = append(sources, unsplash.NewClient(cfg.StockPhotos.UnsplashKey))
			}
		case "pexels":
			if cfg.StockPhotos.PexelsKey != "" {
				sources = append(sources, pexels.NewClient(cfg.StockPhotos.PexelsKey))
			}
		case "pixabay":
			if cfg.StockPhotos.PixabayKey != "" {
				sources = append(sources, pixabay.NewClient(cfg.StockPhotos.PixabayKey))
			}
		}
	}
	return sources
}

// NewEnrichersFromConfig creates enrichment clients based on config.
// Returns nil if enrichment is disabled.
func NewEnrichersFromConfig(cfg *Config) []enrichment.Enricher {
	if !cfg.Enrichment.Enabled {
		return nil
	}
	userAgent := "PlentyOne/1.0"
	var enrichers []enrichment.Enricher
	for _, name := range cfg.Enrichment.Sources {
		switch name {
		case "openfoodfacts":
			enrichers = append(enrichers, openfoodfacts.NewClient(userAgent))
		case "wikidata":
			enrichers = append(enrichers, wikidata.NewClient(userAgent))
		}
	}
	return enrichers
}

// NewQualityScorerFromConfig creates a quality scorer from config.
// Returns nil if quality scoring is disabled.
func NewQualityScorerFromConfig(cfg *Config) *quality.Scorer {
	if !cfg.Quality.Enabled {
		return nil
	}
	return quality.NewScorer(quality.Thresholds{
		MinOverallScore: cfg.Quality.MinOverallScore,
		MinTextScore:    cfg.Quality.MinTextScore,
		MinImageScore:   cfg.Quality.MinImageScore,
		MinDataScore:    cfg.Quality.MinDataScore,
	})
}
