# slogmulti

[![ci](https://github.com/olekukonko/slogmulti/workflows/ci/badge.svg?branch=master)](https://github.com/olekukonko/slogmulti/actions?query=workflow%3Aci)
[![Total views](https://img.shields.io/sourcegraph/rrc/github.com/olekukonko/slogmulti.svg)](https://sourcegraph.com/github.com/olekukonko/slogmulti)
[![Godoc](https://godoc.org/github.com/olekukonko/slogmulti?status.svg)](https://godoc.org/github.com/olekukonko/slogmulti)

`slogmulti` is a Go package that extends the standard `log/slog` package by providing a `MultiHandler` to combine multiple `slog.Handler` instances into a single, efficient handler. It batches log entries for performance, processes them asynchronously, and propagates errors through a dedicated channel. Additionally, it includes a `Wrapper` type to adapt any `io.Writer` into a `slog.Handler`, with customizable formatting.

This package is ideal for applications requiring logging to multiple destinations (e.g., console, files, databases) while maintaining high throughput and clean error handling.

---

## Features

- **Multiple Handlers**: Dispatch logs to several `slog.Handler` instances concurrently.
- **Batching**: Group logs into batches, configurable by size or flush interval, to minimize overhead.
- **Asynchronous Processing**: Queue logs for background processing, enhancing performance.
- **Error Propagation**: Collect and access errors from handlers via a dedicated channel.
- **Dynamic Configuration**: Add handlers or adjust settings at runtime.
- **Custom Formatting**: Use the `Wrapper` type to format logs with custom logic over any `io.Writer`.
- **Clean Shutdown**: Ensure all queued logs are processed before termination.

---

## Installation

Install `slogmulti` using:

```bash
go get github.com/olekukonko/slogmulti
```

Requires Go 1.21 or later due to its dependency on `log/slog`.

---

## Usage

### Basic Example

Log to multiple destinations with default settings:

```go
package main

import (
	"log/slog"
	"os"
	"github.com/olekukonko/slogmulti"
)

func main() {
	// Define handlers
	consoleHandler := slog.NewTextHandler(os.Stdout, nil)
	jsonHandler := slog.NewJSONHandler(os.Stderr, nil)

	// Create a MultiHandler with defaults
	mh := slogmulti.NewDefaultMultiHandler(consoleHandler, jsonHandler)
	logger := slog.New(mh)

	// Log a message
	logger.Info("Hello, world!")

	// Ensure all logs are flushed
	mh.Close()
}
```

### Custom Configuration

Customize batching and error handling:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
	"github.com/olekukonko/slogmulti"
)

func main() {
	// Define handlers
	consoleHandler := slog.NewTextHandler(os.Stdout, nil)
	jsonHandler := slog.NewJSONHandler(os.Stderr, nil)

	// Configure MultiHandler
	mh := slogmulti.NewMultiHandler(
		slogmulti.WithHandlers(consoleHandler, jsonHandler),
		slogmulti.WithBatchSize(20),              // Batch up to 20 logs
		slogmulti.WithFlushInterval(500*time.Millisecond), // Flush every 500ms
	)

	logger := slog.New(mh)

	// Log with context
	ctx := context.Background()
	logger.InfoContext(ctx, "Application starting")

	// Handle errors in a goroutine
	go func() {
		for err := range mh.Errors() {
			fmt.Fprintf(os.Stderr, "Log error: %v\n", err)
		}
	}()

	// Dynamically add a handler
	debugHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	mh.Add(debugHandler)

	// Shutdown cleanly
	mh.Close()
}
```

### PostgreSQL Example

Log to PostgreSQL and console simultaneously:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/oklog/ulid/v2"
	"github.com/olekukonko/slogmulti"
	"log/slog"
	"os"
	"time"
)

// PostgresHandler logs to a PostgreSQL database
type PostgresHandler struct {
	conn *pgx.Conn
}

func NewPostgresHandler(dsn string) (*PostgresHandler, error) {
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		return nil, err
	}
	h := &PostgresHandler{conn: conn}
	_, err = conn.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS logs (
			uid TEXT PRIMARY KEY,
			timestamp TIMESTAMP,
			level TEXT,
			message TEXT,
			attrs JSONB
		)`)
	if err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("failed to create table: %v", err)
	}
	return h, nil
}

func (h *PostgresHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := slogmulti.FnExtract(r)
	data, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("marshal attrs: %v", err)
	}
	_, err = h.conn.Exec(ctx,
		"INSERT INTO logs (uid, timestamp, level, message, attrs) VALUES ($1, $2, $3, $4, $5)",
		ulid.Make().String(), r.Time, r.Level.String(), r.Message, data)
	return err
}

func (h *PostgresHandler) Enabled(ctx context.Context, level slog.Level) bool { return true }
func (h *PostgresHandler) WithAttrs(attrs []slog.Attr) slog.Handler          { return h }
func (h *PostgresHandler) WithGroup(name string) slog.Handler                { return h }
func (h *PostgresHandler) Close() error                                      { return h.conn.Close(context.Background()) }

func main() {
	// Console handler
	textHandler := slog.NewTextHandler(os.Stdout, nil)

	// PostgreSQL handler
	pgHandler, err := NewPostgresHandler("postgres://root:@localhost:26257/test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "PostgreSQL init failed: %v\n", err)
		return
	}

	// Combine handlers
	mh := slogmulti.NewDefaultMultiHandler(textHandler, pgHandler)
	logger := slog.New(mh)

	// Example logs
	logger.Info("App started", slog.String("version", "1.0"))
	logger.Error("Failure", slog.Any("error", errors.New("oops")))

	// Wait briefly and cleanup
	time.Sleep(2 * time.Second)
	if err := mh.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Close error: %v\n", err)
	}
}
```

### Wrapping an `io.Writer`

Convert any `io.Writer` into a `slog.Handler`:

```go
package main

import (
	"log/slog"
	"os"
	"github.com/olekukonko/slogmulti"
)

func main() {
	// Wrap os.Stdout
	handler := slogmulti.NewWrapper(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	defer slogmulti.FnClose(handler)

	logger := slog.New(handler)
	logger.Info("App started", slog.String("version", "1.0"))
}
```

### Custom Formatting with `Wrapper`

Define a custom log format:

```go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"
	"github.com/olekukonko/slogmulti"
)

func main() {
	// Custom formatter
	customFormatter := func(r slog.Record) ([]byte, error) {
		return []byte(fmt.Sprintf("CUSTOM: %s [%s] %s\n", r.Time.Format(time.RFC3339), r.Level, r.Message)), nil
	}

	// Create and configure wrapper
	handler := slogmulti.NewWrapper(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	handler.(*slogmulti.Wrapper).SetFormat(customFormatter)

	logger := slog.New(handler)
	logger.Info("Custom log message")
}
```

### Custom `io.Writer` with `MultiHandler`

Use a custom `io.Writer` with `MultiHandler`:

```go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"github.com/olekukonko/slogmulti"
)

// CustomWriter is a custom io.Writer that writes to os.Stdout.
type CustomWriter struct{}

// Write implements the io.Writer interface.
func (w *CustomWriter) Write(p []byte) (n int, err error) {
	return fmt.Fprintf(os.Stdout, "%s", p)
}

func main() {
	// Wrap os.Stdout
	h1 := slogmulti.NewWrapper(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	defer slogmulti.FnClose(h1)

	// Wrap a custom writer
	customWriter := &CustomWriter{}
	h2 := slogmulti.NewWrapper(customWriter, &slog.HandlerOptions{Level: slog.LevelDebug})
	defer slogmulti.FnClose(h2)

	// Combine into MultiHandler
	mh := slogmulti.NewDefaultMultiHandler(h1, h2)
	defer mh.Close()

	logger := slog.New(mh)

	// Example logs
	logger.Info("App started", slog.String("version", "1.0"))
	logger.Error("Error occurred", slog.Any("error", fmt.Errorf("something went wrong")))
	logger.WithGroup("request").Info("Request received", slog.String("method", "GET"), slog.Int("status", 200))
}
```

### Error Handling

Monitor errors from handlers:

```go
for err := range mh.Errors() {
	fmt.Fprintf(os.Stderr, "Handler error: %v\n", err)
}
```

### Adding Attributes and Groups

Enhance logs with attributes or groups:

```go
mhWithAttrs := mh.WithAttrs([]slog.Attr{slog.String("app", "myapp")})
logger = slog.New(mhWithAttrs)
logger.Info("Log with attributes")

mhWithGroup := mh.WithGroup("request")
logger = slog.New(mhWithGroup)
logger.Info("Request processed", slog.Int("status", 200))
```

---

## API

### Types

- **`MultiHandler`**: Manages multiple handlers with batching and error propagation.
- **`Ingress`**: Internal log entry with context, record, and optional error.
- **`Wrapper`**: Adapts an `io.Writer` into a `slog.Handler` with customizable formatting.

### Functions

- **`NewMultiHandler(...MultiHandlerOption) *MultiHandler`**: Creates a `MultiHandler` with custom options.
- **`NewDefaultMultiHandler(...slog.Handler) *MultiHandler`**: Creates a `MultiHandler` with default settings.
- **`NewWrapper(w io.Writer, opts *slog.HandlerOptions) slog.Handler`**: Wraps an `io.Writer` into a handler.
- **`FnExtract(r slog.Record) map[string]interface{}`**: Extracts attributes from a record.
- **`FnClose(handler slog.Handler) error`**: Closes a handler if it implements `io.Closer`.

### Options for `MultiHandler`

- **`WithHandlers(...slog.Handler)`**: Sets the handlers.
- **`WithBatchSize(int)`**: Sets max batch size (default: 10).
- **`WithFlushInterval(time.Duration)`**: Sets flush interval (default: 1s).
- **`WithErrorChannel(chan error)`**: Sets a custom error channel (default: capacity 100).

### `MultiHandler` Methods

- **`Add(...slog.Handler)`**: Adds handlers dynamically.
- **`Errors() <-chan error`**: Returns the error channel.
- **`Handle(ctx context.Context, r slog.Record) error`**: Queues a log for processing.
- **`Enabled(ctx context.Context, level slog.Level) bool`**: Always `true`, delegates filtering to handlers.
- **`WithAttrs(attrs []slog.Attr) slog.Handler`**: Adds attributes to all handlers.
- **`WithGroup(name string) slog.Handler`**: Applies a group name to all handlers.
- **`Close() error`**: Flushes logs and shuts down.

### `Wrapper` Methods

- **`SetFormat(format WrapperFormatFunc)`**: Sets a custom formatter.
- **`Handle(ctx context.Context, r slog.Record) error`**: Writes formatted logs.
- **`Enabled(ctx context.Context, level slog.Level) bool`**: Checks level against options.
- **`WithAttrs(attrs []slog.Attr) slog.Handler`**: Adds attributes.
- **`WithGroup(name string) slog.Handler`**: Adds a group name.
- **`Close() error`**: Closes the writer if it’s an `io.Closer`.

---

## Defaults

- **Batch Size**: 10
- **Flush Interval**: 1 second
- **Queue Size**: 1000
- **Error Channel Capacity**: 100

---

## Notes

- **Asynchronous Nature**: Logs are processed in the background. Call `Close()` to flush remaining logs.
- **Error Dropping**: If the error channel is full, errors are dropped and logged to `os.Stderr`.
- **Level Filtering**: `MultiHandler.Enabled` always returns `true`, relying on underlying handlers.
- **Thread Safety**: `MultiHandler` is safe for concurrent use; `Wrapper` depends on the underlying `io.Writer`.

---

## Testing

Run the tests with:

```bash
go test -v
```

---

## Contributing

Contributions are welcome! Please submit pull requests or open issues on the [GitHub repository](https://github.com/olekukonko/slogmulti).

---

## License

Licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

### Changes Made

- **Restored `CustomWriter` Example**: Added a new subsection "Custom `io.Writer` with `MultiHandler`" with the exact `CustomWriter` example from your code, showing how to integrate it with `MultiHandler`.
- **Consistency**: Ensured the example uses the same style and structure as others (e.g., `defer` for cleanup, clear comments).
- **Context**: Positioned it after "Custom Formatting with `Wrapper`" to maintain a logical flow from basic wrapping to custom implementations.