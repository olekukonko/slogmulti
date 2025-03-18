package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

const (
	DefaultFlushInterval = 1 * time.Second // Default interval for flushing batches
	DefaultBatchSize     = 10              // Default maximum number of logs in a batch
	DefaultQueueSize     = 1000            // Default capacity of the log queue
)

// Ingress represents a log entry with context, record, and an optional error.
type Ingress struct {
	ctx context.Context // Context associated with the log entry
	r   slog.Record     // The log record to process
	err error           // Optional error from prior processing
}

// AsyncBatchStrategy processes logs asynchronously in batches using a worker goroutine.
type AsyncBatchStrategy struct {
	queue         chan Ingress   // Buffered channel for log entries
	batchSize     int            // Maximum number of logs in a batch
	flushInterval time.Duration  // Interval at which batches are flushed
	wg            sync.WaitGroup // Wait group to ensure clean shutdown
	errors        chan<- error   // Channel to propagate errors (borrowed from MultiHandler)
	handlers      []slog.Handler // Current list of handlers
	mu            sync.RWMutex   // Protects handlers
	flushChan     chan struct{}  // Channel to trigger manual flush
}

// AsyncBatchStrategyOption configures an AsyncBatchStrategy.
type AsyncBatchStrategyOption func(*AsyncBatchStrategy)

// WithBatchSize sets the maximum number of log entries per batch.
// If size <= 0, the default (10) is used.
func WithBatchSize(size int) AsyncBatchStrategyOption {
	return func(abs *AsyncBatchStrategy) {
		if size > 0 {
			abs.batchSize = size
		}
	}
}

// WithFlushInterval sets the interval at which batches are flushed.
// If interval <= 0, the default (1 second) is used.
func WithFlushInterval(interval time.Duration) AsyncBatchStrategyOption {
	return func(abs *AsyncBatchStrategy) {
		if interval > 0 {
			abs.flushInterval = interval
		}
	}
}

// NewAsyncBatchStrategy creates a new AsyncBatchStrategy with the given configuration.
// The errors channel is typically provided by MultiHandler; handlers can be updated later.
func NewAsyncBatchStrategy(errors chan<- error, handlers []slog.Handler, opts ...AsyncBatchStrategyOption) *AsyncBatchStrategy {
	abs := &AsyncBatchStrategy{
		queue:         make(chan Ingress, DefaultQueueSize),
		batchSize:     DefaultBatchSize,
		flushInterval: DefaultFlushInterval,
		errors:        errors,
		handlers:      handlers,
		flushChan:     make(chan struct{}, 1), // Buffered to avoid blocking
	}
	for _, opt := range opts {
		opt(abs)
	}
	abs.startWorkers()
	return abs
}

// Process queues a log record for asynchronous processing.
// It uses the handlers passed to it, ensuring the latest handler list is respected.
func (abs *AsyncBatchStrategy) Process(ctx context.Context, r slog.Record, handlers []slog.Handler, errors chan<- error) {
	abs.mu.Lock()
	abs.handlers = handlers // Update the stored handlers
	abs.mu.Unlock()
	abs.queue <- Ingress{ctx: ctx, r: r, err: nil}
}

// Flush triggers an immediate flush of any pending logs.
func (abs *AsyncBatchStrategy) Flush() {
	select {
	case abs.flushChan <- struct{}{}:
	default: // Non-blocking if flush is already pending
	}
}

// Close stops the worker and flushes any remaining logs.
func (abs *AsyncBatchStrategy) Close() {
	close(abs.queue)
	abs.wg.Wait()
}

// startWorkers launches a goroutine to process queued log entries.
func (abs *AsyncBatchStrategy) startWorkers() {
	abs.wg.Add(1)
	go func() {
		defer abs.wg.Done()
		batch := make([]Ingress, 0, abs.batchSize)
		ticker := time.NewTicker(abs.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case entry, ok := <-abs.queue:
				if !ok {
					abs.flushBatch(batch)
					return
				}
				batch = append(batch, entry)
				if len(batch) >= abs.batchSize {
					abs.flushBatch(batch)
					batch = batch[:0]
				}
			case <-ticker.C:
				if len(batch) > 0 {
					abs.flushBatch(batch)
					batch = batch[:0]
				}
			case <-abs.flushChan:
				if len(batch) > 0 {
					abs.flushBatch(batch)
					batch = batch[:0]
				}
			}
		}
	}()
}

// flushBatch dispatches a batch of log entries to all handlers.
func (abs *AsyncBatchStrategy) flushBatch(batch []Ingress) {
	abs.mu.RLock()
	handlers := abs.handlers
	abs.mu.RUnlock()

	for _, h := range handlers {
		for _, entry := range batch {
			if entry.err != nil {
				select {
				case abs.errors <- entry.err:
				default:
					fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", entry.err)
				}
				continue
			}
			if err := h.Handle(entry.ctx, entry.r); err != nil {
				select {
				case abs.errors <- err:
				default:
					fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", err)
				}
			}
		}
	}
}
