package slogmulti

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// WrapperFormatFunc is a function type for formatting log records.
type WrapperFormatFunc func(r slog.Record) ([]byte, error)

// Wrapper is a wrapper struct that converts an io.Writer into a slog.Handler.
type Wrapper struct {
	writer io.Writer
	opts   *slog.HandlerOptions
	attrs  []slog.Attr
	groups []string
	format WrapperFormatFunc // Custom formatting function
}

// NewWrapper creates a new slog.Handler that wraps an io.Writer.
// By default, it uses a plain text formatter.
func NewWrapper(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &Wrapper{
		writer: w,
		opts:   opts,
		format: WrapperTextFormatter, // Default to text formatting
	}
}

// SetFormat sets a custom formatting function for the handler.
func (w *Wrapper) SetFormat(format WrapperFormatFunc) {
	w.format = format
}

// Handle processes a log record and writes it to the io.Writer using the configured formatter.
func (w *Wrapper) Handle(ctx context.Context, r slog.Record) error {
	// Check if the record's level is enabled
	if !w.Enabled(ctx, r.Level) {
		return nil
	}

	// Format the log record using the configured formatter
	formatted, err := w.format(r)
	if err != nil {
		return fmt.Errorf("failed to format log record: %w", err)
	}

	// Write the formatted log line to the underlying writer
	_, err = w.writer.Write(formatted)
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

// WrapperTextFormatter formats a log record as plain text.
func WrapperTextFormatter(r slog.Record) ([]byte, error) {
	// Format the log entry
	logLine := fmt.Sprintf("%s [%s] %s", r.Time.Format(time.DateTime), r.Level, r.Message)

	// Add attributes from the record
	r.Attrs(func(attr slog.Attr) bool {
		logLine += fmt.Sprintf(" %s=%v", attr.Key, attr.Value)
		return true
	})

	logLine += "\n"
	return []byte(logLine), nil
}

// WrapperJSONFormatter formats a log record as JSON.
func WrapperJSONFormatter(r slog.Record) ([]byte, error) {
	// Create a map to hold the structured log data
	logData := make(map[string]interface{})

	// Add basic log fields
	logData["time"] = r.Time.Format(time.RFC3339)
	logData["level"] = r.Level.String()
	logData["message"] = r.Message

	// Add attributes from the record
	r.Attrs(func(attr slog.Attr) bool {
		logData[attr.Key] = attr.Value.Any()
		return true
	})

	// Marshal the log data to JSON
	return json.Marshal(logData)
}
