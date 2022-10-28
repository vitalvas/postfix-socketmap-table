package socketmap

import (
	"context"
	"fmt"
	"net"
	"sync"
)

const defaultPort int = 27823

type Server struct {
	maps       map[string]LookupFn
	mapsLock   sync.RWMutex
	defaultMap DefaultLookupFn
}

func NewServer() *Server {
	return &Server{
		maps: make(map[string]LookupFn),
	}
}

func (sm *Server) RegisterMap(name string, fn LookupFn) {
	sm.maps[name] = fn
}

func (sm *Server) RegisterDefaultMap(fn DefaultLookupFn) {
	sm.defaultMap = fn
}

func (sm *Server) ListenAndServe(addr string) error {
	return sm.ListenAndServeContext(context.Background(), addr)
}

func (sm *Server) ListenAndServeContext(ctx context.Context, addr string) error {
	if addr == "" {
		addr = fmt.Sprintf(":%d", defaultPort)
	}

	var lc net.ListenConfig

	l, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	return sm.Serve(ctx, l)
}

func (sm *Server) Serve(ctx context.Context, listener net.Listener) error {
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		go sm.handle(ctx, conn)
	}
}
