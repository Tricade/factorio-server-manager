package factorio

import (
	"errors"
	"fmt"
	"log"
	"os"
)

var errProcessUnavailable = errors.New("the Factorio process is not available yet")

// Kill is idempotent for an already stopped runtime. An unpublished startup
// cannot be force-killed: report that state rather than dereferencing Cmd or
// releasing the reservation while startup still owns it.
func (server *Server) Kill() error {
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	stateMutex.Lock()
	if !server.Running && !server.starting {
		stateMutex.Unlock()
		return nil
	}
	if server.Cmd == nil || server.Cmd.Process == nil {
		stateMutex.Unlock()
		return errProcessUnavailable
	}
	process := server.Cmd.Process
	server.Stopping = true
	stateMutex.Unlock()
	server.broadcastStatus()
	stopCheckpointMonitor()
	if err := process.Kill(); err != nil {
		server.SetStopping(false)
		if errors.Is(err, os.ErrProcessDone) {
			server.SetRunning(false)
		}
		return fmt.Errorf("force-stop Factorio process: %w", err)
	}
	server.closeRcon()
	log.Print("Sent force-stop to Factorio process")
	return nil
}

func (server *Server) Stop() error {
	if !server.requestStop() {
		return nil
	}
	if !server.claimStopSignalIfReady() {
		log.Print("Factorio is still starting; safe shutdown is queued until startup completes")
		return nil
	}
	stopCheckpointMonitor()
	runCleanStopCheckpoint()
	return server.signalStop()
}
