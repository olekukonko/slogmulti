package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// Sync processes logs synchronously without batching or concurrency.
type Sync struct {
	errors chan<- error // Channel to propagate errors (borrowed from MultiHandler)
}

// NewSyncStrategy creates a new Sync.
func NewSyncStrategy(errors chan<- error) *Sync {
	return &Sync{errors: errors}
}

// Process handles logs synchronously.
func (ss *Sync) Process(ctx context.Context, r slog.Record, handlers []slog.Handler, errors chan<- error) {
	for _, h := range handlers {
		if err := h.Handle(ctx, r); err != nil {
			select {
			case errors <- err: // Use the passed errors channel
			default:
				fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", err)
			}
		}
	}
}

// Flush is a no-op for Sync.
func (ss *Sync) Flush() {}

// Close is a no-op for Sync.
func (ss *Sync) Close() {}
