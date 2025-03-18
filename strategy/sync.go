package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// SyncStrategy processes logs synchronously without batching or concurrency.
type SyncStrategy struct {
	errors chan<- error // Channel to propagate errors (borrowed from MultiHandler)
}

// NewSyncStrategy creates a new SyncStrategy.
func NewSyncStrategy(errors chan<- error) *SyncStrategy {
	return &SyncStrategy{errors: errors}
}

// Process handles logs synchronously.
func (ss *SyncStrategy) Process(ctx context.Context, r slog.Record, handlers []slog.Handler, errors chan<- error) {
	for _, h := range handlers {
		if err := h.Handle(ctx, r); err != nil {
			select {
			case ss.errors <- err:
			default:
				fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", err)
			}
		}
	}
}

// Flush is a no-op for SyncStrategy.
func (ss *SyncStrategy) Flush() {}

// Close is a no-op for SyncStrategy.
func (ss *SyncStrategy) Close() {}
