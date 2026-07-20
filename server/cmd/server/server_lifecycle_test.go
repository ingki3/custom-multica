package main

import (
	"net"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestServerLifecycleBootIDStablePerProcessInstance(t *testing.T) {
	lifecycle := newServerLifecycle()
	first := lifecycle.BootID
	second := lifecycle.BootID
	if first != second {
		t.Fatalf("boot ID changed within lifecycle: %q != %q", first, second)
	}
	if uuid.Validate(first) != nil {
		t.Fatalf("boot ID is not a UUID: %q", first)
	}
	if other := newServerLifecycle().BootID; other == first {
		t.Fatal("separate process lifecycle should receive a distinct boot ID")
	}
}

func TestBindHTTPServerPublishesOnlyAfterSuccessfulBind(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0"}
	called := false
	listener, err := bindHTTPServer(srv, func(bound net.Listener) {
		called = true
		if bound.Addr() == nil {
			t.Fatal("callback ran before listener had an address")
		}
	})
	if err != nil {
		t.Fatalf("bindHTTPServer: %v", err)
	}
	defer listener.Close()
	if !called {
		t.Fatal("expected ready callback after successful bind")
	}
}

func TestBindHTTPServerDoesNotPublishOnBindFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	called := false
	listener, err := bindHTTPServer(&http.Server{Addr: occupied.Addr().String()}, func(net.Listener) {
		called = true
	})
	if err == nil {
		listener.Close()
		t.Fatal("expected bind failure")
	}
	if called {
		t.Fatal("ready callback ran despite bind failure")
	}
}
