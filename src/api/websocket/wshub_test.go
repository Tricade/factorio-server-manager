package websocket

import (
	"sync"
	"testing"
	"time"
)

func TestGetRoomIsSafeAndIdempotentAcrossGoroutines(t *testing.T) {
	hub := &wsHub{rooms: make(map[string]*wsRoom)}
	const workers = 32
	rooms := make(chan *wsRoom, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			rooms <- hub.GetRoom("gamelog")
		}()
	}
	wait.Wait()
	close(rooms)
	var first *wsRoom
	for room := range rooms {
		if first == nil {
			first = room
			continue
		}
		if room != first {
			t.Fatal("concurrent GetRoom calls created more than one room")
		}
	}
}

func TestSlowRoomClientDoesNotDeadlockRoom(t *testing.T) {
	hub := &wsHub{rooms: make(map[string]*wsRoom)}
	room := hub.GetRoom("server_status")
	client := &wsClient{send: make(chan wsMessage)}
	room.register <- client

	done := make(chan struct{})
	go func() {
		room.Send("first")
		room.Send("second")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("room blocked while removing a client whose send queue was full")
	}
}

func TestSlowGamelogCacheReplayDoesNotDeadlockRoom(t *testing.T) {
	previousCache := LogCache
	LogCache = []string{"cached one", "cached two"}
	t.Cleanup(func() { LogCache = previousCache })

	hub := &wsHub{rooms: make(map[string]*wsRoom)}
	room := hub.GetRoom("gamelog")
	client := &wsClient{send: make(chan wsMessage), allowCommands: func() bool { return true }}
	room.register <- client

	done := make(chan struct{})
	go func() {
		room.unregister <- client
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cached log replay blocked the room on a slow client")
	}
}

func TestClientRoomAllowlist(t *testing.T) {
	for _, allowed := range []string{"gamelog", "server_status"} {
		if !clientRoomAllowed(allowed) {
			t.Fatalf("expected %q to be allowed", allowed)
		}
	}
	for _, rejected := range []string{"", "arbitrary", "../gamelog"} {
		if clientRoomAllowed(rejected) {
			t.Fatalf("expected %q to be rejected", rejected)
		}
	}
}

func TestGamelogRoomRequiresCurrentAdministratorCapability(t *testing.T) {
	administrator := false
	client := &wsClient{allowCommands: func() bool { return administrator }}
	if client.maySubscribeRoom("gamelog") {
		t.Fatal("viewer websocket was allowed to subscribe to live console output")
	}
	if !client.maySubscribeRoom("server_status") {
		t.Fatal("viewer websocket could not subscribe to read-only server status")
	}
	administrator = true
	if !client.maySubscribeRoom("gamelog") {
		t.Fatal("administrator websocket could not subscribe to live console output")
	}
	administrator = false
	if client.maySubscribeRoom("gamelog") {
		t.Fatal("existing websocket retained live console access after authorization was revoked")
	}
}

func TestGamelogRoomStopsSendingAfterAuthorizationRevocation(t *testing.T) {
	hub := &wsHub{rooms: make(map[string]*wsRoom)}
	room := hub.GetRoom("gamelog")
	administrator := true
	client := &wsClient{
		send:          make(chan wsMessage, 1),
		allowCommands: func() bool { return administrator },
	}
	room.register <- client
	room.Send("authorized output")
	select {
	case <-client.send:
	case <-time.After(time.Second):
		t.Fatal("administrator did not receive live console output before revocation")
	}
	administrator = false
	room.Send("sensitive command response")
	select {
	case message := <-client.send:
		t.Fatalf("revoked websocket received live console output: %#v", message)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRCONControlRequiresAdministratorCapability(t *testing.T) {
	command := WsControls{Type: "command", Value: "/players online"}
	allowed := false
	client := &wsClient{allowCommands: func() bool { return allowed }}
	if client.mayDispatchControl(command) {
		t.Fatal("read-only websocket client was allowed to dispatch an RCON command")
	}
	allowed = true
	if !client.mayDispatchControl(command) {
		t.Fatal("administrator websocket client could not dispatch an RCON command")
	}
	allowed = false
	if client.mayDispatchControl(command) {
		t.Fatal("existing websocket retained RCON access after authorization was revoked")
	}
}
