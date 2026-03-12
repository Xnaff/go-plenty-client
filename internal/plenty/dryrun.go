package plenty

import (
	"encoding/json"
	"log/slog"
)

// dryRunLog marshals the payload to JSON and logs the request at info level.
// Used by entity services when the client is in dry-run mode.
func dryRunLog(logger *slog.Logger, method, path string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		logger.Info("dry-run: would send request",
			slog.String("method", method),
			slog.String("path", path),
			slog.String("payload_error", err.Error()),
		)
		return
	}

	logger.Info("dry-run: would send request",
		slog.String("method", method),
		slog.String("path", path),
		slog.String("payload", string(data)),
	)
}
