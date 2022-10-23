package socketmap

import (
	"context"
	"net"
	"testing"

	"github.com/seandlg/netstring"
	"golang.org/x/net/nettest"
)

var testmapCom = map[string]string{
	"foo@example.com": "bar@example.com",
	"baz@example.com": "cux@example.com",
}

func fnCom(ctx context.Context, key string) (*Result, error) {
	v, exists := testmapCom[key]
	if exists {
		return ReplyOK(v), nil
	}
	return ReplyNotFound(), nil
}

func runTestServer() (net.Conn, error) {
	server := NewServer()

	server.RegisterMap("com", fnCom)

	listener, err := nettest.NewLocalListener("tcp")
	if err != nil {
		return nil, err
	}

	go server.Serve(context.Background(), listener)

	addr := listener.Addr()
	conn, err := net.Dial(addr.Network(), addr.String())
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func sendRequest(conn net.Conn, bin []byte) ([]byte, error) {
	conn.Write(bin)
	ns := netstring.ForReading()
	err := ns.ReadFrom(conn)
	if err != nil {
		return nil, err
	}

	return ns.Marshal()
}

func TestServerOneRequestOk(t *testing.T) {
	conn, err := runTestServer()
	if err != nil {
		t.Errorf("Failed to run test server: %s", err)
		t.FailNow()
	}

	bin := []byte("19:com foo@example.com,")
	be := []byte("18:OK bar@example.com,")
	bg, err := sendRequest(conn, bin)

	if err != nil {
		t.Errorf("Error sending request: %s", err)
		t.FailNow()
	}

	if string(bg) != string(be) {
		t.Errorf("Got unexpected response: %s", string(bg))
	}
}

func TestServerOneRequestNotFound(t *testing.T) {
	conn, err := runTestServer()
	if err != nil {
		t.Errorf("Failed to run test server: %s", err)
		t.FailNow()
	}

	bin := []byte("19:com fax@example.com,")
	be := []byte("9:NOTFOUND ,")
	bg, err := sendRequest(conn, bin)

	if err != nil {
		t.Errorf("Error sending request: %s", err)
		t.FailNow()
	}

	if string(bg) != string(be) {
		t.Errorf("Got unexpected response: %s", string(bg))
	}
}

func TestServerTwoRequestsOk(t *testing.T) {
	conn, err := runTestServer()
	if err != nil {
		t.Errorf("Failed to run test server: %s", err)
		t.FailNow()
	}

	bin := []byte("19:com foo@example.com,")
	be := []byte("18:OK bar@example.com,")
	bg, err := sendRequest(conn, bin)
	if err != nil {
		t.Errorf("Error sending request: %s", err)
		t.FailNow()
	}

	if string(bg) != string(be) {
		t.Errorf("Got unexpected response: %s", string(bg))
	}

	bin = []byte("19:com baz@example.com,")
	be = []byte("18:OK cux@example.com,")
	bg, err = sendRequest(conn, bin)
	if err != nil {
		t.Errorf("Error sending request: %s", err)
		t.FailNow()
	}

	if string(bg) != string(be) {
		t.Errorf("Got unexpected response: %s", string(bg))
	}
}

func TestServerOneRequestUnknownMap(t *testing.T) {
	conn, err := runTestServer()
	if err != nil {
		t.Errorf("Failed to run test server: %s", err)
		t.FailNow()
	}

	bin := []byte("19:net foo@example.com,")
	be := []byte("34:TEMP the lookup map does not exist,")
	bg, err := sendRequest(conn, bin)
	if err != nil {
		t.Errorf("Error sending request: %s", err)
		t.FailNow()
	}

	if string(bg) != string(be) {
		t.Errorf("Got unexpected response: %s", string(bg))
	}
}
