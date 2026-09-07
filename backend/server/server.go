package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader websocket.Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "http://localhost:5173"
	},
}

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				client.send <- message
			}
		}
	}
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func (c *Client) readPump(rdb *redis.Client) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if err := rdb.Publish(context.Background(), CHANNEL_NAME, message).Err(); err != nil {
			log.Print("publish:", err)
		}
	}
}

func (c *Client) writePump() {
	for message := range c.send {
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			return
		}
	}
}

func echo(h *Hub, rdb *redis.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()

		client := &Client{
			hub:  h,
			conn: c,
			send: make(chan []byte, 256),
		}

		client.hub.register <- client

		hostname, err := os.Hostname()
		if err != nil {
			log.Print("hostname:", err)
			return
		}

		err = c.WriteJSON(map[string]string{
			"type":   "server_info",
			"server": hostname,
		})
		if err != nil {
			log.Print("server_info:", err)
			return
		}

		go client.writePump()
		client.readPump(rdb)
	})
}

const CHANNEL_NAME = "messages"

func main() {
	flag.Parse()
	log.SetFlags(0)

	hub := Hub{
		clients:   make(map[*Client]bool),
		register:  make(chan *Client),
		broadcast: make(chan []byte),
	}
	fmt.Print("websocket up and running...")

	go hub.run()
	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "",
		DB:       0,
	})
	subscription := rdb.Subscribe(context.Background(), CHANNEL_NAME)
	defer subscription.Close()

	go func() {
		for message := range subscription.Channel() {
			hub.broadcast <- []byte(message.Payload)
		}
	}()

	http.Handle("/echo", echo(&hub, rdb))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
