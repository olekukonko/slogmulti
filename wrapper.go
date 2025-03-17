package slogmulti

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// Wrapper is a wrapper struct that converts an io.WriteCloser into a slog.Handler.
type Wrapper struct {
	writer io.Writer
	opts   *slog.HandlerOptions
	attrs  []slog.Attr
	groups []string
}

// NewWrapper creates a new slog.Handler that wraps an io.WriteCloser.
func NewWrapper(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &Wrapper{
		writer: w,
		opts:   opts,
	}
}

// Handle processes a log record and writes it to the io.WriteCloser.
func (w *Wrapper) Handle(ctx context.Context, r slog.Record) error {
	// Check if the record's level is enabled
	if !w.Enabled(ctx, r.Level) {
		return nil
	}

	// Format the log entry
	logLine := fmt.Sprintf("%s [%s] %s", r.Time.Format(time.DateTime), r.Level, r.Message)

	// Add attributes from the handler
	for _, attr := range w.attrs {
		logLine += fmt.Sprintf(" %s=%v", attr.Key, attr.Value)
	}

	// Add attributes from the record
	r.Attrs(func(attr slog.Attr) bool {
		logLine += fmt.Sprintf(" %s=%v", attr.Key, attr.Value)
		return true
	})

	logLine += "\n"

	// Write the log line to the underlying WriteCloser
	_, err := w.writer.Write([]byte(logLine))
	return err
}

// Enabled checks if the log level is enabled based on the handler options.
func (w *Wrapper) Enabled(ctx context.Context, level slog.Level) bool {
	if w.opts.Level != nil {
		return level >= w.opts.Level.Level()
	}
	return true
}

// WithAttrs returns a new handler with additional attributes.
func (w *Wrapper) WithAttrs(attrs []slog.Attr) slog.Handler {
	newWrapper := *w
	newWrapper.attrs = append(newWrapper.attrs, attrs...)
	return &newWrapper
}

// WithGroup returns a new handler with a group name applied to all attributes.
func (w *Wrapper) WithGroup(name string) slog.Handler {
	newWrapper := *w
	newWrapper.groups = append(newWrapper.groups, name)
	return &newWrapper
}

// Close method checks if the writer implements io.Closer.
func (w *Wrapper) Close() error {
	if closer, ok := w.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
