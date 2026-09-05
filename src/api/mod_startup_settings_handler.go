package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

var getModStartupSettings = factorio.GetModStartupSettings
var updateModStartupSettings = factorio.UpdateModStartupSettings

func GetModStartupSettingsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	settings, err := getModStartupSettings()
	if err != nil {
		writeModStartupSettingsError(w, err)
		return
	}
	WriteResponse(w, settings)
}

func UpdateModStartupSettingsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	body, response, err := ReadRequestBody(w, r)
	if err != nil {
		WriteResponse(w, response)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request factorio.ModStartupSettingsUpdate
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "Invalid mod startup settings request.", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "Invalid mod startup settings request.", http.StatusBadRequest)
		return
	}
	settings, err := updateModStartupSettings(request)
	if err != nil {
		writeModStartupSettingsError(w, err)
		return
	}
	WriteResponse(w, settings)
}

func writeModStartupSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, factorio.ErrServerActive), errors.Is(err, factorio.ErrModStartupSettingsBusy):
		http.Error(w, "Stop Factorio and wait for other Factorio operations to finish before editing mod startup settings.", http.StatusLocked)
	case errors.Is(err, factorio.ErrModStartupSettingsStale):
		http.Error(w, "The active profile, Factorio version, mods, or settings changed. Reload the form and try again.", http.StatusConflict)
	case errors.Is(err, factorio.ErrInvalidModStartupSettings), errors.Is(err, factorio.ErrInvalidModSettingsFile), errors.Is(err, factorio.ErrUnsupportedModSettingsFile):
		http.Error(w, "The mod startup settings are invalid or unsupported. The previous configuration was kept.", http.StatusUnprocessableEntity)
	case errors.Is(err, factorio.ErrModStartupSettingsUnsupported):
		http.Error(w, "This Factorio version cannot expose mod startup settings in the web interface.", http.StatusUnprocessableEntity)
	default:
		// Child-process output can originate in third-party mods and may contain
		// setting values. Keep both the response and the manager log value-free.
		http.Error(w, "Mod startup settings could not be evaluated. The previous configuration was kept.", http.StatusInternalServerError)
	}
}
