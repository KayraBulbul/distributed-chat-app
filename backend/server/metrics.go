package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	messagesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_messages_received_total",
		Help: "Total messages successfully read from WebScoket clients.",
	})

	messagesPublished = promauto.NewCounter(prometheus.CounterOpts{
		Name: "chat_messages_published_total",
		Help: "Total messages successfully published to Redis.",
	})

	activeConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "chat_websocket_connections",
		Help: "Current number of connected WebScoket clients.",
	})
)
