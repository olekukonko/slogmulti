package strategy

import (
	"context"
	"log/slog"
)

// MultiHandlerStrategy defines the interface for log processing strategies.
type MultiHandlerStrategy interface {
	Process(ctx context.Context, r slog.Record, handlers []slog.Handler, errors chan<- error) // Process a log record
	Flush()                                                                                   // Flush pending logs (if applicable)
	Close()                                                                                   // Clean up resources
}
