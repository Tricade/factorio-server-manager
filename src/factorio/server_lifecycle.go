package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type serverStartSnapshot struct {
	BindIP       string
	Port         int
	Savefile     string
	Version      Version
	SettingsJSON []byte
}

// serverBeforeProcessStart is a narrow test seam used to hold the claimed
// starting state before any mutable process setup occurs.
var serverBeforeProcessStart = func(serverStartSnapshot) {}

func (server *Server) autostart() {
	err := server.Run()
	if err != nil {
		log.Printf("Error starting Factorio server: %+v", err)
	}
}

// StartConfiguredAutostart starts the active setup only after all persistent
// manager state has been initialized. Keeping this separate from
// NewFactorioServer prevents the autostart goroutine from racing the first
// profile migration during process startup.
func StartConfiguredAutostart() {
	settings, err := LoadAutostartSettings()
	if err != nil {
		log.Printf("Factorio autostart is disabled because its persisted setting could not be loaded: %v", err)
		return
	}
	if settings.Enabled {
		go instantiated.autostart()
	}
}

type requestedServerStart struct {
	BindIP   string
	Port     int
	Savefile string
}

// Start claims the stopped server synchronously and launches exactly the
// supplied request. A second caller therefore cannot overwrite the first
// caller's bind/save fields before its goroutine constructs the command.
func (server *Server) Start(bindIP string, port int, savefile string) (<-chan error, error) {
	request := &requestedServerStart{BindIP: bindIP, Port: port, Savefile: savefile}
	return server.start(request)
}

// Run retains the historical blocking API for internal autostart and tests.
// Its configuration is captured under the same atomic start claim.
func (server *Server) Run() error {
	result, err := server.start(nil)
	if err != nil {
		return err
	}
	return <-result
}

func (server *Server) start(request *requestedServerStart) (<-chan error, error) {
	// Keep active profile directories stable until the child process has been
	// started (or startup has failed). Profile activation needs the write side.
	profileDataGate.RLock()
	serverLifecycleMutex.Lock()

	stateMutex.RLock()
	busy := server.Running || server.Stopping || server.starting
	stateMutex.RUnlock()
	if busy {
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		return nil, ErrServerActive
	}
	if !worldGenerationLock.TryLock() {
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		return nil, ErrWorldGenerationBusy
	}

	stateMutex.Lock()
	// Recheck after claiming the world-generation exclusion in case process
	// state changed independently while the lock was acquired.
	if server.Running || server.Stopping || server.starting {
		stateMutex.Unlock()
		worldGenerationLock.Unlock()
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		return nil, ErrServerActive
	}

	bindIP := server.BindIP
	port := server.Port
	savefile := server.Savefile
	if request != nil {
		bindIP = request.BindIP
		port = request.Port
		savefile = request.Savefile
	}
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	if port == 0 {
		port = 34197
	}
	if savefile == "" {
		savefile = "Load Latest"
	}
	settingsData, err := json.MarshalIndent(server.Settings, "", "  ")
	if err != nil {
		stateMutex.Unlock()
		worldGenerationLock.Unlock()
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		return nil, fmt.Errorf("marshal Factorio server settings: %w", err)
	}

	server.BindIP = bindIP
	server.Port = port
	server.Savefile = savefile
	server.Stopping = false
	server.startupReady = false
	server.stopSignaled = false
	server.starting = true
	snapshot := serverStartSnapshot{
		BindIP:       bindIP,
		Port:         port,
		Savefile:     savefile,
		Version:      server.Version,
		SettingsJSON: append([]byte(nil), settingsData...),
	}
	stateMutex.Unlock()
	serverLifecycleMutex.Unlock()

	result := make(chan error, 1)
	var releaseOnce sync.Once
	releaseStartupGuards := func() {
		releaseOnce.Do(func() {
			worldGenerationLock.Unlock()
			profileDataGate.RUnlock()
		})
	}
	go func() {
		defer close(result)
		defer releaseStartupGuards()
		serverBeforeProcessStart(snapshot)
		result <- server.runStartSnapshot(snapshot, releaseStartupGuards)
	}()
	return result, nil
}

func (server *Server) runStartSnapshot(snapshot serverStartSnapshot, releaseStartupGuards func()) (err error) {
	publishedRunning := false
	defer func() {
		if publishedRunning {
			return
		}
		stateMutex.Lock()
		server.starting = false
		server.Stopping = false
		server.startupReady = false
		server.stopSignaled = false
		stateMutex.Unlock()
		server.broadcastStatus()
	}()

	config := bootstrap.GetConfig()
	if err := writeFileAtomically(config.SettingsFile, snapshot.SettingsJSON, 0644); err != nil {
		return fmt.Errorf("save startup server settings: %w", err)
	}

	saves, err := ListSaves()
	if err != nil {
		return fmt.Errorf("list saves before start: %w", err)
	}
	if len(saves) == 0 {
		return errors.New("no savefile exists on the server")
	}

	args := []string{}
	// The Factorio server references its executable path. When invoking a
	// custom loader, pass the real binary explicitly so Factorio resolves data.
	if config.GlibcCustom == "true" {
		log.Println("Custom glibc selected, glibc.so location:", config.GlibcLocation, " lib location:", config.GlibcLibLoc)
		args = append(args, "--library-path", config.GlibcLibLoc, config.FactorioBinary, "--executable-path", config.FactorioBinary)
	}

	args = append(args,
		"--bind", snapshot.BindIP,
		"--port", strconv.Itoa(snapshot.Port),
		"--server-settings", config.SettingsFile,
		"--rcon-bind", "127.0.0.1:"+strconv.Itoa(config.FactorioRconPort),
		"--rcon-password", config.FactorioRconPass)
	if snapshot.Version.Greater(Version{0, 17, 0}) {
		args = append(args, "--server-adminlist", config.FactorioAdminFile)
	}

	if strings.HasPrefix(snapshot.Savefile, "Load Latest") {
		latestSave, latestErr := GetLatestSave()
		if latestErr != nil {
			return fmt.Errorf("find latest save: %w", latestErr)
		}
		args = append(args, "--start-server", filepath.Join(config.FactorioSavesDir, latestSave.Name))
	} else {
		if err := ValidatePathElement(snapshot.Savefile); err != nil {
			return fmt.Errorf("invalid save name: %w", err)
		}
		args = append(args, "--start-server", filepath.Join(config.FactorioSavesDir, snapshot.Savefile))
	}
	if config.ChatLogFile != "" {
		args = append(args, "--console-log", config.ChatLogFile)
	}

	var command *exec.Cmd
	if config.GlibcCustom == "true" {
		log.Printf("Starting Factorio server with custom glibc at %s", config.GlibcLocation)
		command = exec.Command(config.GlibcLocation, args...)
	} else {
		log.Printf("Starting Factorio server binary: %s", config.FactorioBinary)
		command = exec.Command(config.FactorioBinary, args...)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open Factorio stdout: %w", err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("open Factorio stdin: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return fmt.Errorf("open Factorio stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		log.Printf("Factorio process failed to start: %s", err)
		return err
	}

	stateMutex.Lock()
	server.Cmd = command
	server.StdOut = stdout
	server.StdIn = stdin
	server.StdErr = stderr
	server.Running = true
	server.starting = false
	stateMutex.Unlock()
	publishedRunning = true
	server.broadcastStatus()
	go server.parseRunningCommand(stdout)
	go server.parseRunningCommand(stderr)
	releaseStartupGuards()

	err = command.Wait()
	log.Printf("Factorio process is closed")
	server.closeRcon()
	server.SetRunning(false)
	if err != nil {
		log.Printf("Factorio process exited with error: %s", err)
		return err
	}
	return nil
}
