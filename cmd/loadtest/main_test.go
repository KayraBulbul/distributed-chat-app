package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func TestUsersSendReconnectAndStop(t *testing.T) {
	var mu sync.Mutex
	users := make(map[string]string)
	connections := make(map[string]int)
	messages := make(map[string]int)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "http://localhost:5173"
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			var user struct {
				Name string `json:"username"`
			}
			if err := json.NewDecoder(r.Body).Decode(&user); err != nil || user.Name == "" {
				http.Error(w, "bad username", http.StatusBadRequest)
				return
			}
			id := uuid.NewString()
			mu.Lock()
			users[id] = user.Name
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{"user_id": id})
		case "/echo":
			id := r.URL.Query().Get("user_id")
			mu.Lock()
			_, exists := users[id]
			mu.Unlock()
			if !exists {
				http.Error(w, "unknown user", http.StatusNotFound)
				return
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			mu.Lock()
			connections[id]++
			firstConnection := connections[id] == 1
			mu.Unlock()
			if err := conn.WriteJSON(map[string]string{"type": "server_info", "server": "test"}); err != nil {
				return
			}
			for {
				kind, body, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if kind != websocket.TextMessage || len(body) == 0 {
					t.Error("expected a nonempty text message")
					return
				}
				mu.Lock()
				messages[id]++
				mu.Unlock()
				if err := conn.WriteJSON(map[string]string{"message_id": uuid.NewString()}); err != nil {
					return
				}
				if firstConnection {
					return // Simulate a server dropping each user's first connection.
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	opts := options{baseURL: server.URL, origin: "http://localhost:5173", users: 3, interval: 10 * time.Millisecond}
	var totals stats
	done := make(chan struct{})
	go func() {
		run(ctx, opts, &totals)
		close(done)
	}()
	for totals.reconnects.Load() < 3 || totals.received.Load() < 6 {
		if !pause(ctx, 10*time.Millisecond) {
			t.Fatal("users did not reconnect and receive messages before timeout")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tester did not stop promptly")
	}
	if totals.connected.Load() != 0 || totals.created.Load() != 3 {
		t.Fatalf("created=%d connected=%d", totals.created.Load(), totals.connected.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(users) != 3 {
		t.Fatalf("reconnects must reuse users, got %d users", len(users))
	}
	for id := range users {
		if connections[id] < 2 || messages[id] < 1 {
			t.Errorf("user %s: connections=%d messages=%d", id, connections[id], messages[id])
		}
	}
}

func TestCreateUserFailure(t *testing.T) {
	for _, body := range []string{`{"user_id":"invalid"}`, `{broken`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(body))
			}))
			defer server.Close()
			if _, err := createUser(context.Background(), server.Client(), server.URL, "test"); err == nil {
				t.Fatal("expected invalid user response to fail")
			}
		})
	}
}

func TestValidateOptions(t *testing.T) {
	valid := options{baseURL: "http://localhost:8880", users: 1, interval: time.Second}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*options){
		func(o *options) { o.users = 0 },
		func(o *options) { o.interval = 0 },
		func(o *options) { o.duration = -time.Second },
		func(o *options) { o.ramp = -time.Second },
		func(o *options) { o.baseURL = "ws://localhost:8880" },
		func(o *options) { o.baseURL = "http://localhost:8880/echo" },
	} {
		opts := valid
		mutate(&opts)
		if err := opts.validate(); err == nil {
			t.Fatalf("expected invalid options to fail: %+v", opts)
		}
	}
}
