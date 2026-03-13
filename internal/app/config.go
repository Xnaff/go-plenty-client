package app

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Log         LogConfig         `mapstructure:"log"`
	Pipeline    PipelineConfig    `mapstructure:"pipeline"`
	AI          AIConfig          `mapstructure:"ai"`
	API         APIConfig         `mapstructure:"api"`
	Images      ImageConfig       `mapstructure:"images"`
	StockPhotos StockPhotoConfig  `mapstructure:"stock_photos"`
	Enrichment  EnrichmentConfig  `mapstructure:"enrichment"`
	Quality     QualityConfig     `mapstructure:"quality"`
}

// ImageConfig holds AI image generation settings.
type ImageConfig struct {
	Provider   string `mapstructure:"provider"`    // AI image provider: openai, mock
	Model      string `mapstructure:"model"`       // gpt-image-1, gpt-image-1-mini
	Quality    string `mapstructure:"quality"`      // low, medium, high
	Size       string `mapstructure:"size"`         // 1024x1024, 1792x1024, 1024x1792
	Format     string `mapstructure:"format"`       // png, webp, jpeg
	PerProduct int    `mapstructure:"per_product"`  // AI images per product
}

// StockPhotoConfig holds stock photo sourcing settings.
type StockPhotoConfig struct {
	Enabled     bool     `mapstructure:"enabled"`
	Providers   []string `mapstructure:"providers"`    // Order = priority: [unsplash, pexels, pixabay]
	PerProduct  int      `mapstructure:"per_product"`
	MinWidth    int      `mapstructure:"min_width"`
	MinHeight   int      `mapstructure:"min_height"`
	Orientation string   `mapstructure:"orientation"`  // landscape, portrait, square
	UnsplashKey string   `mapstructure:"unsplash_key"`
	PexelsKey   string   `mapstructure:"pexels_key"`
	PixabayKey  string   `mapstructure:"pixabay_key"`
}

// EnrichmentConfig holds public database enrichment settings.
type EnrichmentConfig struct {
	Enabled  bool     `mapstructure:"enabled"`
	Sources  []string `mapstructure:"sources"`     // [openfoodfacts, wikidata]
	CacheTTL string   `mapstructure:"cache_ttl"`   // e.g., "24h"
}

// QualityConfig holds quality scoring settings.
type QualityConfig struct {
	Enabled         bool    `mapstructure:"enabled"`
	MinOverallScore float64 `mapstructure:"min_overall_score"`
	MinTextScore    float64 `mapstructure:"min_text_score"`
	MinImageScore   float64 `mapstructure:"min_image_score"`
	MinDataScore    float64 `mapstructure:"min_data_score"`
	FlagAction      string  `mapstructure:"flag_action"`   // warn, skip, fail
}

// APIConfig holds PlentyONE REST API connection settings.
type APIConfig struct {
	BaseURL   string `mapstructure:"base_url"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	RateLimit int    `mapstructure:"rate_limit"`
	Timeout   int    `mapstructure:"timeout"` // seconds
}

// AIConfig holds AI generation provider settings.
type AIConfig struct {
	Provider  string   `mapstructure:"provider"`  // AI provider: mock, openai
	APIKey    string   `mapstructure:"api_key"`   // API key (use env var PLENTYONE_AI_API_KEY)
	Model     string   `mapstructure:"model"`     // Model name (provider-specific)
	Languages []string `mapstructure:"languages"` // Languages to generate text in
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig holds MySQL connection settings.
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	MaxConns int    `mapstructure:"max_conns"`
}

// DatabaseDSN returns a MySQL DSN string for database/sql.
func (d DatabaseConfig) DatabaseDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

// LogConfig holds structured logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // json, text
}

// PipelineConfig holds pipeline execution settings.
type PipelineConfig struct {
	Concurrency     int     `mapstructure:"concurrency"`
	BatchSize       int     `mapstructure:"batch_size"`
	RateLimitPerSec float64 `mapstructure:"rate_limit_per_sec"`
}

// LoadConfig reads configuration from file, env vars, and defaults.
// If cfgFile is provided, it is used directly. Otherwise, Viper searches
// standard paths for a file named "config.yaml".
func LoadConfig(cfgFile string) (*Config, error) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath("$HOME/.plentyone")
	}

	// Defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 3306)
	viper.SetDefault("database.name", "plentyone")
	viper.SetDefault("database.max_conns", 25)
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("pipeline.concurrency", 5)
	viper.SetDefault("pipeline.batch_size", 50)
	viper.SetDefault("pipeline.rate_limit_per_sec", 2.0)
	viper.SetDefault("ai.provider", "mock")
	viper.SetDefault("ai.model", "gpt-4o-mini")
	viper.SetDefault("ai.languages", []string{"en", "de", "es", "fr", "it"})
	viper.SetDefault("api.base_url", "")
	viper.SetDefault("api.username", "")
	viper.SetDefault("api.password", "")
	viper.SetDefault("api.rate_limit", 40)
	viper.SetDefault("api.timeout", 60)
	viper.SetDefault("images.provider", "mock")
	viper.SetDefault("images.model", "gpt-image-1")
	viper.SetDefault("images.quality", "medium")
	viper.SetDefault("images.size", "1024x1024")
	viper.SetDefault("images.format", "png")
	viper.SetDefault("images.per_product", 1)
	viper.SetDefault("stock_photos.enabled", false)
	viper.SetDefault("stock_photos.providers", []string{"unsplash", "pexels", "pixabay"})
	viper.SetDefault("stock_photos.per_product", 2)
	viper.SetDefault("stock_photos.min_width", 800)
	viper.SetDefault("stock_photos.min_height", 600)
	viper.SetDefault("stock_photos.orientation", "landscape")
	viper.SetDefault("enrichment.enabled", false)
	viper.SetDefault("enrichment.sources", []string{"openfoodfacts", "wikidata"})
	viper.SetDefault("enrichment.cache_ttl", "24h")
	viper.SetDefault("quality.enabled", true)
	viper.SetDefault("quality.min_overall_score", 0.6)
	viper.SetDefault("quality.min_text_score", 0.5)
	viper.SetDefault("quality.min_image_score", 0.4)
	viper.SetDefault("quality.min_data_score", 0.3)
	viper.SetDefault("quality.flag_action", "warn")

	// Environment variables: PLENTYONE_DATABASE_HOST, etc.
	viper.SetEnvPrefix("PLENTYONE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		// Config file not found is OK -- use defaults + env vars
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}
