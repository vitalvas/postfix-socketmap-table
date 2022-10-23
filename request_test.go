package socketmap

import (
	"bytes"
	"testing"

	"github.com/seandlg/netstring"
)

func TestRequestDecode(t *testing.T) {
	b := []byte("23:testmap foo@example.com,")
	ns := netstring.ForReading()
	ns.ReadFrom(bytes.NewReader(b))

	r := &Request{}
	if err := r.Decode(ns); err != nil {
		t.Errorf("Failed to decode: %s", err)
	}
	if r.Name != "testmap" {
		t.Errorf("Got unexpected map name: %s", r.Name)
	}
	if r.Key != "foo@example.com" {
		t.Errorf("Got unexpected lookup key: %s", r.Key)
	}
}
