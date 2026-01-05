package socketmap

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/nettest"
)

var testmapCom = map[string]string{
	"foo@example.com": "bar@example.com",
	"baz@example.com": "cux@example.com",
}

func fnCom(_ context.Context, key string) (*Result, error) {
	v, exists := testmapCom[key]
	if exists {
		return ReplyOK(v), nil
	}
	return ReplyNotFound(), nil
}

func runTestServer(t *testing.T, opts ...func(*Server)) (*Server, net.Conn) {
	t.Helper()

	server := NewServer(nil)
	server.RegisterMap("com", fnCom)

	for _, opt := range opts {
		opt(server)
	}

	listener, err := nettest.NewLocalListener("tcp")
	require.NoError(t, err)

	go server.Serve(context.Background(), listener)

	addr := listener.Addr()
	conn, err := net.Dial(addr.Network(), addr.String())
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close()
		server.Close()
	})

	return server, conn
}

func sendRequest(t *testing.T, conn net.Conn, bin []byte) []byte {
	t.Helper()

	_, err := conn.Write(bin)
	require.NoError(t, err)

	ns := NetstringForReading()
	_, err = ns.ReadFrom(conn)
	require.NoError(t, err)

	result, err := ns.Marshal()
	require.NoError(t, err)

	return result
}

func TestServer(t *testing.T) {
	t.Run("single request ok", func(t *testing.T) {
		_, conn := runTestServer(t)

		bin := []byte("19:com foo@example.com,")
		expected := []byte("18:OK bar@example.com,")
		result := sendRequest(t, conn, bin)

		assert.Equal(t, expected, result)
	})

	t.Run("single request not found", func(t *testing.T) {
		_, conn := runTestServer(t)

		bin := []byte("19:com fax@example.com,")
		expected := []byte("9:NOTFOUND ,")
		result := sendRequest(t, conn, bin)

		assert.Equal(t, expected, result)
	})

	t.Run("multiple requests on same connection", func(t *testing.T) {
		_, conn := runTestServer(t)

		tests := []struct {
			request  []byte
			expected []byte
		}{
			{[]byte("19:com foo@example.com,"), []byte("18:OK bar@example.com,")},
			{[]byte("19:com baz@example.com,"), []byte("18:OK cux@example.com,")},
		}

		for _, tc := range tests {
			result := sendRequest(t, conn, tc.request)
			assert.Equal(t, tc.expected, result)
		}
	})

	t.Run("unknown map returns temp fail", func(t *testing.T) {
		_, conn := runTestServer(t)

		bin := []byte("19:net foo@example.com,")
		expected := []byte("34:TEMP the lookup map does not exist,")
		result := sendRequest(t, conn, bin)

		assert.Equal(t, expected, result)
	})

	t.Run("default map handler", func(t *testing.T) {
		_, conn := runTestServer(t, func(s *Server) {
			s.RegisterDefaultMap(func(_ context.Context, dict, key string) (*Result, error) {
				return ReplyOK("default:" + dict + ":" + key), nil
			})
		})

		bin := []byte("19:net foo@example.com,")
		expected := []byte("30:OK default:net:foo@example.com,")
		result := sendRequest(t, conn, bin)

		assert.Equal(t, expected, result)
	})
}

func TestServerShutdown(t *testing.T) {
	t.Run("graceful shutdown", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("com", fnCom)

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errChan := make(chan error, 1)
		go func() {
			errChan <- server.Serve(ctx, listener)
		}()

		time.Sleep(10 * time.Millisecond)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()

		err = server.Shutdown(shutdownCtx)
		require.NoError(t, err)
	})

	t.Run("close immediately", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("com", fnCom)

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		time.Sleep(10 * time.Millisecond)

		err = server.Close()
		require.NoError(t, err)
	})
}

func TestServerWithConfig(t *testing.T) {
	t.Run("custom config", func(t *testing.T) {
		config := &Config{
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
			IdleTimeout:  10 * time.Second,
			MaxReplySize: 50000,
		}

		server := NewServer(config)
		assert.Equal(t, config.ReadTimeout, server.config.ReadTimeout)
		assert.Equal(t, config.WriteTimeout, server.config.WriteTimeout)
		assert.Equal(t, config.IdleTimeout, server.config.IdleTimeout)
		assert.Equal(t, config.MaxReplySize, server.config.MaxReplySize)
	})

	t.Run("default config matches postfix defaults", func(t *testing.T) {
		config := DefaultConfig()
		assert.Equal(t, 27823, config.Port)
		assert.Equal(t, 100*time.Second, config.ReadTimeout)
		assert.Equal(t, 100*time.Second, config.WriteTimeout)
		assert.Equal(t, 10*time.Second, config.IdleTimeout)
		assert.Equal(t, 100000, config.MaxReplySize)
	})
}

func TestErrServerClosed(t *testing.T) {
	server := NewServer(nil)

	listener, err := nettest.NewLocalListener("tcp")
	require.NoError(t, err)

	go server.Serve(context.Background(), listener)

	time.Sleep(10 * time.Millisecond)

	server.Close()

	listener2, err := nettest.NewLocalListener("tcp")
	require.NoError(t, err)

	err = server.Serve(context.Background(), listener2)
	assert.Equal(t, ErrServerClosed, err)
}

func TestMaxReplySize(t *testing.T) {
	tests := []struct {
		name      string
		maxSize   int
		replyData string
		request   []byte
		expected  []byte
	}{
		{
			name:      "reply within limit succeeds",
			maxSize:   1000,
			replyData: "short",
			request:   []byte("6:test x,"),
			expected:  []byte("8:OK short,"),
		},
		{
			name:      "zero max reply size means no limit",
			maxSize:   0,
			replyData: "test",
			request:   []byte("6:test x,"),
			expected:  []byte("7:OK test,"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			replyData := tc.replyData
			server := NewServer(&Config{MaxReplySize: tc.maxSize})
			server.RegisterMap("test", func(_ context.Context, _ string) (*Result, error) {
				return ReplyOK(replyData), nil
			})

			listener, err := nettest.NewLocalListener("tcp")
			require.NoError(t, err)

			go server.Serve(context.Background(), listener)

			conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
			require.NoError(t, err)

			t.Cleanup(func() {
				conn.Close()
				server.Close()
			})

			result := sendRequest(t, conn, tc.request)
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("reply exceeding limit closes connection", func(t *testing.T) {
		server := NewServer(&Config{MaxReplySize: 10})
		server.RegisterMap("test", func(_ context.Context, _ string) (*Result, error) {
			return ReplyOK("this is a very long reply that exceeds the limit"), nil
		})

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		t.Cleanup(func() {
			conn.Close()
			server.Close()
		})

		_, err = conn.Write([]byte("6:test x,"))
		require.NoError(t, err)

		ns := NetstringForReading()
		_, err = ns.ReadFrom(conn)
		assert.Error(t, err)
	})
}

func TestListenAndServe(t *testing.T) {
	t.Run("with default address", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		ctx, cancel := context.WithCancel(context.Background())

		errChan := make(chan error, 1)
		go func() {
			errChan <- server.ListenAndServe(ctx, "")
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case err := <-errChan:
			assert.True(t, err == context.Canceled || err == nil || errors.Is(err, net.ErrClosed))
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for server to stop")
		}
	})

	t.Run("with custom address", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		ctx, cancel := context.WithCancel(context.Background())

		errChan := make(chan error, 1)
		go func() {
			errChan <- server.ListenAndServe(ctx, "127.0.0.1:0")
		}()

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case err := <-errChan:
			assert.True(t, err == context.Canceled || err == nil || errors.Is(err, net.ErrClosed))
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for server to stop")
		}
	})

	t.Run("invalid address returns error", func(t *testing.T) {
		server := NewServer(nil)
		err := server.ListenAndServe(context.Background(), "invalid:address:format:99999")
		assert.Error(t, err)
	})
}

func TestListenAndServeUnix(t *testing.T) {
	t.Run("unix socket", func(t *testing.T) {
		// Use /tmp directly to avoid macOS 104-byte Unix socket path limit
		socketPath := fmt.Sprintf("/tmp/socketmap_test_%d.sock", time.Now().UnixNano())
		t.Cleanup(func() {
			_ = os.Remove(socketPath)
		})

		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		ctx, cancel := context.WithCancel(context.Background())

		errChan := make(chan error, 1)
		go func() {
			errChan <- server.ListenAndServeUnix(ctx, socketPath)
		}()

		time.Sleep(50 * time.Millisecond)

		conn, err := net.Dial("unix", socketPath)
		require.NoError(t, err)

		result := sendRequest(t, conn, []byte("20:test foo@example.com,"))
		assert.Equal(t, []byte("18:OK bar@example.com,"), result)

		conn.Close()
		cancel()

		select {
		case err := <-errChan:
			assert.True(t, err == context.Canceled || err == nil || errors.Is(err, net.ErrClosed))
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for server to stop")
		}
	})

	t.Run("invalid unix path returns error", func(t *testing.T) {
		server := NewServer(nil)
		err := server.ListenAndServeUnix(context.Background(), "/nonexistent/path/test.sock")
		assert.Error(t, err)
	})
}

func TestServeContextCancellation(t *testing.T) {
	server := NewServer(nil)
	server.RegisterMap("test", fnCom)

	listener, err := nettest.NewLocalListener("tcp")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Serve(ctx, listener)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-errChan:
		assert.True(t, err == context.Canceled || errors.Is(err, net.ErrClosed))
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for server to stop")
	}
}

func TestShutdownTimeout(t *testing.T) {
	server := NewServer(nil)
	server.RegisterMap("test", func(_ context.Context, _ string) (*Result, error) {
		time.Sleep(5 * time.Second)
		return ReplyOK("slow"), nil
	})

	listener, err := nettest.NewLocalListener("tcp")
	require.NoError(t, err)

	go server.Serve(context.Background(), listener)

	conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
	require.NoError(t, err)

	go func() {
		conn.Write([]byte("6:test x,"))
	}()

	time.Sleep(50 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = server.Shutdown(shutdownCtx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	conn.Close()
	server.Close()
}

func TestLookupError(t *testing.T) {
	t.Run("registered map returns error", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", func(_ context.Context, _ string) (*Result, error) {
			return nil, errors.New("lookup failed")
		})

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		t.Cleanup(func() {
			conn.Close()
			server.Close()
		})

		result := sendRequest(t, conn, []byte("6:test x,"))
		assert.Equal(t, []byte("18:TEMP lookup failed,"), result)
	})

	t.Run("default map returns error", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterDefaultMap(func(_ context.Context, _, _ string) (*Result, error) {
			return nil, errors.New("default lookup failed")
		})

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		t.Cleanup(func() {
			conn.Close()
			server.Close()
		})

		result := sendRequest(t, conn, []byte("6:test x,"))
		assert.Equal(t, []byte("26:TEMP default lookup failed,"), result)
	})
}

func TestConnectionClose(t *testing.T) {
	t.Run("client closes connection", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		conn.Close()

		time.Sleep(50 * time.Millisecond)

		server.Close()
	})

	t.Run("malformed request closes connection", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		t.Cleanup(func() {
			conn.Close()
			server.Close()
		})

		_, err = conn.Write([]byte("7:testmap,"))
		require.NoError(t, err)

		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, err = conn.Read(buf)
		assert.True(t, err == io.EOF || errors.Is(err, net.ErrClosed) || isTimeoutError(err))
	})

	t.Run("invalid netstring closes connection", func(t *testing.T) {
		config := &Config{
			ReadTimeout: 50 * time.Millisecond,
		}
		server := NewServer(config)
		server.RegisterMap("test", fnCom)

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		t.Cleanup(func() {
			conn.Close()
			server.Close()
		})

		_, err = conn.Write([]byte("invalid netstring data"))
		require.NoError(t, err)

		buf := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, err = conn.Read(buf)
		assert.Error(t, err)
	})
}

func TestTimeouts(t *testing.T) {
	t.Run("idle timeout closes connection", func(t *testing.T) {
		config := &Config{
			IdleTimeout: 50 * time.Millisecond,
		}
		server := NewServer(config)
		server.RegisterMap("test", fnCom)

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		t.Cleanup(func() {
			conn.Close()
			server.Close()
		})

		time.Sleep(100 * time.Millisecond)

		_, err = conn.Write([]byte("6:test x,"))
		if err == nil {
			buf := make([]byte, 1024)
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			_, err = conn.Read(buf)
		}
		assert.True(t, err != nil)
	})

	t.Run("zero timeouts work correctly", func(t *testing.T) {
		config := &Config{
			ReadTimeout:  0,
			WriteTimeout: 0,
			IdleTimeout:  0,
		}
		server := NewServer(config)
		server.RegisterMap("test", fnCom)

		listener, err := nettest.NewLocalListener("tcp")
		require.NoError(t, err)

		go server.Serve(context.Background(), listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		require.NoError(t, err)

		t.Cleanup(func() {
			conn.Close()
			server.Close()
		})

		result := sendRequest(t, conn, []byte("20:test foo@example.com,"))
		assert.Equal(t, []byte("18:OK bar@example.com,"), result)
	})
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// Benchmarks

func BenchmarkServer(b *testing.B) {
	server := NewServer(&Config{})
	server.RegisterMap("test", func(_ context.Context, key string) (*Result, error) {
		return ReplyOK(key), nil
	})

	listener, err := nettest.NewLocalListener("tcp")
	if err != nil {
		b.Fatal(err)
	}

	go server.Serve(context.Background(), listener)

	conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() {
		conn.Close()
		server.Close()
	})

	request := []byte("20:test foo@example.com,")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := conn.Write(request)
		if err != nil {
			b.Fatal(err)
		}

		ns := NetstringForReading()
		if _, err := ns.ReadFrom(conn); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRequestDecode(b *testing.B) {
	data := []byte("23:testmap foo@example.com,")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := NetstringForReading()
		if _, err := ns.ReadFrom(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}

		r := &Request{}
		if err := r.Decode(ns); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResultEncode(b *testing.B) {
	result := ReplyOK("bar@example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := result.Encode().Marshal()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// mockConn implements net.Conn for testing error paths
type mockConn struct {
	readData         []byte
	readPos          int
	readErr          error
	writeErr         error
	setDeadlineErr   error
	closeErr         error
	writtenData      []byte
	localAddr        net.Addr
	remoteAddr       net.Addr
	failOnDeadline   bool
	deadlineFailType string
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n = copy(b, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writtenData = append(m.writtenData, b...)
	return len(b), nil
}

func (m *mockConn) Close() error {
	return m.closeErr
}

func (m *mockConn) LocalAddr() net.Addr {
	if m.localAddr != nil {
		return m.localAddr
	}
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}
}

func (m *mockConn) RemoteAddr() net.Addr {
	if m.remoteAddr != nil {
		return m.remoteAddr
	}
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321}
}

func (m *mockConn) SetDeadline(_ time.Time) error {
	if m.failOnDeadline {
		return m.setDeadlineErr
	}
	return nil
}

func (m *mockConn) SetReadDeadline(_ time.Time) error {
	if m.failOnDeadline && (m.deadlineFailType == "" || m.deadlineFailType == "read") {
		return m.setDeadlineErr
	}
	return nil
}

func (m *mockConn) SetWriteDeadline(_ time.Time) error {
	if m.failOnDeadline && (m.deadlineFailType == "" || m.deadlineFailType == "write") {
		return m.setDeadlineErr
	}
	return nil
}

// mockConnEOFAfterFirst returns data for first request, then EOF
type mockConnEOFAfterFirst struct {
	firstRequest []byte
	readPos      int
	firstDone    bool
	writtenData  []byte
}

func (m *mockConnEOFAfterFirst) Read(b []byte) (n int, err error) {
	if m.firstDone {
		return 0, io.EOF
	}
	if m.readPos >= len(m.firstRequest) {
		m.firstDone = true
		return 0, io.EOF
	}
	n = copy(b, m.firstRequest[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConnEOFAfterFirst) Write(b []byte) (n int, err error) {
	m.writtenData = append(m.writtenData, b...)
	return len(b), nil
}

func (m *mockConnEOFAfterFirst) Close() error                       { return nil }
func (m *mockConnEOFAfterFirst) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockConnEOFAfterFirst) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockConnEOFAfterFirst) SetDeadline(_ time.Time) error      { return nil }
func (m *mockConnEOFAfterFirst) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockConnEOFAfterFirst) SetWriteDeadline(_ time.Time) error { return nil }

func TestHandleDeadlineErrors(t *testing.T) {
	t.Run("SetReadDeadline error in idle timeout", func(t *testing.T) {
		server := NewServer(&Config{
			IdleTimeout: time.Second,
		})
		server.RegisterMap("test", fnCom)

		conn := &mockConn{
			readData:         []byte("20:test foo@example.com,"),
			failOnDeadline:   true,
			setDeadlineErr:   errors.New("deadline error"),
			deadlineFailType: "read",
		}

		err := server.handle(context.Background(), conn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "deadline")
	})

	t.Run("SetWriteDeadline error", func(t *testing.T) {
		server := NewServer(&Config{
			WriteTimeout: time.Second,
		})
		server.RegisterMap("test", fnCom)

		conn := &mockConn{
			readData:         []byte("20:test foo@example.com,"),
			failOnDeadline:   true,
			setDeadlineErr:   errors.New("write deadline error"),
			deadlineFailType: "write",
		}

		err := server.handle(context.Background(), conn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "deadline")
	})

	t.Run("SetReadDeadline error in netstringRead", func(t *testing.T) {
		server := NewServer(&Config{
			ReadTimeout: time.Second,
			IdleTimeout: 0,
		})
		server.RegisterMap("test", fnCom)

		conn := &mockConn{
			readData:         []byte("20:test foo@example.com,"),
			failOnDeadline:   true,
			setDeadlineErr:   errors.New("read deadline error"),
			deadlineFailType: "read",
		}

		err := server.handle(context.Background(), conn)
		assert.Error(t, err)
	})

	t.Run("write error", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		conn := &mockConn{
			readData: []byte("20:test foo@example.com,"),
			writeErr: errors.New("write error"),
		}

		err := server.handle(context.Background(), conn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write error")
	})

	t.Run("read error non-EOF", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		conn := &mockConn{
			readErr: errors.New("network error"),
		}

		err := server.handle(context.Background(), conn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("EOF on second read", func(t *testing.T) {
		server := NewServer(nil)
		server.RegisterMap("test", fnCom)

		// First request succeeds, then EOF on second read
		conn := &mockConnEOFAfterFirst{
			firstRequest: []byte("20:test foo@example.com,"),
		}

		err := server.handle(context.Background(), conn)
		assert.ErrorIs(t, err, io.EOF)
	})
}

func FuzzServerRequest(f *testing.F) {
	f.Add([]byte("4:test key,"))
	f.Add([]byte("19:com foo@example.com,"))
	f.Add([]byte("7:testmap,"))
	f.Add([]byte("0:,"))
	f.Add([]byte("invalid"))
	f.Add([]byte("100:short,"))

	f.Fuzz(func(t *testing.T, data []byte) {
		server := NewServer(&Config{
			ReadTimeout:  100 * time.Millisecond,
			WriteTimeout: 100 * time.Millisecond,
			IdleTimeout:  100 * time.Millisecond,
		})
		server.RegisterMap("test", func(_ context.Context, key string) (*Result, error) {
			return ReplyOK(key), nil
		})
		server.RegisterMap("com", func(_ context.Context, key string) (*Result, error) {
			return ReplyOK(key), nil
		})

		listener, err := nettest.NewLocalListener("tcp")
		if err != nil {
			t.Skip("cannot create listener")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		go server.Serve(ctx, listener)

		conn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
		if err != nil {
			t.Skip("cannot connect")
		}
		defer conn.Close()

		conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		_, _ = conn.Write(data)

		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		buf := make([]byte, 4096)
		_, _ = conn.Read(buf)

		server.Close()
	})
}
