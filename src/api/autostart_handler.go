package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

const maxAutostartRequestSize = 4 * 1024

var loadAutostartSettings = factorio.LoadAutostartSettings
var setAutostartEnabled = factorio.SetAutostartEnabled

func GetAutostartSettings(w http.ResponseWriter, _ *http.Request) {
	settings, err := loadAutostartSettings()
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to load autostart settings: %s", err), http.StatusInternalServerError)
		return
	}
	writeAutostartSettings(w, settings)
}

func UpdateAutostartSettings(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAutostartRequestSize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse autostart settings: %s", err), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Unable to parse autostart settings: request must contain one JSON object", http.StatusBadRequest)
		return
	}
	if request.Enabled == nil {
		http.Error(w, "Unable to parse autostart settings: enabled is required", http.StatusBadRequest)
		return
	}

	settings, err := setAutostartEnabled(*request.Enabled)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to save autostart settings: %s", err), http.StatusInternalServerError)
		return
	}
	writeAutostartSettings(w, settings)
}

func writeAutostartSettings(w http.ResponseWriter, settings factorio.AutostartSettings) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	WriteResponse(w, settings)
}
