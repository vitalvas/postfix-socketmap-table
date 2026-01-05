# Postfix SocketMap Table Server

A Go library for implementing Postfix socketmap lookup tables. This library provides a server that implements the [Postfix socketmap protocol](https://www.postfix.org/socketmap_table.5.html), allowing you to create custom lookup tables for Postfix mail server.

## Installation

```bash
go get github.com/vitalvas/postfix-socketmap-table
```

## Usage

### Basic Example

```go
package main

import (
    "context"
    "log"

    socketmap "github.com/vitalvas/postfix-socketmap-table"
)

func main() {
    server := socketmap.NewServer(nil)

    // Register a lookup function for a specific map
    server.RegisterMap("virtual_domains", func(ctx context.Context, key string) (*socketmap.Result, error) {
        if key == "example.com" {
            return socketmap.ReplyOK("example.com"), nil
        }
        return socketmap.ReplyNotFound(), nil
    })

    if err := server.ListenAndServe(context.Background(), "127.0.0.1:27823"); err != nil {
        log.Fatal(err)
    }
}
```

### Postfix Configuration

Add to your Postfix configuration (`main.cf`):

```
virtual_mailbox_domains = socketmap:inet:127.0.0.1:27823:virtual_domains
```

## API

### Server

```go
// Create a new server with default configuration
server := socketmap.NewServer(nil)

// Create a new server with custom configuration
server := socketmap.NewServer(&socketmap.Config{
    Port:         27823,
    ReadTimeout:  100 * time.Second,
    WriteTimeout: 100 * time.Second,
    IdleTimeout:  10 * time.Second,
    MaxReplySize: 100000,
})
```

### Lookup Functions

```go
// LookupFn - for specific map lookups
type LookupFn func(ctx context.Context, key string) (*Result, error)

// DefaultLookupFn - fallback for unknown maps
type DefaultLookupFn func(ctx context.Context, dict, key string) (*Result, error)
```

Register handlers:

```go
// Register a handler for a specific map name
server.RegisterMap("mapname", func(ctx context.Context, key string) (*socketmap.Result, error) {
    // lookup logic
    return socketmap.ReplyOK(value), nil
})

// Register a default handler for unknown maps
server.RegisterDefaultMap(func(ctx context.Context, dict, key string) (*socketmap.Result, error) {
    // fallback lookup logic
    return socketmap.ReplyOK(value), nil
})
```

### Reply Types

```go
socketmap.ReplyOK(data string)           // Successful lookup
socketmap.ReplyNotFound()                // Key not found
socketmap.ReplyTempFail(reason string)   // Temporary failure
socketmap.ReplyPermFail(reason string)   // Permanent failure
socketmap.ReplyTimeout(reason string)    // Timeout
```

### Server Lifecycle

```go
// Start TCP server
server.ListenAndServe(ctx, "127.0.0.1:27823")

// Start Unix socket server
server.ListenAndServeUnix(ctx, "/var/run/socketmap.sock")

// Graceful shutdown
server.Shutdown(ctx)

// Immediate close
server.Close()
```

### Unix Socket Configuration

For Unix socket support, configure Postfix (`main.cf`):

```
virtual_mailbox_domains = socketmap:unix:/var/run/socketmap.sock:virtual_domains
```

## Configuration

| Option | Default | Description |
|--------|---------|-------------|
| `Port` | 27823 | Default TCP port for the server |
| `ReadTimeout` | 100s | Maximum duration for reading a request |
| `WriteTimeout` | 100s | Maximum duration for writing a response |
| `IdleTimeout` | 10s | Maximum duration to wait for the next request |
| `MaxReplySize` | 100000 | Maximum reply size in bytes (0 = no limit) |

Default values match Postfix defaults.

## License

MIT License
