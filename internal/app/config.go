package app

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Log      LogConfig      `mapstructure:"log"`
	Pipeline PipelineConfig `mapstructure:"pipeline"`
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
