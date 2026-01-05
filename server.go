package socketmap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

var (
	// ErrServerClosed is returned when the server is shut down.
	ErrServerClosed = errors.New("server closed")

	// ErrReplyTooLarge is returned when a reply exceeds the maximum size.
	ErrReplyTooLarge = errors.New("reply exceeds maximum size")
)

// Config holds the server configuration options.
type Config struct {
	// Port is the default TCP port for the server.
	// Default is 27823.
	Port int

	// ReadTimeout is the maximum duration for reading a request.
	// Postfix default is 100 seconds.
	// Zero means no timeout.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration for writing a response.
	// Postfix default is 100 seconds.
	// Zero means no timeout.
	WriteTimeout time.Duration

	// IdleTimeout is the maximum duration to wait for the next request.
	// Postfix default is 10 seconds.
	// Zero means no timeout.
	IdleTimeout time.Duration

	// MaxReplySize is the maximum size of a reply in bytes.
	// Postfix default is 100,000 bytes.
	MaxReplySize int
}

// DefaultConfig returns a Config with Postfix-compatible defaults.
func DefaultConfig() *Config {
	return &Config{
		Port:         27823,
		ReadTimeout:  100 * time.Second,
		WriteTimeout: 100 * time.Second,
		IdleTimeout:  10 * time.Second,
		MaxReplySize: 100000,
	}
}

type Server struct {
	config     *Config
	maps       map[string]LookupFn
	mapsLock   sync.RWMutex
	defaultMap DefaultLookupFn

	mu         sync.Mutex
	listeners  map[net.Listener]struct{}
	activeConn map[net.Conn]struct{}
	closed     bool
	doneChan   chan struct{}
}

// NewServer creates a new Server with the provided configuration.
// If config is nil, DefaultConfig() is used.
func NewServer(config *Config) *Server {
	if config == nil {
		config = DefaultConfig()
	}

	return &Server{
		config:     config,
		maps:       make(map[string]LookupFn),
		listeners:  make(map[net.Listener]struct{}),
		activeConn: make(map[net.Conn]struct{}),
		doneChan:   make(chan struct{}),
	}
}

// RegisterMap registers a lookup function for the given map name.
func (sm *Server) RegisterMap(name string, fn LookupFn) {
	sm.mapsLock.Lock()
	sm.maps[name] = fn
	sm.mapsLock.Unlock()
}

// RegisterDefaultMap sets a fallback lookup function for unknown maps.
func (sm *Server) RegisterDefaultMap(fn DefaultLookupFn) {
	sm.mapsLock.Lock()
	sm.defaultMap = fn
	sm.mapsLock.Unlock()
}

// ListenAndServe listens on the given TCP address and serves requests.
// The context controls the lifetime of the server.
func (sm *Server) ListenAndServe(ctx context.Context, addr string) error {
	if addr == "" {
		addr = fmt.Sprintf(":%d", sm.config.Port)
	}

	var lc net.ListenConfig

	l, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	return sm.Serve(ctx, l)
}

// ListenAndServeUnix listens on the given Unix socket path and serves requests.
// The context controls the lifetime of the server.
func (sm *Server) ListenAndServeUnix(ctx context.Context, path string) error {
	var lc net.ListenConfig

	l, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return err
	}

	return sm.Serve(ctx, l)
}

// Serve accepts connections on the listener and handles them.
// The context controls the lifetime of the server.
func (sm *Server) Serve(ctx context.Context, listener net.Listener) error {
	sm.mu.Lock()
	if sm.closed {
		sm.mu.Unlock()
		return ErrServerClosed
	}
	sm.listeners[listener] = struct{}{}
	sm.mu.Unlock()

	defer func() {
		sm.mu.Lock()
		delete(sm.listeners, listener)
		sm.mu.Unlock()
		listener.Close()
	}()

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			sm.mu.Lock()
			closed := sm.closed
			sm.mu.Unlock()

			if closed {
				return ErrServerClosed
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			return err
		}

		sm.trackConn(conn, true)
		go func() {
			sm.handle(ctx, conn)
			sm.trackConn(conn, false)
		}()
	}
}

// Shutdown gracefully shuts down the server without interrupting active connections.
// It closes all listeners and waits for all connections to finish.
func (sm *Server) Shutdown(ctx context.Context) error {
	sm.mu.Lock()
	sm.closed = true
	for l := range sm.listeners {
		l.Close()
	}
	sm.mu.Unlock()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		sm.mu.Lock()
		count := len(sm.activeConn)
		sm.mu.Unlock()

		if count == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Close immediately closes all active connections and listeners.
func (sm *Server) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.closed = true

	for l := range sm.listeners {
		l.Close()
	}

	for conn := range sm.activeConn {
		conn.Close()
	}

	return nil
}

func (sm *Server) trackConn(conn net.Conn, add bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if add {
		sm.activeConn[conn] = struct{}{}
	} else {
		delete(sm.activeConn, conn)
	}
}
