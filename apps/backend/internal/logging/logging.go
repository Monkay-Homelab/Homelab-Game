package logging

import (
	"log/slog"
	"os"
)

// Init configures the default slog logger based on the ENV environment variable.
//
//   - ENV=production: JSON handler at Info level (machine-parseable for log aggregation).
//   - All other values (or unset): Text handler at Debug level (human-readable for development).
//
// Call this once, early in main(), before any logging occurs.
func Init() {
	env := os.Getenv("ENV")

	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	slog.SetDefault(slog.New(handler))
}
