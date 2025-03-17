package slogmulti

import (
	"io"
	"log/slog"
)

// FnExtract extracts the attributes from a slog.Record and returns them as a map.
// The map's keys are the attribute keys, and the values are the corresponding attribute values.
func FnExtract(r slog.Record) map[string]interface{} {
	attrs := make(map[string]interface{})
	r.Attrs(func(a slog.Attr) bool {
		switch a.Value.Kind() {
		case slog.KindString:
			attrs[a.Key] = a.Value.String()
		case slog.KindInt64:
			attrs[a.Key] = a.Value.Int64()
		case slog.KindFloat64:
			attrs[a.Key] = a.Value.Float64()
		case slog.KindBool:
			attrs[a.Key] = a.Value.Bool()
		case slog.KindTime:
			attrs[a.Key] = a.Value.Time()
		case slog.KindAny:
			if err, ok := a.Value.Any().(error); ok {
				attrs[a.Key] = err.Error() // Convert error to string
			} else {
				attrs[a.Key] = a.Value.Any() // Fallback to raw value
			}
		default:
			attrs[a.Key] = a.Value.String() // Safe fallback for unhandled kinds
		}
		return true
	})
	return attrs
}

// FnCloseHandler closes the handler if it implements the io.Closer interface.
// It returns an error if the handler cannot be closed or if the handler does not support closing.
func FnCloseHandler(handler slog.Handler) error {
	if closer, ok := handler.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
