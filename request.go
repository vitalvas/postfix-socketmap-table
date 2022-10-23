package socketmap

import (
	"errors"
	"strings"

	"github.com/seandlg/netstring"
)

var errorMalformedRequest = errors.New("malformed request")

// A request as received from Postfix
type Request struct {
	Name string
	Key  string
}

func (r *Request) Decode(ns *netstring.Netstring) error {
	// Get the original content of the message
	b, err := ns.Bytes()
	if err != nil {
		return err
	}

	// The message needs to contain a map name and the lookup key, separated by a space
	parts := strings.SplitN(string(b), " ", 2)
	if len(parts) != 2 {
		return errorMalformedRequest
	}

	r.Name = parts[0]
	r.Key = parts[1]

	return nil
}
