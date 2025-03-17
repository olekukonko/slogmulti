package slogmulti

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type mockHandler struct {
	mu        sync.Mutex
	records   []slog.Record
	shouldErr bool
}

func (h *mockHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
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
	mh := NewDefaultMultiHandler(h)

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
		WithBatchSize(2),
		WithFlushInterval(500*time.Millisecond),
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
	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	records := h.getRecords()
	if len(records) != 2 {
		t.Errorf("expected 2 records after batch size reached, got %d", len(records))
	}
}

func TestFlushInterval(t *testing.T) {
	h := &mockHandler{}
	mh := NewMultiHandler(
		WithHandlers(h),
		WithBatchSize(5),
		WithFlushInterval(100*time.Millisecond),
	)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "flush test", 0)
	mh.Handle(ctx, r)

	time.Sleep(50 * time.Millisecond)
	if len(h.getRecords()) != 0 {
		t.Errorf("expected no records before flush interval, got %d", len(h.getRecords()))
	}

	time.Sleep(100 * time.Millisecond)
	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	records := h.getRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record after flush interval, got %d", len(records))
	}
}

func TestErrorPropagation(t *testing.T) {
	h := &mockHandler{shouldErr: true}
	mh := NewDefaultMultiHandler(h)

	errChan := mh.Errors()
	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "error test", 0)
	mh.Handle(ctx, r)

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}

	select {
	case err := <-errChan:
		if err == nil || err.Error() != "mock handler error" {
			t.Errorf("expected 'mock handler error', got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for error propagation")
	}
}

func TestMultipleHandlers(t *testing.T) {
	h1 := &mockHandler{}
	h2 := &mockHandler{}
	mh := NewDefaultMultiHandler(h1, h2)

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
	mh := NewDefaultMultiHandler(h1)

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
	mh := NewDefaultMultiHandler(h)

	attrs := []slog.Attr{slog.String("key", "value")}
	mhWithAttrs := mh.WithAttrs(attrs).(*MultiHandler)

	ctx := context.Background()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "attrs test", 0)
	mhWithAttrs.Handle(ctx, r)

	if err := mhWithAttrs.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
	// Do not close mh, as it's not used after WithAttrs

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
		WithBatchSize(5),
		WithFlushInterval(1*time.Second),
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

	_, ok := <-mh.Errors()
	if ok {
		t.Error("expected error channel to be closed after Close")
	}
}

//func TestQueueFull(t *testing.T) {
//	h := &mockHandler{}
//	mh := NewMultiHandler(
//		WithHandlers(h),
//		WithBatchSize(2),                  // Batch size larger than queue capacity
//		WithFlushInterval(10*time.Second), // Long flush interval to avoid ticker flush
//	)
//
//	// Override queue with capacity 1
//	mh.queue = make(chan Ingress, 1)
//
//	ctx := context.Background()
//	r1 := slog.NewRecord(time.Now(), slog.LevelInfo, "queue1", 0)
//	r2 := slog.NewRecord(time.Now(), slog.LevelInfo, "queue2", 0)
//
//	// Fill the queue with r1
//	mh.Handle(ctx, r1)
//
//	// Attempt to enqueue r2 in a goroutine; should block until worker processes r1
//	done := make(chan struct{})
//	go func() {
//		mh.Handle(ctx, r2)
//		close(done)
//	}()
//
//	// Wait briefly to ensure r2 is blocked (queue full)
//	time.Sleep(50 * time.Millisecond)
//
//	// Check records: should only have r1 processed so far
//	records := h.getRecords()
//	if len(records) != 1 {
//		t.Errorf("expected 1 record before queue is full, got %d", len(records))
//	}
//
//	// Close the handler to flush remaining records
//	if err := mh.Close(); err != nil {
//		t.Fatalf("Close returned unexpected error: %v", err)
//	}
//
//	// After close, both records should be processed
//	records = h.getRecords()
//	if len(records) != 2 {
//		t.Errorf("expected 2 records after close, got %d", len(records))
//	}
//
//	// Ensure the goroutine completed
//	select {
//	case <-done:
//		// Success
//	case <-time.After(1 * time.Second):
//		t.Error("timeout waiting for r2 to be processed")
//	}
//}

func TestEnabled(t *testing.T) {
	mh := NewDefaultMultiHandler(&mockHandler{})

	ctx := context.Background()
	if !mh.Enabled(ctx, slog.LevelDebug) {
		t.Error("expected Enabled to return true for Debug")
	}
	if !mh.Enabled(ctx, slog.LevelError) {
		t.Error("expected Enabled to return true for Error")
	}

	if err := mh.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
}
