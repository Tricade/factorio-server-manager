package factorio

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"

	"github.com/OpenFactorioServerManager/rcon"
)

var serverRCONCommandMutex sync.Mutex

func localRCONAddress() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(bootstrap.GetConfig().FactorioRconPort))
}

func connectRC() error {
	config := bootstrap.GetConfig()
	rconAddr := localRCONAddress()
	server := GetFactorioServer()
	console, err := rcon.Dial(rconAddr, config.FactorioRconPass)
	if err != nil {
		log.Printf("Cannot create rcon session: %s", err)
		return err
	}
	pendingStop, installed := server.installStartupRCON(console)
	if !installed {
		_ = console.Close()
		return nil
	}
	log.Printf("rcon session established on %s", rconAddr)
	if pendingStop {
		log.Printf("Factorio startup completed with a shutdown request pending")
		runCleanStopCheckpoint()
		return server.signalStop()
	}
	startCheckpointMonitor(server)

	return nil
}

// installStartupRCON publishes the usable console before readiness can claim
// a queued stop. Clean-stop checkpointing therefore never observes the
// impossible state "startup ready, RCON nil".
func (server *Server) installStartupRCON(console *rcon.RemoteConsole) (pendingStop bool, installed bool) {
	if !server.setRconIfRunning(console) {
		return false, false
	}
	return server.markStartupReady(), true
}

func (server *Server) runPersistentRCONCommand(command string) (string, error) {
	serverRCONCommandMutex.Lock()
	defer serverRCONCommandMutex.Unlock()

	console := server.getRcon()
	if console == nil {
		return "", errors.New("Factorio RCON is not ready")
	}
	requestID, err := console.Write(command)
	if err != nil {
		return "", fmt.Errorf("send RCON command: %w", err)
	}
	response, responseID, err := console.Read()
	if err != nil {
		return "", fmt.Errorf("read RCON response: %w", err)
	}
	if responseID != requestID {
		return "", fmt.Errorf("unexpected RCON response id %d for request %d", responseID, requestID)
	}
	return strings.TrimRight(response, "\r\n"), nil
}
