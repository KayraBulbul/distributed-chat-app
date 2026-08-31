package server

import (
	"flag"
	"html/template"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var addr *string = flag.String("addr", "localhost:8080", "http service address")

var upgrader websocket.Upgrader = websocket.Upgrader{}

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

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.hub.broadcast <- message
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

func echo(h *Hub) http.Handler {
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

		go client.writePump()
		client.readPump()
	})
}

func home(w http.ResponseWriter, r *http.Request) {
	err := homeTemplate.Execute(w, "ws://"+r.Host+"/echo")
	if err != nil {
		log.Println("template:", err)
		return
	}
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	hub := Hub{
		clients:   make(map[*Client]bool),
		register:  make(chan *Client),
		broadcast: make(chan []byte),
	}

	go hub.run()

	http.Handle("/echo", echo(&hub))
	http.HandleFunc("/", home)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

var homeTemplate = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<script>  
window.addEventListener("load", function(evt) {

    var output = document.getElementById("output");
    var input = document.getElementById("input");
    var ws;

    var print = function(message) {
        var d = document.createElement("div");
        d.textContent = message;
        output.appendChild(d);
        output.scroll(0, output.scrollHeight);
    };

    document.getElementById("open").onclick = function(evt) {
        if (ws) {
            return false;
        }
        ws = new WebSocket("{{.}}");
        ws.onopen = function(evt) {
            print("OPEN");
        }
        ws.onclose = function(evt) {
            print("CLOSE");
            ws = null;
        }
        ws.onmessage = function(evt) {
            print("RESPONSE: " + evt.data);
        }
        ws.onerror = function(evt) {
            print("ERROR: " + evt.data);
        }
        return false;
    };

    document.getElementById("send").onclick = function(evt) {
        if (!ws) {
            return false;
        }
        print("SEND: " + input.value);
        ws.send(input.value);
        return false;
    };

    document.getElementById("close").onclick = function(evt) {
        if (!ws) {
            return false;
        }
        ws.close();
        return false;
    };

});
</script>
</head>
<body>
<table>
<tr><td valign="top" width="50%">
<p>Click "Open" to create a connection to the server, 
"Send" to send a message to the server and "Close" to close the connection. 
You can change the message and send multiple times.
<p>
<form>
<button id="open">Open</button>
<button id="close">Close</button>
<p><input id="input" type="text" value="Hello world!">
<button id="send">Send</button>
</form>
</td><td valign="top" width="50%">
<div id="output" style="max-height: 70vh;overflow-y: scroll;"></div>
</td></tr></table>
</body>
</html>
`))
