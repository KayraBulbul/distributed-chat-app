package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReadPumpUnregistersOnDisconnect(t *testing.T) {
	hub := &Hub{unregister: make(chan *Client, 1)}
	done := make(chan any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { done <- recover() }()
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		client := &Client{hub: hub, conn: conn}
		// No messages are sent, so a disconnect must not reach the DB or Redis.
		client.readPump(nil, nil)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	select {
	case panicValue := <-done:
		if panicValue != nil {
			t.Fatalf("readPump panicked on disconnect: %v", panicValue)
		}
	case <-time.After(time.Second):
		t.Fatal("readPump did not return after disconnect")
	}
	select {
	case <-hub.unregister:
	default:
		t.Fatal("disconnected client was not unregistered")
	}
}
