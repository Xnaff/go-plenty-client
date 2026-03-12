package app

import (
	"log/slog"
	"os"
)

// SetupLogger creates the application logger based on config.
// It sets the logger as the default via slog.SetDefault.
func SetupLogger(cfg LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug,
	}

	var handler slog.Handler
	switch cfg.Format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// LoggerWithRunContext returns a logger with pipeline run correlation fields.
func LoggerWithRunContext(logger *slog.Logger, runID, jobID int64) *slog.Logger {
	return logger.With(
		slog.Int64("run_id", runID),
		slog.Int64("job_id", jobID),
	)
}

// LoggerWithEntity returns a logger with entity-level context for detailed tracing.
func LoggerWithEntity(logger *slog.Logger, entityType string, localID int64) *slog.Logger {
	return logger.With(
		slog.String("entity_type", entityType),
		slog.Int64("local_id", localID),
	)
}
