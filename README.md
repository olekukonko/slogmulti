# slogmulti

[![ci](https://github.com/olekukonko/slogmulti/workflows/ci/badge.svg?branch=master)](https://github.com/olekukonko/slogmulti/actions?query=workflow%3Aci)
[![Total views](https://img.shields.io/sourcegraph/rrc/github.com/olekukonko/slogmulti.svg)](https://sourcegraph.com/github.com/olekukonko/slogmulti)
[![Godoc](https://godoc.org/github.com/olekukonko/slogmulti?status.svg)](https://godoc.org/github.com/olekukonko/slogmulti)

`slogmulti` is a Go package that enhances the standard `log/slog` package with a `MultiHandler` to combine multiple `slog.Handler` instances into a single, efficient handler. It supports batching and asynchronous processing under the hood, with errors propagated through a dedicated channel. Additionally, it provides a `Wrapper` type to turn any `io.Writer` into a `slog.Handler` with customizable formatting.

This package is perfect for logging to multiple destinations (e.g., console, files, databases) with high performance and straightforward error handling, without requiring users to manage strategies unless desired.

---

## Features

- **Multiple Handlers**: Send logs to several `slog.Handler` instances at once.
- **Efficient Processing**: Default asynchronous batching for performance (configurable if needed).
- **Error Propagation**: Collect handler errors via a dedicated channel.
- **Dynamic Updates**: Add handlers at runtime with ease.
- **Custom Formatting**: Use `Wrapper` to adapt `io.Writer` with custom log formats.
- **Simple Shutdown**: Ensure all logs are processed with a single `Close()` call.

---

## Installation

Install `slogmulti` with:

```bash
go get github.com/olekukonko/slogmulti
```

Requires Go 1.21 or later due to its dependency on `log/slog`.

---

## Usage

### Basic Example

Log to multiple destinations with minimal setup:

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

	// Combine handlers with defaults
	mh := slogmulti.NewDefaultMultiHandler(consoleHandler, jsonHandler)
	logger := slog.New(mh)

	// Log a message
	logger.Info("Hello, world!")

	// Clean up
	mh.Close()
}
```

### Adding Handlers Dynamically

Add more handlers after creation:

```go
package main

import (
	"log/slog"
	"os"
	"github.com/olekukonko/slogmulti"
)

func main() {
	// Start with one handler
	consoleHandler := slog.NewTextHandler(os.Stdout, nil)
	mh := slogmulti.NewDefaultMultiHandler(consoleHandler)
	logger := slog.New(mh)

	logger.Info("Initial log")

	// Add another handler later
	jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
	mh.Add(jsonHandler)

	logger.Info("Log to both handlers")

	mh.Close()
}
```

### Error Handling

Monitor errors from handlers:

```go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"github.com/olekukonko/slogmulti"
)

func main() {
	consoleHandler := slog.NewTextHandler(os.Stdout, nil)
	mh := slogmulti.NewDefaultMultiHandler(consoleHandler)
	logger := slog.New(mh)

	// Handle errors asynchronously
	go func() {
		for err := range mh.Errors() {
			fmt.Fprintf(os.Stderr, "Log error: %v\n", err)
		}
	}()

	logger.Info("Starting up")
	mh.Close()
}
```

### Custom Strategy (Optional)

For advanced users, customize processing with a strategy:

```go
package main

import (
	"log/slog"
	"os"
	"time"
	"github.com/olekukonko/slogmulti"
	"github.com/olekukonko/slogmulti/strategy"
)

func main() {
	consoleHandler := slog.NewTextHandler(os.Stdout, nil)
	jsonHandler := slog.NewJSONHandler(os.Stderr, nil)

	// Custom async strategy
	mh := slogmulti.NewMultiHandler(
		slogmulti.WithHandlers(consoleHandler, jsonHandler),
		slogmulti.WithStrategy(strategy.NewAsyncBatchStrategy(
			strategy.WithBatchSize(20),
			strategy.WithFlushInterval(500*time.Millisecond),
		)),
	)
	logger := slog.New(mh)

	logger.Info("Custom batching")
	mh.Close()
}
```

### Wrapping an `io.Writer`

Turn any `io.Writer` into a handler:

```go
package main

import (
	"log/slog"
	"os"
	"github.com/olekukonko/slogmulti"
)

func main() {
	handler := slogmulti.NewWrapper(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	logger.Info("Wrapped writer log")
	slogmulti.FnClose(handler)
}
```

### Custom Formatting with `Wrapper`

Use a custom format:

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
	customFormatter := func(r slog.Record) ([]byte, error) {
		return []byte(fmt.Sprintf("CUSTOM: %s [%s] %s\n", r.Time.Format(time.RFC3339), r.Level, r.Message)), nil
	}

	handler := slogmulti.NewWrapper(os.Stdout, nil)
	handler.(*slogmulti.Wrapper).SetFormat(customFormatter)
	logger := slog.New(handler)

	logger.Info("Custom format log")
	slogmulti.FnClose(handler)
}
```

### Combining `Wrapper` with `MultiHandler`

Log to multiple custom writers:

```go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"github.com/olekukonko/slogmulti"
)

type CustomWriter struct{}

func (w *CustomWriter) Write(p []byte) (n int, err error) {
	return fmt.Fprintf(os.Stdout, "Custom: %s", p)
}

func main() {
	h1 := slogmulti.NewWrapper(os.Stdout, nil)
	h2 := slogmulti.NewWrapper(&CustomWriter{}, nil)

	mh := slogmulti.NewDefaultMultiHandler(h1, h2)
	logger := slog.New(mh)

	logger.Info("Dual writer log")
	mh.Close()
}
```

### PostgreSQL Example

Log to a database and console:

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
	attrs := make(map[string]interface{})
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
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
	textHandler := slog.NewTextHandler(os.Stdout, nil)
	pgHandler, err := NewPostgresHandler("postgres://root:@localhost:26257/test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "PostgreSQL init failed: %v\n", err)
		return
	}

	mh := slogmulti.NewDefaultMultiHandler(textHandler, pgHandler)
	logger := slog.New(mh)

	logger.Info("App started", slog.String("version", "1.0"))
	logger.Error("Failure", slog.Any("error", errors.New("oops")))

	time.Sleep(100 * time.Millisecond)
	mh.Close()
}
```

---

## API

### Types

- **`MultiHandler`**: Combines multiple handlers with default async batching.
- **`Wrapper`**: Adapts an `io.Writer` into a `slog.Handler`.

### Functions

- **`NewMultiHandler(...MultiHandlerOption) *MultiHandler`**: Creates a custom `MultiHandler`.
- **`NewDefaultMultiHandler(...slog.Handler) *MultiHandler`**: Creates a `MultiHandler` with default async batching.
- **`NewWrapper(w io.Writer, opts *slog.HandlerOptions) slog.Handler`**: Wraps an `io.Writer`.
- **`FnClose(handler slog.Handler) error`**: Closes a handler if it supports `io.Closer`.

### Options for `MultiHandler`

- **`WithHandlers(...slog.Handler)`**: Sets initial handlers.
- **`WithStrategy(strategy.Handler)`**: Sets a custom strategy (e.g., `strategy.NewAsyncBatchStrategy`).
- **`WithErrorChannel(chan error)`**: Sets a custom error channel (default: capacity 100).

### `MultiHandler` Methods

- **`Add(...slog.Handler)`**: Adds handlers dynamically.
- **`Errors() <-chan error`**: Returns the error channel.
- **`Handle(ctx context.Context, r slog.Record) error`**: Queues a log.
- **`Enabled(ctx context.Context, level slog.Level) bool`**: Delegates to handlers.
- **`WithAttrs(attrs []slog.Attr) slog.Handler`**: Adds attributes.
- **`WithGroup(name string) slog.Handler`**: Adds a group.
- **`Flush()`**: Flushes pending logs.
- **`Close() error`**: Flushes and shuts down.

### `Wrapper` Methods

- **`SetFormat(format WrapperFormatFunc)`**: Sets a custom formatter.
- **`Handle(ctx context.Context, r slog.Record) error`**: Writes formatted logs.
- **`Enabled(ctx context.Context, level slog.Level) bool`**: Checks level.
- **`WithAttrs(attrs []slog.Attr) slog.Handler`**: Adds attributes.
- **`WithGroup(name string) slog.Handler`**: Adds a group.
- **`Close() error`**: Closes the writer if applicable.

### Strategy (Optional)

- **`strategy.NewAsyncBatchStrategy(...AsyncBatchStrategyOption) *strategy.Async`**: Custom async batching.
- **`strategy.NewSyncStrategy(errors chan<- error) *strategy.Sync`**: Synchronous processing.
- Options: `strategy.WithBatchSize(int)`, `strategy.WithFlushInterval(time.Duration)`.

---

## Defaults

- **Batch Size**: `10` (async strategy)
- **Flush Interval**: `1 second` (async strategy)
- **Queue Size**: `1000` (async strategy)
- **Error Channel Capacity**: `100`

---

## Notes

- **Default Behavior**: `NewDefaultMultiHandler` uses async batching; use `WithStrategy` for sync or custom behavior.
- **Error Handling**: Errors are dropped if the channel is full, logged to `os.Stderr`.
- **Thread Safety**: `MultiHandler` is concurrent-safe; `Wrapper` depends on the `io.Writer`.

---

## Testing

Run tests with:

```bash
go test -v
```

---

## License

MIT License. See [LICENSE](LICENSE).
