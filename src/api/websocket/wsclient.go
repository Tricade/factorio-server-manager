package websocket

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Timeout for sending a message
	writeWait = 10 * time.Second

	// Timeout between the answer of two pong messages
	pongWait = 60 * time.Second

	// Period in which a new ping message is sent. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size sent from a client
	maxMessageSize = 2048
)

// The upgrader to upgrade from http to ws protocol
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// The websocket client, that is the middleman between a websocket connection and the hub.
// It manages every communication between the hub and the websocket connection.
type wsClient struct {
	// The hub this client is registered to.
	hub *wsHub

	// The websocket connection.
	conn *websocket.Conn

	// channel to send messages to the websocket connection.
	send chan wsMessage

	// Authorization is re-evaluated for every RCON command so deleting or
	// downgrading an account (or changing its password) revokes an existing
	// socket without waiting for disconnect.
	allowCommands func() bool
}

// read messages from the websocket connection, choose what has to be done with it and execute that action
// messages with room name, send to the room
// messages with empty room name and controls set, will execute that control, nothing will be sent to other clients
// messages with empty room name and empty controls will send the message to all clients registered in the hub.
//
// This pump has to be executed in a goroutine!
func (client *wsClient) readPump() {
	// When this pump closes, unregister the client and close the websocket connection
	defer func() {
		client.hub.unregister <- client
		client.conn.Close()
	}()

	// Setup some websocket connection settings
	client.conn.SetReadLimit(maxMessageSize)
	client.conn.SetReadDeadline(time.Now().Add(pongWait))
	client.conn.SetPongHandler(func(string) error {
		client.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		// wait and read the next incoming message on the websocket.
		var message wsMessage
		err := client.conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		if message.RoomName == "" {
			// controls messages will not sent to other clients, they only are relevant for the server
			if message.Controls != (WsControls{}) {
				// this message is a control message, do its job!
				switch message.Controls.Type {
				case "subscribe":
					if !client.maySubscribeRoom(message.Controls.Value) {
						continue
					}
					room := client.hub.GetRoom(message.Controls.Value)
					room.register <- client
				case "unsubscribe":
					if !clientRoomAllowed(message.Controls.Value) {
						continue
					}
					room := client.hub.GetRoom(message.Controls.Value)
					room.unregister <- client
				case "command":
					if client.mayDispatchControl(message.Controls) {
						client.hub.dispatchControl(message.Controls)
					}
				}
			}
		}
	}
}

func (client *wsClient) mayDispatchControl(controls WsControls) bool {
	return controls.Type != "command" || (client.allowCommands != nil && client.allowCommands())
}

func clientRoomAllowed(name string) bool {
	return name == "gamelog" || name == "server_status"
}

// Server status is operational read-only data. The live game log also carries
// RCON responses, so it must use the same dynamically re-evaluated
// administrator capability as command dispatch.
func (client *wsClient) maySubscribeRoom(name string) bool {
	if !clientRoomAllowed(name) {
		return false
	}
	if name == "server_status" {
		return true
	}
	return client.allowCommands != nil && client.allowCommands()
}

// write message to the websocket connection.
// messages from client.send channel are sent
// Also starts a timer ticker to send ping messages
//
// This pump has to be executed in a goroutine!
func (client *wsClient) writePump() {
	// setup ping message ticker
	ticker := time.NewTicker(pingPeriod)

	// stop the ticker and close the websocket connection, when this pump is finished
	defer func() {
		ticker.Stop()
		client.conn.Close()
	}()

	for {

		select {
		case message, ok := <-client.send:
			// Setup timeout
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// The hub closed the channel. Therefore notify the client.
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// send the message as json
			err := client.conn.WriteJSON(message)
			if err != nil {
				return
			}
		case <-ticker.C:
			// Setup timeout
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))

			// send a ping message
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWs is the http handler to upgrade from http to ws..
// Also the startup point for a client
func ServeWs(w http.ResponseWriter, r *http.Request, allowCommands func() bool) {
	// upgrade the connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// setup the client
	client := &wsClient{
		hub:           WebsocketHub,
		conn:          conn,
		send:          make(chan wsMessage, 256),
		allowCommands: allowCommands,
	}

	// register this client in the hub
	client.hub.register <- client

	// start the pipes for the new client in goroutines
	go client.writePump()
	go client.readPump()
}
