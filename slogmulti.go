package slogmulti

import (
	"context"
	"github.com/olekukonko/slogmulti/strategy"
	"log/slog"
	"sync"
)

const (
	DefaultErrorSize = 100 // Default capacity of the error channel
)

// MultiHandler combines multiple slog handlers, batching logs for efficiency.
// It queues log records and processes them in batches, either when the batch size
// is reached or after a specified flush interval. Errors from handlers are collected
// and can be retrieved via the Errors channel.
type MultiHandler struct {
	handlers []slog.Handler                // The underlying handlers to dispatch logs to
	strategy strategy.MultiHandlerStrategy // Strategy for processing logs
	errors   chan error                    // Channel to propagate errors
	mu       sync.RWMutex                  // Protects handlers for concurrent access
}

// MultiHandlerOption configures a MultiHandler during creation.
type MultiHandlerOption func(*MultiHandler)

// WithStrategy sets the log processing strategy.
func WithStrategy(strategy strategy.MultiHandlerStrategy) MultiHandlerOption {
	return func(mh *MultiHandler) {
		mh.strategy = strategy
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
//
// NewMultiHandler creates a new MultiHandler with the given options.
func NewMultiHandler(opts ...MultiHandlerOption) *MultiHandler {
	mh := &MultiHandler{
		handlers: make([]slog.Handler, 0),
		errors:   make(chan error, DefaultErrorSize),
	}
	for _, opt := range opts {
		opt(mh)
	}
	if mh.strategy == nil {
		// Default to AsyncBatchStrategy with the initial handlers
		mh.strategy = strategy.NewAsyncBatchStrategy(mh.errors, mh.handlers)
	}
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
	mh.mu.Lock()
	defer mh.mu.Unlock()
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

// Handle queues a log record for asynchronous processing.
// It returns nil immediately, with errors propagated via the Errors channel.
func (mh *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	mh.mu.RLock()
	handlers := mh.handlers
	mh.mu.RUnlock()
	mh.strategy.Process(ctx, r, handlers, mh.errors)
	return nil
}

// Enabled reports whether any handler is enabled for the given level.
// For simplicity, it always returns true, delegating filtering to underlying handlers.
func (mh *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	mh.mu.RLock()
	defer mh.mu.RUnlock()
	for _, h := range mh.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// WithAttrs returns a new MultiHandler with additional attributes applied to all handlers.
func (mh *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	mh.mu.RLock()
	newHandlers := make([]slog.Handler, len(mh.handlers))
	for i, h := range mh.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	mh.mu.RUnlock()
	return NewMultiHandler(
		WithHandlers(newHandlers...),
		WithStrategy(mh.strategy),
		WithErrorChannel(mh.errors),
	)
}

// WithGroup returns a new MultiHandler with a group name applied to all handlers.
func (mh *MultiHandler) WithGroup(name string) slog.Handler {
	mh.mu.RLock()
	newHandlers := make([]slog.Handler, len(mh.handlers))
	for i, h := range mh.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	mh.mu.RUnlock()
	return NewMultiHandler(
		WithHandlers(newHandlers...),
		WithStrategy(mh.strategy),
		WithErrorChannel(mh.errors),
	)
}

// Flush triggers the strategy to flush any pending logs.
func (mh *MultiHandler) Flush() {
	mh.strategy.Flush()
}

// Close flushes any remaining logs and stops the worker.
// It closes the error channel after all logs are processed.
// Call this when the handler is no longer needed to ensure proper cleanup.
func (mh *MultiHandler) Close() error {
	mh.strategy.Close()
	return nil
}
