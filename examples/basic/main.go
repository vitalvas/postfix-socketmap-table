package main

import (
	"context"

	socketmap "github.com/vitalvas/postfix-socketmap-table"
	"golang.org/x/exp/slices"
)

func main() {
	server := socketmap.NewServer()

	server.RegisterMap("virtual_mailbox_domains", lookupVirtualMailboxDomains)

}

var testDomains = []string{
	"example.com",
	"example.net",
}

func lookupVirtualMailboxDomains(ctx context.Context, key string) (*socketmap.Result, error) {
	if slices.Contains(testDomains, key) {
		return socketmap.ReplyOK(key), nil
	}

	return socketmap.ReplyNotFound(), nil
}
