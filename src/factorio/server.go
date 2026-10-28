package factorio

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os/exec"
	"sync"

	"github.com/OpenFactorioServerManager/factorio-server-manager/api/websocket"
	"github.com/OpenFactorioServerManager/rcon"
)

type Server struct {
	Cmd            *exec.Cmd              `json:"-"`
	Savefile       string                 `json:"savefile"`
	Latency        int                    `json:"latency"`
	BindIP         string                 `json:"bindip"`
	Port           int                    `json:"port"`
	Running        bool                   `json:"running"`
	Stopping       bool                   `json:"stopping"`
	Version        Version                `json:"fac_version"`
	BaseModVersion string                 `json:"base_mod_version"`
	StdOut         io.ReadCloser          `json:"-"`
	StdErr         io.ReadCloser          `json:"-"`
	StdIn          io.WriteCloser         `json:"-"`
	Settings       map[string]interface{} `json:"-"`
	Rcon           *rcon.RemoteConsole    `json:"-"`
	LogChan        chan []string          `json:"-"`
	startupReady   bool
	stopSignaled   bool
	starting       bool
}

type ServerStatus struct {
	Savefile       string  `json:"savefile"`
	Latency        int     `json:"latency"`
	BindIP         string  `json:"bindip"`
	Port           int     `json:"port"`
	Running        bool    `json:"running"`
	Stopping       bool    `json:"stopping"`
	Version        Version `json:"fac_version"`
	BaseModVersion string  `json:"base_mod_version"`
}

var instantiated Server

var stateMutex sync.RWMutex

var serverLifecycleMutex sync.Mutex

var ErrServerActive = errors.New("the Factorio server is already running, starting or stopping")

func (server *Server) SetRunning(newState bool) {
	stopped := false
	stateMutex.Lock()
	if server.Running == newState {
		stateMutex.Unlock()
		return
	}
	server.Running = newState
	server.starting = false
	if !newState {
		stopped = true
		server.Stopping = false
		server.startupReady = false
		server.stopSignaled = false
	}
	stateMutex.Unlock()
	if stopped {
		stopCheckpointMonitor()
	}
	server.broadcastStatus()
}

func (server *Server) SetStopping(stopping bool) {
	stateMutex.Lock()
	if server.Stopping == stopping {
		stateMutex.Unlock()
		return
	}
	server.Stopping = stopping
	if !stopping {
		server.stopSignaled = false
	}
	stateMutex.Unlock()
	server.broadcastStatus()
}

// A stop can race process exit, or arrive while startup owns its reservation.
// Queue it only for a live/start-pending runtime, never a fully stopped one.
func (server *Server) requestStop() bool {
	stateMutex.Lock()
	if !server.Running && !server.starting {
		stateMutex.Unlock()
		return false
	}
	server.Stopping = true
	stateMutex.Unlock()
	server.broadcastStatus()
	return true
}

func (server *Server) broadcastStatus() {
	wsRoom := websocket.WebsocketHub.GetRoom("server_status")
	response, err := json.Marshal(server.Snapshot())
	if err != nil {
		log.Printf("Error encoding server status: %s", err)
		return
	}
	wsRoom.Send(string(response))
}

func (server *Server) GetRunning() bool {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return server.Running
}

func (server *Server) IsBusy() bool {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return server.Running || server.Stopping || server.starting
}

func (server *Server) IsStopping() bool {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return server.Stopping
}

func (server *Server) isStartupReady() bool {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return server.Running && server.startupReady
}

func (server *Server) Snapshot() ServerStatus {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return ServerStatus{
		Savefile:       server.Savefile,
		Latency:        server.Latency,
		BindIP:         server.BindIP,
		Port:           server.Port,
		Running:        server.Running,
		Stopping:       server.Stopping,
		Version:        server.Version,
		BaseModVersion: server.BaseModVersion,
	}
}

func (server *Server) ConfigureStart(bindIP string, port int, savefile string) error {
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	stateMutex.Lock()
	defer stateMutex.Unlock()
	if server.Running || server.starting {
		return ErrServerActive
	}
	server.BindIP = bindIP
	server.Port = port
	server.Savefile = savefile
	server.Stopping = false
	server.startupReady = false
	server.stopSignaled = false
	return nil
}

func (server *Server) claimStopSignalIfReady() bool {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	if !server.Running || !server.Stopping || !server.startupReady || server.stopSignaled {
		return false
	}
	server.stopSignaled = true
	return true
}

func (server *Server) markStartupReady() bool {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	// Factorio can write its RCON-ready line immediately after Cmd.Start, before
	// Run has published Running=true. Keep the readiness marker even in that
	// narrow window so a following stop request cannot be queued forever.
	server.startupReady = true
	if !server.Running || !server.Stopping || server.stopSignaled {
		return false
	}
	server.stopSignaled = true
	return true
}

func SetFactorioServer(server Server) {
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	stateMutex.Lock()
	defer stateMutex.Unlock()
	instantiated = server
}

func GetFactorioServer() (f *Server) {
	return &instantiated
}
