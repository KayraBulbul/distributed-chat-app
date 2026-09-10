package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type options struct {
	baseURL  string
	origin   string
	users    int
	interval time.Duration
	duration time.Duration
	ramp     time.Duration
}

type stats struct {
	created    atomic.Int64
	connected  atomic.Int64
	sent       atomic.Int64
	received   atomic.Int64
	reconnects atomic.Int64
	errors     atomic.Int64
}

func main() {
	var opts options
	flag.StringVar(&opts.baseURL, "url", "http://localhost:8880", "Chat app HTTP URL through Caddy")
	flag.StringVar(&opts.origin, "origin", "http://localhost:5173", "WebSocket Origin allowed by the server")
	flag.IntVar(&opts.users, "users", 10, "Number of simulated users")
	flag.DurationVar(&opts.interval, "interval", 5*time.Second, "Average message interval per user (randomized +/-25%)")
	flag.DurationVar(&opts.duration, "duration", 0, "Total run duration including ramp-up; 0 runs until Ctrl+C")
	flag.DurationVar(&opts.ramp, "ramp", 5*time.Second, "Time over which to start all users; 0 starts everyone immediately")
	flag.Parse()
	if err := opts.validate(); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.duration)
		defer cancel()
	}

	log.Printf("Starting %d users at %s; interval=%s ramp=%s", opts.users, opts.baseURL, opts.interval, opts.ramp)
	log.Print("Creates persistent test users and messages. Press Ctrl+C to stop.")
	var totals stats
	run(ctx, opts, &totals)
	totals.report()
}

func (o options) validate() error {
	u, err := url.Parse(o.baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("-url must be an http:// or https:// URL")
	}
	if u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("-url must be a base URL without a path, query, or fragment")
	}
	if o.users < 1 || o.interval < time.Millisecond || o.duration < 0 || o.ramp < 0 {
		return fmt.Errorf("-users must be positive, -interval at least 1ms, and -duration/-ramp nonnegative")
	}
	return nil
}

func run(ctx context.Context, opts options, totals *stats) {
	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	prefix := "loadtest-" + uuid.NewString()[:8]
	var workers sync.WaitGroup
	started := time.Now()
	for i := 0; i < opts.users; i++ {
		var offset time.Duration
		if opts.users > 1 {
			offset = time.Duration(float64(opts.ramp) * float64(i) / float64(opts.users-1))
		}
		if !pause(ctx, time.Until(started.Add(offset))) {
			break
		}
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			simulateUser(ctx, client, opts, fmt.Sprintf("%s-%d", prefix, index+1), totals)
		}(i)
	}
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			totals.report()
		}
	}
}

func createUser(ctx context.Context, client *http.Client, baseURL, name string) (string, error) {
	endpoint, _ := url.Parse(baseURL)
	endpoint.Path = "/users"
	body, _ := json.Marshal(map[string]string{"username": name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create user: HTTP %d", resp.StatusCode)
	}
	var user struct {
		ID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	if _, err := uuid.Parse(user.ID); err != nil {
		return "", fmt.Errorf("create user: invalid user_id: %w", err)
	}
	return user.ID, nil
}

func simulateUser(ctx context.Context, client *http.Client, opts options, name string, totals *stats) {
	id, err := createUser(ctx, client, opts.baseURL, name)
	if err != nil {
		if ctx.Err() == nil {
			totals.errors.Add(1)
			log.Printf("%s: %v", name, err)
		}
		return
	}
	totals.created.Add(1)
	endpoint, _ := url.Parse(opts.baseURL)
	endpoint.Path = "/echo"
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	endpoint.RawQuery = url.Values{"user_id": {id}}.Encode()
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	header := http.Header{"Origin": {opts.origin}}
	delay := time.Second
	connectedBefore := false
	for ctx.Err() == nil {
		conn, resp, err := dialer.DialContext(ctx, endpoint.String(), header)
		if err != nil {
			if resp != nil {
				resp.Body.Close()
			}
		} else {
			if connectedBefore {
				totals.reconnects.Add(1)
			}
			connectedBefore = true
			totals.connected.Add(1)
			connectionStart := time.Now()
			err = session(ctx, conn, opts.interval, name, totals)
			totals.connected.Add(-1)
			if time.Since(connectionStart) >= 10*time.Second {
				delay = time.Second
			}
		}
		if ctx.Err() != nil {
			return
		}
		totals.errors.Add(1)
		log.Printf("%s: %v; reconnecting", name, err)
		if !pause(ctx, delay+time.Duration(rand.Float64()*float64(time.Second))) {
			return
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func session(ctx context.Context, conn *websocket.Conn, interval time.Duration, name string, totals *stats) error {
	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	readDone := make(chan error, 1)
	go func() {
		for {
			var message struct {
				ID string `json:"message_id"`
			}
			if err := conn.ReadJSON(&message); err != nil {
				readDone <- err
				return
			}
			if message.ID != "" {
				totals.received.Add(1)
			}
		}
	}()
	// Closing the socket unblocks the reader; wait for it before reconnecting.
	readerFinished := false
	defer func() {
		stopClose()
		conn.Close()
		if !readerFinished {
			<-readDone
		}
	}()
	timer := time.NewTimer(time.Duration(rand.Float64() * float64(interval)))
	defer timer.Stop()
	for sequence := 1; ; {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readDone:
			readerFinished = true
			return err
		case <-timer.C:
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return err
			}
			message := fmt.Sprintf("%s message %d at %s", name, sequence, time.Now().UTC().Format(time.RFC3339Nano))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
				return err
			}
			totals.sent.Add(1)
			sequence++
			timer.Reset(time.Duration(float64(interval) * (0.75 + rand.Float64()*0.5)))
		}
	}
}

func pause(ctx context.Context, delay time.Duration) bool {
	if ctx.Err() != nil {
		return false
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *stats) report() {
	log.Printf("created=%d connected=%d sent=%d received=%d reconnects=%d errors=%d",
		s.created.Load(), s.connected.Load(), s.sent.Load(), s.received.Load(), s.reconnects.Load(), s.errors.Load())
}
