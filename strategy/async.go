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

// Async processes logs asynchronously in batches using a worker goroutine.
type Async struct {
	queue         chan Ingress   // Buffered channel for log entries
	batchSize     int            // Maximum number of logs in a batch
	flushInterval time.Duration  // Interval at which batches are flushed
	wg            sync.WaitGroup // Wait group to ensure clean shutdown
	errors        chan<- error   // Channel to propagate errors (set by MultiHandler)
	handlers      []slog.Handler // Current list of handlers
	mu            sync.RWMutex   // Protects handlers
	flushChan     chan struct{}  // Channel to trigger manual flush
}

// AsyncBatchStrategyOption configures an Async.
type AsyncBatchStrategyOption func(*Async)

// WithBatchSize sets the maximum number of log entries per batch.
func WithBatchSize(size int) AsyncBatchStrategyOption {
	return func(abs *Async) {
		if size > 0 {
			abs.batchSize = size
		}
	}
}

// WithFlushInterval sets the interval at which batches are flushed.
func WithFlushInterval(interval time.Duration) AsyncBatchStrategyOption {
	return func(abs *Async) {
		if interval > 0 {
			abs.flushInterval = interval
		}
	}
}

// NewAsyncBatchStrategy creates a new Async with the given configuration.
func NewAsyncBatchStrategy(opts ...AsyncBatchStrategyOption) *Async {
	abs := &Async{
		queue:         make(chan Ingress, DefaultQueueSize),
		batchSize:     DefaultBatchSize,
		flushInterval: DefaultFlushInterval,
		handlers:      make([]slog.Handler, 0),
		flushChan:     make(chan struct{}, 1), // Buffered to avoid blocking
	}
	for _, opt := range opts {
		opt(abs)
	}
	abs.startWorkers()
	return abs
}

// Process queues a log record for asynchronous processing.
func (abs *Async) Process(ctx context.Context, r slog.Record, handlers []slog.Handler, errors chan<- error) {
	abs.mu.Lock()
	abs.handlers = handlers // Update the stored handlers
	abs.errors = errors     // Update the errors channel
	abs.mu.Unlock()
	abs.queue <- Ingress{ctx: ctx, r: r, err: nil}
}

// Flush triggers an immediate flush of any pending logs.
func (abs *Async) Flush() {
	select {
	case abs.flushChan <- struct{}{}:
	default: // Non-blocking if flush is already pending
	}
}

// ResizeQueue resizes the queue capacity (for testing or reconfiguration).
func (abs *Async) ResizeQueue(size int) {
	abs.mu.Lock()
	defer abs.mu.Unlock()
	abs.queue = make(chan Ingress, size)
}

// Close stops the worker and flushes any remaining logs.
func (abs *Async) Close() {
	close(abs.queue)
	abs.wg.Wait()
}

// startWorkers launches a goroutine to process queued log entries.
func (abs *Async) startWorkers() {
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
func (abs *Async) flushBatch(batch []Ingress) {
	abs.mu.RLock()
	handlers := abs.handlers
	errors := abs.errors
	abs.mu.RUnlock()

	for _, h := range handlers {
		for _, entry := range batch {
			if entry.err != nil {
				if errors != nil {
					select {
					case errors <- entry.err:
					default:
						fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", entry.err)
					}
				}
				continue
			}
			if err := h.Handle(entry.ctx, entry.r); err != nil {
				if errors != nil {
					select {
					case errors <- err:
					default:
						fmt.Fprintf(os.Stderr, "[slogmulti] dropped error due to full channel: %v\n", err)
					}
				}
			}
		}
	}
}
