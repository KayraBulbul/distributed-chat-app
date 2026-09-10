package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/KayraBulbul/distributed-chat-app/backend/config"
	"github.com/KayraBulbul/distributed-chat-app/backend/internal/database"
	"github.com/KayraBulbul/distributed-chat-app/backend/server/handlers"
	"github.com/KayraBulbul/distributed-chat-app/backend/server/middleware"
	"github.com/KayraBulbul/distributed-chat-app/backend/server/response"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
			activeConnections.Set(float64(len(h.clients)))
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				activeConnections.Set(float64(len(h.clients)))
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				client.send <- message
			}
		}
	}
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	userID   uuid.UUID
	username string
}

func (c *Client) readPump(rdb *redis.Client, cfg *config.Config) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			continue
		}
		messagesReceived.Inc()

		params := database.CreateMessageParams{
			UserID: c.userID,
			Body:   string(message),
		}
		msg, err := cfg.Queries.CreateMessage(context.Background(), params)
		if err != nil {
			log.Print("error saving message to db:", err)
			continue
		}

		type jsonMessage struct {
			MessageID uuid.UUID `json:"message_id"`
			Username  string    `json:"username"`
			Body      string    `json:"body"`
		}

		payload, err := json.Marshal(jsonMessage{
			MessageID: msg.MessageID,
			Username:  c.username,
			Body:      msg.Body,
		})
		if err != nil {
			log.Print("encode message:", err)
			continue
		}

		if err := rdb.Publish(context.Background(),
			CHANNEL_NAME,
			payload,
		).Err(); err != nil {
			log.Print("publish:", err)
			continue
		}
		messagesPublished.Inc()
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

func echo(h *Hub, rdb *redis.Client, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := uuid.Parse(r.URL.Query().Get("user_id"))
		if err != nil {
			response.WithError(w, http.StatusBadRequest, "Invalid userID")
			return
		}

		user, err := cfg.Queries.FindUserByID(r.Context(), userID)
		if err != nil {
			response.WithError(w, http.StatusNotFound, "Cannot find user")
			return
		}

		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		defer c.Close()

		client := &Client{
			hub:      h,
			conn:     c,
			send:     make(chan []byte, 256),
			userID:   user.UserID,
			username: user.Username,
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
		client.readPump(rdb, cfg)
	})
}

const CHANNEL_NAME = "messages"

func main() {
	flag.Parse()
	log.SetFlags(0)

	hub := Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		broadcast:  make(chan []byte),
		unregister: make(chan *Client),
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

	cfg, err := config.CreateCfg()
	if err != nil {
		log.Fatal("error creating config")
	}

	http.HandleFunc("/users", handlers.CreateUser(&cfg))
	http.Handle("/echo", echo(&hub, rdb, &cfg))
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", middleware.Cors(http.DefaultServeMux)))
}
