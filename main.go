package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Msg struct {
	Username string    `json:"username"`
	Content  string    `json:"content"`
	Time     time.Time `json:"time"`
	UserList []string  `json:"user_list"`
	IsSystem bool      `json:"is_system"`
}

type Client struct {
	Username string
	Conn     *websocket.Conn
	Send     chan Msg
}

type Hub struct {
	Clients    map[*Client]bool
	BroadCast  chan Msg
	Register   chan *Client
	Unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		BroadCast:  make(chan Msg),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			log.Printf("Clients %s connected. Total Clients %d", client.Username, len(h.Clients))

			welcomeMsg := Msg{
				Username: "System",
				Content:  client.Username + "joined the chat",
				Time:     time.Now(),
				IsSystem: true,
				UserList: h.GetUserNames(),
			}
			h.BroadCast <- welcomeMsg

		case client := <-h.Unregister:
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				log.Printf("Client %s disconnected. Totla Clients %d", client.Username, len(h.Clients))
			}
			goodbyeMsg := Msg{
				Username: "System",
				Content:  client.Username + "left the chat",
				Time:     time.Now(),
				IsSystem: true,
				UserList: h.GetUserNames(),
			}
			h.BroadCast <- goodbyeMsg

		case message := <-h.BroadCast:
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
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
		errors.New("username not found")
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error %s", err)
		return
	}

	client := &Client{
		Username: username,
		Conn:     conn,
		Send:     make(chan Msg, 256),
	}

	hub.Register <- client

	go client.readMessages(hub)
	go client.writeMessages()
}

func (c *Client) readMessages(hub *Hub) {
	defer func() {
		hub.Unregister <- c
		c.Conn.Close()
	}()

	for {
		var msg Msg
		err := c.Conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Read error: %v", err)
			break
		}
		msg.Username = c.Username
		msg.Time = time.Now()

		hub.BroadCast <- msg
	}
}

func (c *Client) writeMessages() {
	defer c.Conn.Close()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteJSON(message); err != nil {
				log.Printf("Write error: %v", err)
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
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { handleWebSocket(hub, w, r) })
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("/"))))

	log.Println("Chat server starting on :8080")
	log.Println("Visit http://localhost:8080 to use the chat")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

//The CheckOrigin function allows WebSocket connections from any origin, but in a production application, you should implement security checks.

// var jwtKey = []byte("your_secret_key")

// type User struct {
// 	Username string `json:"username"`
// 	Password string `json:"password"`
// }

// var users = make(map[string]string)

// type Claims struct {
// 	Username string `json:"username"`
// 	jwt.StandardClaims
// }

// // WebSocket upgrader
// var upgrader = websocket.Upgrader{
// 	CheckOrigin: func(r *http.Request) bool {
// 		return true
// 	},
// }

// type Message struct {
// 	Username string `json:"username"`
// 	Message  string `json:"message"`
// }

// var clients = make(map[*websocket.Conn]string)
// var broadcast = make(chan Message)

// var messageHistory = []Message{}

// const maxMessageHistory = 50

// func main() {
// 	http.HandleFunc("/", homePage)
// 	http.HandleFunc("/register", registerHandler)
// 	http.HandleFunc("/login", loginHandler)
// 	http.HandleFunc("/ws", authenticateWS(handleConnections))

// 	go handleMessages()

// 	fmt.Println("Server started on :8080")
// 	err := http.ListenAndServe(":8080", nil)
// 	if err != nil {
// 		panic("Error starting server: " + err.Error())
// 	}
// }

// func homePage(w http.ResponseWriter, r *http.Request) {
// 	http.ServeFile(w, r, "index.html")
// }

// func loginHandler(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != "POST" {
// 		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
// 		return
// 	}

// 	var user User
// 	err := json.NewDecoder(r.Body).Decode(&user)
// 	if err != nil {
// 		http.Error(w, "Invalid request body", http.StatusBadRequest)
// 		return
// 	}

// 	storedPasssword, exists := users[user.Username]
// 	if !exists {
// 		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
// 		return
// 	}

// 	err = bcrypt.CompareHashAndPassword([]byte(storedPasssword), []byte(user.Password))
// 	if err != nil {
// 		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
// 		return
// 	}

// 	expirationTime := time.Now().Add(24 * time.Hour)
// 	claims := &Claims{
// 		Username: user.Username,
// 		StandardClaims: jwt.StandardClaims{
// 			ExpiresAt: expirationTime.Unix(),
// 		},
// 	}

// 	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
// 	tokenString, err := token.SignedString(jwtKey)
// 	if err != nil {
// 		http.Error(w, "Server error", http.StatusInternalServerError)
// 		return
// 	}

// 	json.NewEncoder(w).Encode(map[string]string{"token": tokenString})

// }

// func registerHandler(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != "POST" {
// 		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
// 		return
// 	}

// 	var user User
// 	err := json.NewDecoder(r.Body).Decode(&user)
// 	if err != nil {
// 		http.Error(w, "Username already exist", http.StatusConflict)
// 		return
// 	}

// 	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
// 	if err != nil {
// 		http.Error(w, "Server error", http.StatusInternalServerError)
// 		return
// 	}

// 	users[user.Username] = string(hashedPassword)

// 	w.WriteHeader(http.StatusCreated)
// 	json.NewEncoder(w).Encode(map[string]string{"message": "User registered successfull"})

// }

// func authenticateWS(next http.HandlerFunc) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		// Get token from URL query parameter
// 		tokenString := r.URL.Query().Get("token")
// 		if tokenString == "" {
// 			http.Error(w, "Unauthorized", http.StatusUnauthorized)
// 			return
// 		}

// 		// Parse and validate token
// 		claims := &Claims{}
// 		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
// 			return jwtKey, nil
// 		})

// 		if err != nil || !token.Valid {
// 			http.Error(w, "Unauthorized", http.StatusUnauthorized)
// 			return
// 		}

// 		// Store username in request context
// 		ctx := r.Context()
// 		r = r.WithContext(context.WithValue(ctx, "username", claims.Username))

// 		// Call the next handler
// 		next(w, r)
// 	}
// }

// func handleConnections(w http.ResponseWriter, r *http.Request) {

// 	username := r.Context().Value("username").(string)
// 	// We upgrade the HTTP connection to a WebSocket connection using the upgrader
// 	conn, err := upgrader.Upgrade(w, r, nil)
// 	if err != nil {
// 		fmt.Println(err)
// 		return
// 	}
// 	defer conn.Close()

// 	clients[conn] = username
// 	joinMsg := Message{
// 		Username: "System",
// 		Message:  username + " has joined the chat",
// 	}
// 	broadcast <- joinMsg
// 	for _, msg := range messageHistory {
// 		err := conn.WriteJSON(msg)
// 		if err != nil {
// 			fmt.Println(err)
// 			return
// 		}
// 	}

// 	for {
// 		var msg Message
// 		err := conn.ReadJSON(&msg)
// 		if err != nil {
// 			fmt.Println(err)
// 			delete(clients, conn)
// 			return
// 		}
// 		msg.Username = username
// 		broadcast <- msg

// 	}
// }

// // This function listens for messages on the broadcast channel and sends them to all connected clients.
// // If there’s an error sending a message to a client, we close their connection and remove them from the clients map.
// func handleMessages() {

// 	for {
// 		msg := <-broadcast

// 		messageHistory = append(messageHistory, msg)
// 		if len(messageHistory) > maxMessageHistory {
// 			messageHistory = messageHistory[1:]
// 		}

// 		for client := range clients {
// 			err := client.WriteJSON(msg)
// 			if err != nil {
// 				fmt.Println(err)
// 				client.Close()
// 				delete(clients, client)
// 			}
// 		}
// 	}
// }
