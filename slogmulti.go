package slogmulti

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"log/slog"
)

const (
	DefaultFlushInterval = 1 * time.Second
	DefaultBatchSize     = 10
	DefaultQueueSize     = 1000
	DefaultErrorSize     = 100
)

// Ingress represents a log entry with context, record, and an optional error.
type Ingress struct {
	ctx context.Context
	r   slog.Record
	err error
}

// MultiHandler combines multiple slog handlers, batching logs for efficiency.
// It queues log records and processes them in batches, either when the batch size
// is reached or after a specified flush interval. Errors from handlers are collected
// and can be retrieved via the Errors channel.
type MultiHandler struct {
	handlers      []slog.Handler // The underlying handlers to dispatch logs to
	queue         chan Ingress   // Buffered channel for log entries
	batchSize     int            // Maximum number of logs in a batch
	flushInterval time.Duration  // Interval at which batches are flushed
	wg            sync.WaitGroup // Wait group to ensure clean shutdown
	errors        chan error     // Channel to propagate errors from handlers
}

// MultiHandlerOption configures a MultiHandler during creation.
type MultiHandlerOption func(*MultiHandler)

// WithBatchSize sets the maximum number of log entries per batch.
// If size <= 0, the default (10) is used.
func WithBatchSize(size int) MultiHandlerOption {
	return func(mh *MultiHandler) {
		if size > 0 {
			mh.batchSize = size
		}
	}
}

// WithFlushInterval sets the interval at which batches are flushed, even if not full.
// If interval <= 0, the default (1 second) is used.
func WithFlushInterval(interval time.Duration) MultiHandlerOption {
	return func(mh *MultiHandler) {
		if interval > 0 {
			mh.flushInterval = interval
		}
	}
}

// WithHandlers adds the specified handlers to the MultiHandler.
// This option can be used multiple times, and handlers are appended.
func WithHandlers(handlers ...slog.Handler) MultiHandlerOption {
	return func(mh *MultiHandler) {
		mh.handlers = append(mh.handlers, handlers...)
	}
}

// WithErrorChannel sets a custom channel for error propagation.
// If not provided, a default channel with capacity 100 is created.
func WithErrorChannel(errChan chan error) MultiHandlerOption {
	return func(mh *MultiHandler) {
		if errChan != nil {
			mh.errors = errChan
		}
	}
}

// NewMultiHandler creates a new MultiHandler with the given options.
// Handlers must be provided via WithHandlers or added later via Add.
// Errors from handlers are sent to the Errors channel.
// Example:
//
//	mh := NewMultiHandler(WithBatchSize(20), WithHandlers(h1, h2))
func NewMultiHandler(opts ...MultiHandlerOption) *MultiHandler {
	mh := &MultiHandler{
		handlers:      make([]slog.Handler, 0),
		queue:         make(chan Ingress, DefaultQueueSize),
		errors:        make(chan error, DefaultErrorSize), // Default error channel
		batchSize:     DefaultBatchSize,
		flushInterval: DefaultFlushInterval,
	}
	for _, opt := range opts {
		opt(mh)
	}
	mh.startWorkers()
	return mh
}

// NewDefaultMultiHandler creates a new MultiHandler with default settings and the given handlers.
// It uses a batch size of 10, flush interval of 1 second, and a default error channel.
// Example:
//
//	mh := NewDefaultMultiHandler(h1, h2)
func NewDefaultMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return NewMultiHandler(WithHandlers(handlers...))
}

// Add appends additional handlers to the MultiHandler after creation.
// This can be called at any time to dynamically extend the handler list.
// Example:
//
//	mh.Add(h3, h4)
func (mh *MultiHandler) Add(handlers ...slog.Handler) {
	mh.handlers = append(mh.handlers, handlers...)
}

// Errors returns the channel where handler errors are sent.
// Users can listen on this channel to handle errors asynchronously.
// Example:
//
//	for err := range mh.Errors() {
//	    fmt.Println("Log error:", err)
//	}
func (mh *MultiHandler) Errors() <-chan error {
	return mh.errors
}

// startWorkers launches a goroutine to process queued log entries.
// It batches logs and flushes them based on batch size or flush interval.
func (mh *MultiHandler) startWorkers() {
	mh.wg.Add(1)
	go func() {
		defer mh.wg.Done()
		batch := make([]Ingress, 0, mh.batchSize)
		ticker := time.NewTicker(mh.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case entry, ok := <-mh.queue:
				if !ok {
					mh.flushBatch(batch)
					close(mh.errors) // Close error channel when done
					return
				}
				batch = append(batch, entry)
				if len(batch) >= mh.batchSize {
					mh.flushBatch(batch)
					batch = batch[:0]
				}
			case <-ticker.C:
				if len(batch) > 0 {
					mh.flushBatch(batch)
					batch = batch[:0]
				}
			}
		}
	}()
}

// flushBatch dispatches a batch of log entries to all handlers.
// Errors from handlers are sent to the errors channel instead of just being logged.
func (mh *MultiHandler) flushBatch(batch []Ingress) {
	for _, h := range mh.handlers {
		for _, entry := range batch {
			// If the entry already has an error, propagate it
			if entry.err != nil {
				select {
				case mh.errors <- entry.err:
				default: // Drop if channel is full
					fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", entry.err)
				}
				continue
			}
			// Process the log entry and capture any handler error
			if err := h.Handle(entry.ctx, entry.r); err != nil {
				select {
				case mh.errors <- err:
				default: // Drop if channel is full
					fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", err)
				}
			}
		}
	}
}

// Handle queues a log record for asynchronous processing.
// It returns nil immediately, with errors propagated via the Errors channel.
func (mh *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	mh.queue <- Ingress{ctx: ctx, r: r, err: nil}
	return nil
}

// Enabled reports whether any handler is enabled for the given level.
// For simplicity, it always returns true, delegating filtering to underlying handlers.
func (mh *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

// WithAttrs returns a new MultiHandler with additional attributes applied to all handlers.
func (mh *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(mh.handlers))
	for i, h := range mh.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return NewMultiHandler(
		WithHandlers(newHandlers...),
		WithBatchSize(mh.batchSize),
		WithFlushInterval(mh.flushInterval),
		WithErrorChannel(mh.errors), // Preserve the same error channel
	)
}

// WithGroup returns a new MultiHandler with a group name applied to all handlers.
func (mh *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(mh.handlers))
	for i, h := range mh.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return NewMultiHandler(
		WithHandlers(newHandlers...),
		WithBatchSize(mh.batchSize),
		WithFlushInterval(mh.flushInterval),
		WithErrorChannel(mh.errors), // Preserve the same error channel
	)
}

// Close flushes any remaining logs and stops the worker.
// It closes the error channel after all logs are processed.
// Call this when the handler is no longer needed to ensure proper cleanup.
func (mh *MultiHandler) Close() error {
	close(mh.queue)
	mh.wg.Wait()
	return nil
}
