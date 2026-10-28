//go:build !windows

package factorio

import (
	"errors"
	"fmt"
	"log"
	"os"
)

func (server *Server) signalStop() error {
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	stateMutex.RLock()
	var process *os.Process
	if server.Cmd != nil {
		process = server.Cmd.Process
	}
	stateMutex.RUnlock()
	if process == nil {
		server.SetStopping(false)
		return errProcessUnavailable
	}
	if err := process.Signal(os.Interrupt); err != nil {
		server.SetStopping(false)
		if errors.Is(err, os.ErrProcessDone) {
			server.SetRunning(false)
		}
		return fmt.Errorf("send shutdown signal to Factorio process: %w", err)
	}
	log.Print("Sent SIGINT to Factorio process; waiting for clean shutdown")
	server.closeRcon()
	return nil
}

func (server *Server) checkProcessHealth(string) {}
