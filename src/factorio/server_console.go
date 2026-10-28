package factorio

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/api/websocket"
	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/rcon"
)

var publishProcessLog = func(text string) {
	websocket.WebsocketHub.GetRoom("gamelog").Send(text)
}

func (server *Server) setRconIfRunning(console *rcon.RemoteConsole) bool {
	serverRCONCommandMutex.Lock()
	defer serverRCONCommandMutex.Unlock()
	stateMutex.Lock()
	if !server.Running {
		stateMutex.Unlock()
		return false
	}
	previous := server.Rcon
	server.Rcon = console
	stateMutex.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return true
}

func (server *Server) getRcon() *rcon.RemoteConsole {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return server.Rcon
}

func (server *Server) closeRcon() {
	serverRCONCommandMutex.Lock()
	defer serverRCONCommandMutex.Unlock()
	stateMutex.Lock()
	console := server.Rcon
	server.Rcon = nil
	stateMutex.Unlock()
	if console == nil {
		return
	}
	if err := console.Close(); err != nil {
		log.Printf("Error closing rcon connection: %s", err)
	}
}

func (server *Server) parseRunningCommand(std io.ReadCloser) error {
	reader := bufio.NewReaderSize(std, 16*1024)
	for {
		line, readErr := readBoundedLogLine(reader, 64*1024)
		if line != "" || readErr == nil {
			text := redactLogLine(line)
			log.Printf("Factorio Server: %s", text)
			if err := server.writeLog(text); err != nil {
				log.Printf("Write Factorio console log: %v", err)
			}
			if !isManagerRCONConnectionLog(text) {
				// The room drops slow clients; one ordered send avoids a goroutine per line.
				publishProcessLog(text)
			}
			if strings.Contains(text, "Starting RCON interface at IP") {
				if err := connectRC(); err != nil {
					log.Printf("Connect Factorio RCON: %v", err)
				}
			}
			server.checkProcessHealth(text)
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			log.Printf("Read Factorio process output: %v", readErr)
			return readErr
		}
	}
}

func (server *Server) writeLog(logline string) error {
	file, err := os.OpenFile(bootstrap.GetConfig().ConsoleLogFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open console log: %w", err)
	}
	_, writeErr := file.WriteString(logline + "\n")
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func init() {
	websocket.WebsocketHub.RegisterControlHandler <- serverWebsocketControl
}

// react to websocket control messages and run the command if it is requested
func serverWebsocketControl(controls websocket.WsControls) {
	if controls.Type == "command" {
		command := controls.Value
		server := GetFactorioServer()
		if server.GetRunning() {
			response, err := server.runPersistentRCONCommand(command)
			if err != nil {
				log.Printf("Error running RCON command: %s", err)
				return
			}
			if response != "" {
				websocket.WebsocketHub.GetRoom("gamelog").Send(redactLogLine(response))
			}
		}
	}
}
