package main

import (
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type serverLifecycle struct {
	BootID    string
	StartedAt time.Time
}

func newServerLifecycle() serverLifecycle {
	return serverLifecycle{BootID: uuid.NewString(), StartedAt: time.Now().UTC()}
}

// bindHTTPServer invokes onBound only after the listener has successfully
// bound. Keeping binding synchronous prevents server.ready from being emitted
// for address-in-use and other startup failures.
func bindHTTPServer(srv *http.Server, onBound func(net.Listener)) (net.Listener, error) {
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return nil, err
	}
	if onBound != nil {
		onBound(listener)
	}
	return listener, nil
}
