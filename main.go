package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type MsgType string

const (
	PublicMessage  MsgType = "public"
	PrivateMessage MsgType = "Private"
	SystemMessage  MsgType = "system"
)

type Msg struct {
	Type     MsgType   `json:"type"`
	Username string    `json:"username"`
	Content  string    `json:"content"`
	Time     time.Time `json:"time"`
	UserList []string  `json:"user_list"`
	IsSystem bool      `json:"is_system"`
	To       string    `json:"to,omitempty"`
	From     string    `json:"from,omitempty"`
}

type Client struct {
	Username string
	Conn     *websocket.Conn
	Send     chan Msg
}

type Hub struct {
	Clients    map[*Client]bool
	BroadCast  chan Msg
	Private    chan Msg
	Register   chan *Client
	Unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		BroadCast:  make(chan Msg, 256), // Buffered channel
		Private:    make(chan Msg, 256),
		Register:   make(chan *Client, 256), // Buffered channel
		Unregister: make(chan *Client, 256), // Buffered channel
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			log.Printf("Client %s connected. Total Clients %d", client.Username, len(h.Clients))

			welcomeMsg := Msg{
				Type:     SystemMessage,
				Username: "System",
				Content:  client.Username + " joined the chat",
				Time:     time.Now(),
				IsSystem: true,
				UserList: h.GetUserNames(),
			}
			h.BroadCast <- welcomeMsg

		case client := <-h.Unregister:
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				log.Printf("Client %s disconnected. Total Clients %d", client.Username, len(h.Clients))

				goodbyeMsg := Msg{
					Type:     SystemMessage,
					Username: "System",
					Content:  client.Username + " left the chat",
					Time:     time.Now(),
					IsSystem: true,
					UserList: h.GetUserNames(),
				}
				h.BroadCast <- goodbyeMsg
			}

		case message := <-h.BroadCast:
			log.Printf("Broadcasting message from %s: %s", message.Username, message.Content)
			// Always update user list for all messages
			message.UserList = h.GetUserNames()

			// Send to ALL connected clients
			for client := range h.Clients {
				select {
				case client.Send <- message:
					log.Printf("Message sent to %s", client.Username)
				default:
					log.Printf("Failed to send to %s, closing connection", client.Username)
					close(client.Send)
					delete(h.Clients, client)
				}
			}

		case privateMsg := <-h.Private:
			log.Printf("Sending private messages from %s to %s", privateMsg.From, privateMsg.To)

			var sender, recipient *Client
			for client := range h.Clients {
				if client.Username == privateMsg.From {
					sender = client
				}
				if client.Username == privateMsg.To {
					recipient = client
				}
			}
			if sender != nil {
				select {
				case sender.Send <- privateMsg:
					log.Printf("Private message sent to sender %s", sender.Username)
				default:
					log.Printf("Failed to send private message to sender %s", sender.Username)
				}
			}

			if recipient != nil {
				select {
				case recipient.Send <- privateMsg:
					log.Printf("Private message sent to recipient %s", recipient.Username)
				default:
					log.Printf("Failed to send private message to recipient %s", recipient.Username)
				}
			} else {
				// Recipient not found, send error message to sender
				if sender != nil {
					errorMsg := Msg{
						Type:     SystemMessage,
						Username: "System",
						Content:  "User '" + privateMsg.To + "' is not online",
						Time:     time.Now(),
						IsSystem: true,
					}
					select {
					case sender.Send <- errorMsg:
					default:
					}
				}
			}
		}
	}
}

func (h *Hub) GetUserNames() []string {
	var usernames []string
	for client := range h.Clients {
		usernames = append(usernames, client.Username)
	}
	return usernames
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		log.Printf("WebSocket connection attempt without username")
		http.Error(w, "Username required", http.StatusBadRequest)
		return
	}

	log.Printf("WebSocket upgrade request from %s", username)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error for %s: %s", username, err)
		return
	}

	log.Printf("WebSocket connection established for %s", username)

	client := &Client{
		Username: username,
		Conn:     conn,
		Send:     make(chan Msg, 256),
	}

	log.Printf("Starting goroutines for %s", username)
	go client.readMessages(hub)
	go client.writeMessages()

	log.Printf("Registering client %s", username)
	// Move registration after starting goroutines to prevent blocking
	hub.Register <- client
}

func (c *Client) readMessages(hub *Hub) {
	defer func() {
		log.Printf("readMessages defer called for %s", c.Username)
		hub.Unregister <- c
		c.Conn.Close()
	}()

	log.Printf("Starting to read messages for %s", c.Username)

	for {
		var msg Msg
		err := c.Conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Read error for %s: %v", c.Username, err)
			break
		}
		log.Printf("Received message from %s: %s", c.Username, msg.Content)
		msg.Username = c.Username
		msg.Time = time.Now()
		if msg.Type == PrivateMessage && msg.To != "" {
			// Private message
			msg.From = c.Username
			log.Printf("Received private message from %s to %s: %s", c.Username, msg.To, msg.Content)
			hub.Private <- msg
		} else {
			// Public message
			msg.Type = PublicMessage
			msg.IsSystem = false
			log.Printf("Received public message from %s: %s", c.Username, msg.Content)
			hub.BroadCast <- msg
		}
	}
}

func (c *Client) writeMessages() {
	defer func() {
		log.Printf("writeMessages defer called for %s", c.Username)
		c.Conn.Close()
	}()

	log.Printf("Starting to write messages for %s", c.Username)

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				log.Printf("Send channel closed for %s", c.Username)
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			log.Printf("Writing message to %s: %s", c.Username, message.Content)
			if err := c.Conn.WriteJSON(message); err != nil {
				log.Printf("Write error for %s: %v", c.Username, err)
				return
			}
		}
	}
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "index.html")
}

func main() {
	hub := NewHub()
	go hub.Run()

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})

	log.Println("Chat server starting on :8080")
	log.Println("Visit http://localhost:8080 to use the chat")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
