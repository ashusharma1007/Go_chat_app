package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

//The CheckOrigin function allows WebSocket connections from any origin, but in a production application, you should implement security checks.

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Message struct {
	Username string `json:"username"`
	Message  string `json:"message"`
}

var clients = make(map[*websocket.Conn]bool)
var broadcast = make(chan Message)

var messageHistory = []Message{}

const maxMessageHistory = 50

func main() {
	http.HandleFunc("/", homePage)
	http.HandleFunc("/ws", handleConnections)

	go handleMessages()

	fmt.Println("Server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic("Error starting server: " + err.Error())
	}
}

func homePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}
func handleConnections(w http.ResponseWriter, r *http.Request) {
	// We upgrade the HTTP connection to a WebSocket connection using the upgrader
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	clients[conn] = true

	for _, msg := range messageHistory {
		err := conn.WriteJSON(msg)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			fmt.Println(err)
			delete(clients, conn)
			return
		}

		broadcast <- msg
	}
}

// This function listens for messages on the broadcast channel and sends them to all connected clients.
// If there’s an error sending a message to a client, we close their connection and remove them from the clients map.
func handleMessages() {

	for {
		msg := <-broadcast

		messageHistory = append(messageHistory, msg)
		if len(messageHistory) > maxMessageHistory {
			messageHistory = messageHistory[1:]
		}

		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				fmt.Println(err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}
