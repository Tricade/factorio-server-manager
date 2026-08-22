package factorio

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenFactorioServerManager/factorio-server-manager/api/websocket"
	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
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
var once sync.Once
var stateMutex sync.RWMutex
var factorioVersionPattern = regexp.MustCompile(`Version.*?((\d+\.)?(\d+\.)?(\*|\d+)+)`)

func (server *Server) SetRunning(newState bool) {
	stopped := false
	stateMutex.Lock()
	if server.Running == newState {
		stateMutex.Unlock()
		return
	}
	server.Running = newState
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

func (server *Server) ConfigureStart(bindIP string, port int, savefile string) {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	server.BindIP = bindIP
	server.Port = port
	server.Savefile = savefile
	server.Stopping = false
	server.startupReady = false
	server.stopSignaled = false
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

func (server *Server) setRconIfRunning(console *rcon.RemoteConsole) bool {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	if !server.Running || server.Stopping {
		return false
	}
	server.Rcon = console
	return true
}

func (server *Server) getRcon() *rcon.RemoteConsole {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return server.Rcon
}

func (server *Server) closeRcon() {
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

func (server *Server) autostart() {
	var err error
	if server.BindIP == "" {
		server.BindIP = "0.0.0.0"

	}
	if server.Port == 0 {
		server.Port = 34197
	}
	if server.Savefile == "" {
		server.Savefile = "Load Latest"
	}

	err = server.Run()

	if err != nil {
		log.Printf("Error starting Factorio server: %+v", err)
		return
	}

}

func SetFactorioServer(server Server) {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	instantiated = server
}

func readFactorioVersionMetadata() (Version, string, error) {
	config := bootstrap.GetConfig()
	var output []byte
	var err error
	if config.GlibcCustom == "true" {
		output, err = exec.Command(config.GlibcLocation, "--library-path", config.GlibcLibLoc, config.FactorioBinary, "--version").Output()
	} else {
		output, err = exec.Command(config.FactorioBinary, "--version").Output()
	}
	if err != nil {
		return Version{}, "", fmt.Errorf("run Factorio version command: %w", err)
	}

	found := factorioVersionPattern.FindStringSubmatch(string(output))
	if len(found) < 2 {
		return Version{}, "", fmt.Errorf("parse Factorio version output: version marker missing")
	}
	var version Version
	if err := version.UnmarshalText([]byte(found[1])); err != nil {
		return Version{}, "", fmt.Errorf("parse Factorio version: %w", err)
	}

	baseModInfoFile := filepath.Join(config.FactorioBaseModDir, "info.json")
	baseModInfo, err := os.ReadFile(baseModInfoFile)
	if err != nil {
		return Version{}, "", fmt.Errorf("read base mod info: %w", err)
	}
	var modInfo ModInfo
	if err := json.Unmarshal(baseModInfo, &modInfo); err != nil {
		return Version{}, "", fmt.Errorf("decode base mod info: %w", err)
	}
	return version, modInfo.Version, nil
}

// RefreshFactorioVersion updates only executable metadata. Unlike rebuilding the
// Server singleton it preserves the stopped/running state, selected save and
// settings, so replacing a Factorio release cannot implicitly autostart or reset
// the current server state.
func RefreshFactorioVersion() error {
	version, baseModVersion, err := readFactorioVersionMetadata()
	if err != nil {
		return err
	}
	stateMutex.Lock()
	defer stateMutex.Unlock()
	instantiated.Version = version
	instantiated.BaseModVersion = baseModVersion
	return nil
}

func NewFactorioServer() (err error) {
	server := Server{}
	server.Settings = make(map[string]interface{})
	config := bootstrap.GetConfig()
	if err = os.MkdirAll(config.FactorioConfigDir, 0755); err != nil {
		log.Printf("failed to create config directory: %v", err)
		return
	}

	settingsPath := config.SettingsFile
	var settings *os.File

	if _, err = os.Stat(settingsPath); os.IsNotExist(err) {
		// copy example settings to supplied settings file, if not exists
		log.Printf("Server settings at %s not found, copying example server settings.\n", settingsPath)

		examplePath := filepath.Join(config.FactorioDir, "data", "server-settings.example.json")

		var example *os.File
		example, err = os.Open(examplePath)
		if err != nil {
			log.Printf("failed to open example server settings: %v", err)
			return
		}
		defer example.Close()

		settings, err = os.Create(settingsPath)
		if err != nil {
			log.Printf("failed to create server settings file: %v", err)
			return
		}
		defer settings.Close()

		_, err = io.Copy(settings, example)
		if err != nil {
			log.Printf("failed to copy example server settings: %v", err)
			return
		}

		err = example.Close()
		if err != nil {
			log.Printf("failed to close example server settings: %s", err)
			return
		}
	} else {
		// otherwise, open file normally
		settings, err = os.Open(settingsPath)
		if err != nil {
			log.Printf("failed to open server settings file: %v", err)
			return
		}
		defer settings.Close()
	}

	// before reading reset offset
	if _, err = settings.Seek(0, 0); err != nil {
		log.Printf("error while seeking in settings file: %v", err)
		return
	}

	if err = json.NewDecoder(settings).Decode(&server.Settings); err != nil {
		log.Printf("error reading %s: %v", settingsPath, err)
		return
	}

	log.Printf("Loaded Factorio settings from %s\n", settingsPath)

	server.Version, server.BaseModVersion, err = readFactorioVersionMetadata()
	if err != nil {
		log.Printf("error loading Factorio version metadata: %s", err)
		return
	}

	// load admins from additional file
	if (server.Version.Greater(Version{0, 17, 0})) {
		if _, err = os.Stat(config.FactorioAdminFile); os.IsNotExist(err) {
			//save empty admins-file
			err = ioutil.WriteFile(config.FactorioAdminFile, []byte("[]"), 0664)
			server.Settings["admins"] = make([]string, 0)
		} else {
			var data []byte
			data, err = ioutil.ReadFile(config.FactorioAdminFile)
			if err != nil {
				log.Printf("Error loading FactorioAdminFile: %s", err)
				return
			}

			var jsonData interface{}
			err = json.Unmarshal(data, &jsonData)
			if err != nil {
				log.Printf("Error unmarshalling FactorioAdminFile: %s", err)
				return
			}

			server.Settings["admins"] = jsonData
		}
	}

	SetFactorioServer(server)
	return
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

func GetFactorioServer() (f *Server) {
	return &instantiated
}

// ReloadFactorioServerProfile refreshes the stopped server singleton after a
// profile has replaced the active config directory. It deliberately does not
// start Factorio; selecting a profile and starting its save remain separate
// user actions.
func ReloadFactorioServerProfile(bindIP string, port int, savefile string) error {
	server := GetFactorioServer()
	if server.GetRunning() || server.IsStopping() {
		return errors.New("stop Factorio before reloading a profile")
	}

	config := bootstrap.GetConfig()
	settingsFile, err := os.Open(config.SettingsFile)
	if err != nil {
		return fmt.Errorf("open profile server settings: %w", err)
	}
	defer settingsFile.Close()

	settings := make(map[string]interface{})
	if err := json.NewDecoder(settingsFile).Decode(&settings); err != nil {
		return fmt.Errorf("decode profile server settings: %w", err)
	}

	snapshot := server.Snapshot()
	if snapshot.Version.Greater(Version{0, 17, 0}) {
		adminData, err := os.ReadFile(config.FactorioAdminFile)
		if errors.Is(err, os.ErrNotExist) {
			adminData = []byte("[]")
		} else if err != nil {
			return fmt.Errorf("read profile admin list: %w", err)
		}
		var admins interface{}
		if err := json.Unmarshal(adminData, &admins); err != nil {
			return fmt.Errorf("decode profile admin list: %w", err)
		}
		settings["admins"] = admins
	}

	if bindIP == "" {
		bindIP = config.FactorioIP
	}
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	if port == 0 {
		port = 34197
	}

	stateMutex.Lock()
	server.Settings = settings
	server.BindIP = bindIP
	server.Port = port
	server.Savefile = savefile
	server.Stopping = false
	server.startupReady = false
	server.stopSignaled = false
	stateMutex.Unlock()
	server.broadcastStatus()
	return nil
}

func (server *Server) Run() error {
	if !worldGenerationLock.TryLock() {
		return ErrWorldGenerationBusy
	}
	startupGuardHeld := true
	defer func() {
		if startupGuardHeld {
			worldGenerationLock.Unlock()
		}
	}()
	if server.GetRunning() || server.IsStopping() {
		return errors.New("Factorio server is already running or stopping")
	}

	var err error
	config := bootstrap.GetConfig()
	data, err := json.MarshalIndent(server.Settings, "", "  ")
	if err != nil {
		log.Println("Failed to marshal FactorioServerSettings: ", err)
	} else {
		ioutil.WriteFile(config.SettingsFile, data, 0644)
	}

	saves, err := ListSaves()
	if err != nil {
		log.Println("Failed to get saves list: ", err)
	}

	if len(saves) == 0 {
		return errors.New("No savefile exists on the server")
	}

	args := []string{}

	//The factorio server refenences its executable-path, since we execute the ld.so file and pass the factorio binary as a parameter
	//the game would use the path to the ld.so file as it's executable path and crash, to prevent this the parameter "--executable-path" is added
	if config.GlibcCustom == "true" {
		log.Println("Custom glibc selected, glibc.so location:", config.GlibcLocation, " lib location:", config.GlibcLibLoc)
		args = append(args, "--library-path", config.GlibcLibLoc, config.FactorioBinary, "--executable-path", config.FactorioBinary)
	}

	args = append(args,
		"--bind", server.BindIP,
		"--port", strconv.Itoa(server.Port),
		"--server-settings", config.SettingsFile,
		"--rcon-bind", "127.0.0.1:"+strconv.Itoa(config.FactorioRconPort),
		"--rcon-password", config.FactorioRconPass)

	if (server.Version.Greater(Version{0, 17, 0})) {
		args = append(args, "--server-adminlist", config.FactorioAdminFile)
	}

	if strings.HasPrefix(server.Savefile, "Load Latest") {
		latestSave, latestErr := GetLatestSave()
		if latestErr != nil {
			return fmt.Errorf("find latest save: %w", latestErr)
		}
		args = append(args, "--start-server", filepath.Join(config.FactorioSavesDir, latestSave.Name))
	} else {
		if err := ValidatePathElement(server.Savefile); err != nil {
			return fmt.Errorf("invalid save name: %w", err)
		}
		args = append(args, "--start-server", filepath.Join(config.FactorioSavesDir, server.Savefile))
	}

	// Write chat log to a different file if requested (if not it will be mixed-in with the default logfile)
	if config.ChatLogFile != "" {
		args = append(args, "--console-log", config.ChatLogFile)
	}

	if config.GlibcCustom == "true" {
		log.Printf("Starting Factorio server with custom glibc at %s", config.GlibcLocation)
		server.Cmd = exec.Command(config.GlibcLocation, args...)
	} else {
		log.Printf("Starting Factorio server binary: %s", config.FactorioBinary)
		server.Cmd = exec.Command(config.FactorioBinary, args...)
	}

	server.StdOut, err = server.Cmd.StdoutPipe()
	if err != nil {
		log.Printf("Error opening stdout pipe: %s", err)
		return err
	}

	server.StdIn, err = server.Cmd.StdinPipe()
	if err != nil {
		log.Printf("Error opening stdin pipe: %s", err)
		return err
	}

	server.StdErr, err = server.Cmd.StderrPipe()
	if err != nil {
		log.Printf("Error opening stderr pipe: %s", err)
		return err
	}

	go server.parseRunningCommand(server.StdOut)
	go server.parseRunningCommand(server.StdErr)

	err = server.Cmd.Start()
	if err != nil {
		log.Printf("Factorio process failed to start: %s", err)
		return err
	}
	server.SetRunning(true)
	worldGenerationLock.Unlock()
	startupGuardHeld = false

	err = server.Cmd.Wait()
	log.Printf("Factorio process is closed")
	server.closeRcon()
	server.SetRunning(false)
	if err != nil {
		log.Printf("Factorio process exited with error: %s", err)
		return err
	}

	return nil
}

func (server *Server) parseRunningCommand(std io.ReadCloser) (err error) {
	stdScanner := bufio.NewScanner(std)
	for stdScanner.Scan() {
		text := stdScanner.Text()

		log.Printf("Factorio Server: %s", text)
		if err := server.writeLog(text); err != nil {
			log.Printf("Error: %s", err)
		}

		// Internal localhost RCON probes are implementation noise, not useful game output.
		if !isManagerRCONConnectionLog(text) {
			wsRoom := websocket.WebsocketHub.GetRoom("gamelog")
			go wsRoom.Send(text)
		}

		line := strings.Fields(text)
		// Ensure logline slice is in bounds
		if len(line) > 1 {
			// Check if Factorio Server reports any errors if so handle it
			if line[1] == "Error" {
				err := server.checkLogError(line)
				if err != nil {
					log.Printf("Error checking Factorio Server Error: %s", err)
				}
			}
			// If rcon port opens indicated in log connect to rcon
			rconLog := "Starting RCON interface at IP"
			// check if slice index is greater than 2 to prevent panic
			if len(line) > 2 {
				// log line for opened rcon connection
				if strings.Contains(text, rconLog) {
					log.Printf("Rcon running on Factorio Server")
					err = connectRC()
					if err != nil {
						log.Printf("Error: %s", err)
					}
				}

				server.checkProcessHealth(text)
			}
		}
	}
	if err := stdScanner.Err(); err != nil {
		log.Printf("Error reading std buffer: %s", err)
		return err
	}
	return nil
}

func (server *Server) writeLog(logline string) error {
	config := bootstrap.GetConfig()
	logfileName := config.ConsoleLogFile
	file, err := os.OpenFile(logfileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Cannot open logfile %s for appending Factorio Server output: %s", logfileName, err)
		return err
	}
	defer file.Close()

	logline = logline + "\n"

	if _, err = file.WriteString(logline); err != nil {
		log.Printf("Error appending to %s: %s", logfileName, err)
		return err
	}

	return nil
}

func (server *Server) checkLogError(logline []string) error {
	// TODO Handle errors generated by running Factorio Server
	log.Println(logline)

	return nil
}

func init() {
	websocket.WebsocketHub.RegisterControlHandler <- serverWebsocketControl
}

// react to websocket control messages and run the command if it is requested
func serverWebsocketControl(controls websocket.WsControls) {
	log.Println(controls)
	if controls.Type == "command" {
		command := controls.Value
		server := GetFactorioServer()
		if server.GetRunning() {
			log.Printf("Received command: %v", command)

			console := server.getRcon()
			if console == nil {
				log.Printf("Cannot send command before the RCON connection is ready")
				return
			}
			reqId, err := console.Write(command)
			if err != nil {
				log.Printf("Error sending rcon command: %s", err)
				return
			}

			log.Printf("Command sent to Factorio with rcon request id: %v", reqId)
		}
	}
}
