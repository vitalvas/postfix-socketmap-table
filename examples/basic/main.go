package main

import (
	"context"
	"log"
	"slices"

	socketmap "github.com/vitalvas/postfix-socketmap-table"
)

func main() {
	server := socketmap.NewServer(nil)

	server.RegisterMap("virtual_mailbox_domains", lookupVirtualMailboxDomains)

	if err := server.ListenAndServe(context.Background(), "127.0.0.1:27823"); err != nil {
		log.Fatal(err)
	}
}

var testDomains = []string{
	"example.com",
	"example.net",
}

func lookupVirtualMailboxDomains(_ context.Context, key string) (*socketmap.Result, error) {
	if slices.Contains(testDomains, key) {
		return socketmap.ReplyOK(key), nil
	}

	return socketmap.ReplyNotFound(), nil
}
