package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

func LogTail(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	config := bootstrap.GetConfig()
	resp, err = factorio.TailLog()
	if err != nil {
		resp = fmt.Sprintf("Could not tail %s: %s", config.FactorioLog, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// LoadConfig returns JSON response of config.ini file

func LoadConfig(w http.ResponseWriter, r *http.Request) {
	var err error
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	config := bootstrap.GetConfig()
	configContents, err := factorio.LoadConfig(config.FactorioConfigFile)
	if err != nil {
		resp = fmt.Sprintf("Could not retrieve config.ini: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp = configForRequest(configContents, requestUserIsAdministrator(r))

	log.Printf("Sent config.ini response")
}

func StartServer(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	var server = factorio.GetFactorioServer()
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	if factorio.WorldGenerationBusy() {
		resp = "A world preview or creation is still running"
		w.WriteHeader(http.StatusConflict)
		return
	}

	if server.IsBusy() {
		resp = "Factorio server is already running or stopping"
		w.WriteHeader(http.StatusConflict)
		return
	}

	log.Printf("Starting Factorio server.")

	body, resp, err := ReadRequestBody(w, r)
	if err != nil {
		return
	}

	var request struct {
		Savefile string `json:"savefile"`
		BindIP   string `json:"bindip"`
		Port     int    `json:"port"`
	}
	err = json.Unmarshal(body, &request)
	if err != nil {
		resp = fmt.Sprintf("Error unmarshalling server settings JSON: %s", err)
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check if savefile was submitted with request to start server.
	if request.Savefile == "" {
		resp = "Error starting Factorio server: No save file provided"
		log.Println(resp)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	parsedIP := net.ParseIP(request.BindIP)
	if parsedIP == nil || parsedIP.To4() == nil {
		resp = "Error starting Factorio server: invalid IPv4 bind address"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if request.Port < 1 || request.Port > 65535 {
		resp = "Error starting Factorio server: port must be between 1 and 65535"
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("Starting Factorio server on %s:%d with save %q", request.BindIP, request.Port, request.Savefile)
	startResult, startErr := server.Start(request.BindIP, request.Port, request.Savefile)
	if startErr != nil {
		resp = fmt.Sprintf("Error starting Factorio server: %s", startErr)
		if errors.Is(startErr, factorio.ErrServerActive) || errors.Is(startErr, factorio.ErrWorldGenerationBusy) {
			w.WriteHeader(http.StatusConflict)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	deadline := time.NewTimer(4 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()

	started := false
	for !started {
		select {
		case runErr := <-startResult:
			if runErr != nil {
				resp = fmt.Sprintf("Error starting Factorio server: %s", runErr)
				if errors.Is(runErr, factorio.ErrWorldGenerationBusy) {
					w.WriteHeader(http.StatusConflict)
					return
				}
			} else {
				resp = "Factorio server exited before startup completed"
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		case <-ticker.C:
			started = server.GetRunning()
		case <-deadline.C:
			resp = "Error starting Factorio server: startup timed out"
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
	}

	resp = fmt.Sprintf("Factorio server with save: %s started on port: %d", request.Savefile, request.Port)
	log.Println(resp)
}

func StopServer(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	var server = factorio.GetFactorioServer()
	if server.IsStopping() {
		resp = "Factorio server is already stopping"
		w.WriteHeader(http.StatusConflict)
		return
	}
	if server.GetRunning() {
		err := server.Stop()
		if err != nil {
			resp = fmt.Sprintf("Error stopping factorio server: %s", err)
			log.Println(resp)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp = "Factorio safe shutdown requested"
		log.Println(resp)
	} else {
		resp = "Factorio server is not running"
		w.WriteHeader(http.StatusConflict)
		return
	}
}

func KillServer(w http.ResponseWriter, r *http.Request) {
	var resp interface{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	var server = factorio.GetFactorioServer()
	if server.GetRunning() {
		err := server.Kill()
		if err != nil {
			resp = fmt.Sprintf("Error killing factorio server: %s", err)
			log.Println(resp)
			return
		}

		log.Printf("Killed Factorio server.")
		resp = "Factorio server killed"
	} else {
		resp = "Factorio server is not running"
		w.WriteHeader(http.StatusBadRequest)
	}
}

func CheckServer(w http.ResponseWriter, r *http.Request) {
	defer func() {
		WriteResponse(w, factorio.GetFactorioServer().Snapshot())
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
}

func FactorioVersion(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{}

	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	snapshot := factorio.GetFactorioServer().Snapshot()
	resp["version"] = snapshot.Version.String()
	resp["base_mod_version"] = snapshot.BaseModVersion
}

func FactorioReleaseStatus(w http.ResponseWriter, r *http.Request) {
	status, err := factorio.GetReleaseStatus()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading official Factorio release status: %s", err), http.StatusBadGateway)
		return
	}
	status = releaseStatusForRequest(status, requestUserIsAdministrator(r))
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, status)
}

func releaseStatusForRequest(status factorio.ReleaseStatus, administrator bool) factorio.ReleaseStatus {
	if !administrator && status.MetadataError != "" {
		status.MetadataError = "Official release metadata is temporarily unavailable."
	}
	return status
}

func GetGameMode(w http.ResponseWriter, r *http.Request) {
	status, err := factorio.GetGameModeStatus()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading Factorio game mode: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, status)
}

func UpdateGameMode(w http.ResponseWriter, r *http.Request) {
	body, response, err := ReadRequestBody(w, r)
	if err != nil {
		WriteResponse(w, response)
		return
	}
	var request struct {
		Mode factorio.GameMode `json:"mode"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse the request body: %s", err), http.StatusBadRequest)
		return
	}
	status, err := factorio.SetGameMode(request.Mode)
	if err != nil {
		statusCode := http.StatusBadRequest
		if errors.Is(err, factorio.ErrServerActive) {
			statusCode = http.StatusLocked
		}
		http.Error(w, fmt.Sprintf("Error changing Factorio game mode: %s", err), statusCode)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, status)
}

// InstallFactorioRelease installs a supported official Factorio headless release.
// The requested rolling channel or exact version is validated before download.

func InstallFactorioRelease(w http.ResponseWriter, r *http.Request) {
	var resp interface{}
	defer func() {
		WriteResponse(w, resp)
	}()

	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	body, bodyError, err := ReadRequestBody(w, r)
	if err != nil {
		resp = bodyError
		return
	}
	var request struct {
		Target  string `json:"target"`
		Channel string `json:"channel"` // backwards-compatible with 0.11.0 clients
	}
	if err := json.Unmarshal(body, &request); err != nil {
		resp = fmt.Sprintf("Unable to parse the request body: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	target := request.Target
	if target == "" {
		target = request.Channel
	}
	normalizedTarget, err := factorio.NormalizeReleaseTarget(target)
	if err != nil {
		resp = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	server := factorio.GetFactorioServer()
	if server.GetRunning() || server.IsStopping() {
		resp = "Stop the Factorio server before installing a release"
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err := factorio.InstallRelease(normalizedTarget); err != nil {
		resp = fmt.Sprintf("Error installing Factorio %s release: %s", normalizedTarget, err)
		w.WriteHeader(installReleaseErrorStatus(err))
		return
	}
	resp = map[string]string{
		"target":            normalizedTarget,
		"installed_version": factorio.GetFactorioServer().Snapshot().BaseModVersion,
		"message":           "Factorio release installed",
	}
}

func installReleaseErrorStatus(err error) int {
	if errors.Is(err, factorio.ErrServerActive) || errors.Is(err, factorio.ErrWorldGenerationBusy) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// Unmarshall the User object from the given bytearray
// This function has side effects (it will write to resp and to w, in case of an error)
