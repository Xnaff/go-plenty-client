package app

import (
	"fmt"
	"log/slog"

	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/janemig/plentyone/internal/generate"
	"github.com/janemig/plentyone/internal/generate/mock"
	oaiprovider "github.com/janemig/plentyone/internal/generate/openai"
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
