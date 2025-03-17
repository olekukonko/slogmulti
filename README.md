# slogmulti

[![ci](https://github.com/olekukonko/slogmulti/workflows/ci/badge.svg?branch=master)](https://github.com/olekukonko/slogmulti/actions?query=workflow%3Aci)
[![Total views](https://img.shields.io/sourcegraph/rrc/github.com/olekukonko/slogmulti.svg)](https://sourcegraph.com/github.com/olekukonko/slogmulti)
[![Godoc](https://godoc.org/github.com/olekukonko/slogmulti?status.svg)](https://godoc.org/github.com/olekukonko/slogmulti)


`slogmulti` is a Go package that provides a `MultiHandler` for the `log/slog` package. It allows you to combine multiple `slog.Handler` instances into a single handler, batching log entries for efficiency and providing asynchronous error propagation. This is particularly useful for applications that need to dispatch logs to multiple destinations (e.g., console, file, and remote services) while optimizing performance.


## Features

- **Multiple Handlers**: Dispatch logs to multiple `slog.Handler` instances simultaneously.
- **Batching**: Group log entries into batches to reduce overhead, configurable by size or flush interval.
- **Asynchronous Processing**: Logs are queued and processed in the background, improving throughput.
- **Error Propagation**: Errors from underlying handlers are collected and accessible via a dedicated channel.
- **Dynamic Configuration**: Add handlers dynamically and customize batching behavior with options.
- **Clean Shutdown**: Ensures all queued logs are processed before closing.

## Installation

To use `slogmulti` in your Go project, install it with:

```bash
go get github.com/olekukonko/slogmulti
```

Replace `github.com/olekukonko/slogmulti` with the actual repository path once you’ve published it.

Requires Go 1.21 or later (due to `log/slog` dependency).

## Usage

### Basic Example

Create a `MultiHandler` with default settings and log to multiple handlers:

```go
package main

import (
    "log/slog"
    "os"
    "github.com/olekukonko/slogmulti"
)

func main() {
    // Create handlers (e.g., console and JSON file)
    consoleHandler := slog.NewTextHandler(os.Stdout, nil)
    jsonHandler := slog.NewJSONHandler(os.Stderr, nil)

    // Create MultiHandler with default settings
    mh := slogmulti.NewDefaultMultiHandler(consoleHandler, jsonHandler)
    logger := slog.New(mh)

    // Log a message
    logger.Info("Hello, world!")

    // Close the handler to flush remaining logs
    mh.Close()
}
```

### Custom Configuration

Configure batch size, flush interval, and error handling:

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
    consoleHandler := slog.NewTextHandler(os.Stdout, nil)
    jsonHandler := slog.NewJSONHandler(os.Stderr, nil)

    // Custom MultiHandler with options
    mh := slogmulti.NewMultiHandler(
        slogmulti.WithHandlers(consoleHandler, jsonHandler),
        slogmulti.WithBatchSize(20),             // Batch up to 20 logs
        slogmulti.WithFlushInterval(500*time.Millisecond), // Flush every 500ms
    )

    logger := slog.New(mh)

    // Log with context
    ctx := context.Background()
    logger.InfoContext(ctx, "Starting application")

    // Handle errors asynchronously
    go func() {
        for err := range mh.Errors() {
            fmt.Fprintf(os.Stderr, "Log error: %v\n", err)
        }
    }()

    // Add another handler dynamically
    extraHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
    mh.Add(extraHandler)

    // Clean up
    mh.Close()
}
```


### Postgres Example 

Here is a simple Postgres Example 

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

// PostgresHandler for sending logs to PostgreSQL
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
		)
	`)
	if err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("failed to create PostgreSQL table: %v", err)
	}
	return h, nil
}

func (h *PostgresHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := slogmulti.FnExtract(r)
	data, err := json.Marshal(attrs)
	if err != nil {
		return fmt.Errorf("failed to marshal attrs: %v", err)
	}
	_, err = h.conn.Exec(ctx,
		"INSERT INTO logs (uid, timestamp, level, message, attrs) VALUES ($1, $2, $3, $4, $5)",
		ulid.Make().String(), r.Time, r.Level.String(), r.Message, data)
	return err
}

func (h *PostgresHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *PostgresHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h // We'll handle attrs in Handle method
}

func (h *PostgresHandler) WithGroup(name string) slog.Handler {
	return h // Postgres doesn't need groups
}

func (h *PostgresHandler) Close() error {
	return h.conn.Close(context.Background())
}


func main() {
	// Text handler for stdout
	textHandler := slog.NewTextHandler(os.Stdout, nil)

	// Postgres handler
	postgresHandler, err := NewPostgresHandler("postgres://root:@localhost:26257/test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize PostgreSQL: %v\n", err)
		return
	}

	// Combine handlers
	multiHandler := slogmulti.NewDefaultMultiHandler(textHandler, postgresHandler)
	logger := slog.New(multiHandler)

	type usr struct {
		Username string `db:"username"`
		Level    string `db:"level"`
	}
	// Example logs
	logger.Info("Starting application", slog.String("version", "1.0"))
	logger.Error("Something failed", slog.Any("error", errors.New("oops")))
	logger.Error("Something struct",
		slog.Any("err", errors.New("dancing")),
		slog.Any("user", usr{Username: "test", Level: "info"}),
	)

	// Cleanup
	time.Sleep(2 * time.Second)
	if err := multiHandler.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Error closing multiHandler: %v\n", err)
	}
}

```
### Error Handling

Errors from handlers are sent to the `Errors()` channel:

```go
for err := range mh.Errors() {
    fmt.Println("Handler error:", err)
}
```

### Adding Attributes

Create a new handler with additional attributes:

```go
mhWithAttrs := mh.WithAttrs([]slog.Attr{slog.String("app", "myapp")})
logger = slog.New(mhWithAttrs)
logger.Info("Log with attributes")
```

## API

### Types

- **`MultiHandler`**: The core type that manages multiple handlers, batching, and error propagation.
- **`Ingress`**: Internal struct representing a log entry with context, record, and optional error.

### Functions

- **`NewMultiHandler(...MultiHandlerOption) *MultiHandler`**: Creates a new `MultiHandler` with custom options.
- **`NewDefaultMultiHandler(...slog.Handler) *MultiHandler`**: Creates a `MultiHandler` with default settings and the given handlers.

### Options

- **`WithHandlers(...slog.Handler)`**: Specifies the handlers to use.
- **`WithBatchSize(int)`**: Sets the maximum batch size (default: 10).
- **`WithFlushInterval(time.Duration)`**: Sets the flush interval (default: 1 second).
- **`WithErrorChannel(chan error)`**: Sets a custom error channel (default: capacity 100).

### Methods

- **`Add(...slog.Handler)`**: Dynamically adds handlers.
- **`Errors() <-chan error`**: Returns the channel for handler errors.
- **`Handle(context.Context, slog.Record) error`**: Queues a log record for processing.
- **`Enabled(context.Context, slog.Level) bool`**: Always returns `true`, delegating to underlying handlers.
- **`WithAttrs([]slog.Attr) slog.Handler`**: Returns a new handler with added attributes.
- **`WithGroup(string) slog.Handler`**: Returns a new handler with a group name.
- **`Close() error`**: Flushes remaining logs and shuts down the worker.

## Defaults

- **Batch Size**: 10
- **Flush Interval**: 1 second
- **Queue Size**: 1000
- **Error Channel Capacity**: 100

## Notes

- The `MultiHandler` processes logs asynchronously. Use `Close()` to ensure all logs are flushed before program exit.
- If the error channel is full, errors are dropped with a message to `os.Stderr`.
- The `Enabled` method always returns `true`, relying on underlying handlers for level filtering.

## Testing

The package includes comprehensive tests. Run them with:

```bash
go test -v
```

## Contributing

Contributions are welcome! Please submit pull requests or open issues on the [repository](https://github.com/olekukonko/slogmulti).

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---