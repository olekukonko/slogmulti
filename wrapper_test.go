package slogmulti

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestWrapper_Handle(t *testing.T) {
	// Fixed time for testing
	fixedTime := time.Date(2025, 3, 18, 0, 6, 47, 0, time.UTC)

	tests := []struct {
		name      string
		record    slog.Record
		expected  string
		expectErr bool
	}{
		{
			name: "log at Info level",
			record: func() slog.Record {
				r := slog.NewRecord(fixedTime, slog.LevelInfo, "Test info log", 0)
				r.AddAttrs(slog.String("key", "value")) // Example attribute
				return r
			}(),
			expected:  "2025-03-18 00:06:47 [INFO] Test info log key=value\n",
			expectErr: false,
		},
		{
			name: "log at Debug level, level is higher",
			record: func() slog.Record {
				return slog.NewRecord(fixedTime, slog.LevelDebug, "Test debug log", 0)
			}(),
			expected:  "", // Debug logs should not be written because the handler level is Info
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := NewWrapper(&buf, &slog.HandlerOptions{
				Level: slog.LevelInfo, // Set level to Info for this test
			})

			// Handle the log record
			err := handler.Handle(context.Background(), tt.record)
			if tt.expectErr && err == nil {
				t.Errorf("expected error, but got nil")
			} else if !tt.expectErr && err != nil {
				t.Errorf("expected no error, but got %v", err)
			}

			// Check the output
			actual := buf.String()
			if tt.expected != actual {
				t.Errorf("expected log output:\n%q\nbut got:\n%q", tt.expected, actual)
			}
		})
	}
}

func TestWrapper_Enabled(t *testing.T) {
	handler := NewWrapper(&bytes.Buffer{}, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	tests := []struct {
		name   string
		level  slog.Level
		expect bool
	}{
		{
			name:   "enabled for Info level",
			level:  slog.LevelInfo,
			expect: true,
		},
		{
			name:   "disabled for Debug level",
			level:  slog.LevelDebug,
			expect: false,
		},
		{
			name:   "enabled for Warn level",
			level:  slog.LevelWarn,
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := handler.Enabled(context.Background(), tt.level)
			if actual != tt.expect {
				t.Errorf("expected Enabled(%v) = %v, but got %v", tt.level, tt.expect, actual)
			}
		})
	}
}

func TestWrapper_WithAttrs(t *testing.T) {
	handler := NewWrapper(&bytes.Buffer{}, nil)

	attrs := []slog.Attr{
		{
			Key:   "user",
			Value: slog.StringValue("testuser"),
		},
	}

	// Add attributes using WithAttrs
	newHandler := handler.WithAttrs(attrs)

	// Assert that the new handler has the added attributes
	if len(newHandler.(*Wrapper).attrs) != 1 {
		t.Errorf("expected 1 attribute, but got %d", len(newHandler.(*Wrapper).attrs))
	}

	// Compare individual fields of the attributes
	if newHandler.(*Wrapper).attrs[0].Key != attrs[0].Key {
		t.Errorf("expected Key %q, but got %q", attrs[0].Key, newHandler.(*Wrapper).attrs[0].Key)
	}

	// Manually compare the values by using the String() method (assuming Value is a slog.Value)
	if newHandler.(*Wrapper).attrs[0].Value.String() != attrs[0].Value.String() {
		t.Errorf("expected Value %q, but got %q", attrs[0].Value.String(), newHandler.(*Wrapper).attrs[0].Value.String())
	}
}

func TestWrapper_WithGroup(t *testing.T) {
	handler := NewWrapper(&bytes.Buffer{}, nil)

	// Add group using WithGroup
	newHandler := handler.WithGroup("group1")

	// Assert that the new handler has the added group
	if len(newHandler.(*Wrapper).groups) != 1 {
		t.Errorf("expected 1 group, but got %d", len(newHandler.(*Wrapper).groups))
	}
	if newHandler.(*Wrapper).groups[0] != "group1" {
		t.Errorf("expected group %q, but got %q", "group1", newHandler.(*Wrapper).groups[0])
	}
}

func TestWrapper_Close(t *testing.T) {
	var buf bytes.Buffer
	handler := NewWrapper(&buf, nil)

	// Since Close is not part of the slog.Handler interface, we assert it to the concrete type (*Wrapper)
	if w, ok := handler.(*Wrapper); ok {
		// Call the Close method on the concrete type (Wrapper)
		err := w.Close()
		if err != nil {
			t.Errorf("expected no error, but got %v", err)
		}
	} else {
		t.Errorf("handler is not of type *Wrapper")
	}
}

// Helper function to check if a string contains a substring
func contains(str, substr string) bool {
	return len(str) >= len(substr) && str[:len(substr)] == substr
}
