package slogmulti

import (
	"context"
	"errors"
	"github.com/olekukonko/slogmulti/strategy"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type mockHandler struct {
	mu        sync.Mutex
	records   []slog.Record
	shouldErr bool
	processed chan struct{} // Signal when a record is processed (for testing)
}

func (h *mockHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	if h.processed != nil {
		h.processed <- struct{}{}
	}
	if h.shouldErr {
		return errors.New("mock handler error")
	}
	return nil
}

func (h *mockHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *mockHandler) WithGroup(name string) slog.Handler {
	return h
}

func (h *mockHandler) getRecords() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

func TestBasicLogging(t *testing.T) {
	h := &mockHandler{}
	mh := NewMultiHandler(WithHandlers(h))

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	if err := mh.Handle(ctx, r); err != nil {
		t.Fatalf("Handle returned unexpected error: %v", err)
	}

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	records := h.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
		return
	}
	if records[0].Message != "test message" {
		t.Errorf("expected message 'test message', got %q", records[0].Message)
	}
}

func TestBatching(t *testing.T) {
	h := &mockHandler{}
	mh := NewMultiHandler(
		WithHandlers(h),
		WithStrategy(strategy.NewAsyncBatchStrategy(strategy.WithBatchSize(2), strategy.WithFlushInterval(500*time.Millisecond))),
	)

	ctx := context.Background()
	r1 := slog.NewRecord(time.Now(), slog.LevelInfo, "log1", 0)
	r2 := slog.NewRecord(time.Now(), slog.LevelInfo, "log2", 0)

	mh.Handle(ctx, r1)
	time.Sleep(50 * time.Millisecond)
	if len(h.getRecords()) != 0 {
		t.Errorf("expected no records before batch size reached, got %d", len(h.getRecords()))
	}

	mh.Handle(ctx, r2)
	time.Sleep(50 * time.Millisecond)
	records := h.getRecords()
	if len(records) != 2 {
		t.Errorf("expected 2 records after batch size reached, got %d", len(records))
	}

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}

func TestFlushInterval(t *testing.T) {
	h := &mockHandler{}
	mh := NewMultiHandler(
		WithHandlers(h),
		WithStrategy(strategy.NewAsyncBatchStrategy(strategy.WithBatchSize(5), strategy.WithFlushInterval(100*time.Millisecond))),
	)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "flush test", 0)
	mh.Handle(ctx, r)

	time.Sleep(50 * time.Millisecond)
	if len(h.getRecords()) != 0 {
		t.Errorf("expected no records before flush interval, got %d", len(h.getRecords()))
	}

	time.Sleep(100 * time.Millisecond)
	records := h.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record after flush interval, got %d", len(records))
	}

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}

func TestMultipleHandlers(t *testing.T) {
	h1 := &mockHandler{}
	h2 := &mockHandler{}
	mh := NewMultiHandler(WithHandlers(h1, h2))

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "multi test", 0)
	mh.Handle(ctx, r)

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	for i, h := range []*mockHandler{h1, h2} {
		records := h.getRecords()
		if len(records) != 1 {
			t.Errorf("handler %d: expected 1 record, got %d", i+1, len(records))
		}
	}
}

func TestAddHandler(t *testing.T) {
	h1 := &mockHandler{}
	mh := NewMultiHandler(WithHandlers(h1))

	h2 := &mockHandler{}
	mh.Add(h2)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "add test", 0)
	mh.Handle(ctx, r)

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	for i, h := range []*mockHandler{h1, h2} {
		records := h.getRecords()
		if len(records) != 1 {
			t.Errorf("handler %d: expected 1 record, got %d", i+1, len(records))
		}
	}
}

func TestWithAttrs(t *testing.T) {
	h := &mockHandler{}
	mh := NewMultiHandler(WithHandlers(h))

	attrs := []slog.Attr{slog.String("key", "value")}
	mhWithAttrs := mh.WithAttrs(attrs).(*MultiHandler)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "attrs test", 0)
	mhWithAttrs.Handle(ctx, r)

	if err := mhWithAttrs.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	records := h.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
		return
	}
	if records[0].Message != "attrs test" {
		t.Errorf("expected message 'attrs test', got %q", records[0].Message)
	}
}

func TestClose(t *testing.T) {
	h := &mockHandler{}
	mh := NewMultiHandler(
		WithHandlers(h),
		WithStrategy(strategy.NewAsyncBatchStrategy(strategy.WithBatchSize(5), strategy.WithFlushInterval(1*time.Second))),
	)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "close test", 0)
	mh.Handle(ctx, r)

	if err := mh.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	records := h.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record after close, got %d", len(records))
	}
}

func TestErrorPropagation(t *testing.T) {
	h := &mockHandler{shouldErr: true, processed: make(chan struct{}, 1)}
	mh := NewMultiHandler(
		WithHandlers(h),
		WithStrategy(strategy.NewAsyncBatchStrategy(strategy.WithFlushInterval(10*time.Millisecond))),
	)

	errChan := mh.Errors()
	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "error test", 0)
	mh.Handle(ctx, r)
	mh.Flush() // Force immediate flush

	// Wait for the record to be processed
	<-h.processed

	select {
	case err := <-errChan:
		if err == nil || err.Error() != "mock handler error" {
			t.Errorf("expected 'mock handler error', got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for error propagation")
	}

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}

func TestQueueFull(t *testing.T) {
	h := &mockHandler{processed: make(chan struct{}, 2)} // Buffer for both records
	mh := NewMultiHandler(
		WithHandlers(h),
		WithStrategy(strategy.NewAsyncBatchStrategy(
			strategy.WithBatchSize(2),
			strategy.WithFlushInterval(10*time.Millisecond),
		)),
	)

	// Override queue with capacity 1 to simulate a full queue
	abs := mh.strategy.(*strategy.Async)
	abs.ResizeQueue(1)

	ctx := context.Background()
	r1 := slog.NewRecord(time.Now(), slog.LevelInfo, "queue1", 0)
	r2 := slog.NewRecord(time.Now(), slog.LevelInfo, "queue2", 0)

	// Fill the queue with r1
	mh.Handle(ctx, r1)
	<-h.processed // Wait for r1 to be processed

	// Attempt to enqueue r2 in a goroutine; should block until queue is free
	done := make(chan struct{})
	go func() {
		mh.Handle(ctx, r2)
		close(done)
	}()

	// Check records: should only have r1 processed so far
	records := h.getRecords()
	t.Logf("Records after r1 processed: %d", len(records))
	if len(records) != 1 {
		t.Errorf("expected 1 record before queue is full, got %d", len(records))
	}

	// Wait for r2 to be processed
	<-h.processed

	// Close the handler to ensure cleanup
	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	// After close, both records should be processed
	records = h.getRecords()
	if len(records) != 2 {
		t.Errorf("expected 2 records after close, got %d", len(records))
	}

	// Ensure the goroutine completed
	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond): // Short timeout
		t.Error("timeout waiting for r2 to be processed")
	}
}

func TestSyncStrategy(t *testing.T) {
	h := &mockHandler{}
	mh := NewMultiHandler(
		WithHandlers(h),
		WithStrategy(strategy.NewSyncStrategy(nil)),
	)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "sync test", 0)
	mh.Handle(ctx, r)

	records := h.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record immediately with sync strategy, got %d", len(records))
		return
	}
	if records[0].Message != "sync test" {
		t.Errorf("expected message 'sync test', got %q", records[0].Message)
	}

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}
