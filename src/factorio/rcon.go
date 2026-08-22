package factorio

import (
	"log"
	"strconv"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"

	"github.com/OpenFactorioServerManager/rcon"
)

func connectRC() error {
	var err error
	config := bootstrap.GetConfig()
	rconAddr := config.ServerIP + ":" + strconv.Itoa(config.FactorioRconPort)
	server := GetFactorioServer()
	if server.markStartupReady() {
		log.Printf("Factorio startup completed with a shutdown request pending")
		runCleanStopCheckpoint()
		return server.signalStop()
	}
	console, err := rcon.Dial(rconAddr, config.FactorioRconPass)
	if err != nil {
		log.Printf("Cannot create rcon session: %s", err)
		return err
	}
	if !server.setRconIfRunning(console) {
		_ = console.Close()
		return nil
	}
	log.Printf("rcon session established on %s", rconAddr)
	startCheckpointMonitor(server)

	return nil
}
