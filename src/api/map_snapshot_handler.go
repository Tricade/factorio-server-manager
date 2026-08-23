package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

const maxMapSnapshotSettingsRequestSize = 4 * 1024

var getMapSnapshotState = factorio.GetMapSnapshotState
var triggerMapSnapshot = factorio.TriggerMapSnapshot
var loadMapSnapshotSettings = factorio.LoadMapSnapshotSettings
var setMapSnapshotSettings = factorio.SetMapSnapshotSettings
var readMapSnapshotImage = factorio.ReadMapSnapshotImage
var readMapSnapshotEntities = factorio.ReadMapSnapshotEntities

func GetMapSnapshot(w http.ResponseWriter, _ *http.Request) {
	state, err := getMapSnapshotState()
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to load map snapshot: %s", err), http.StatusInternalServerError)
		return
	}
	writeMapSnapshotState(w, state, http.StatusOK)
}

func RefreshMapSnapshot(w http.ResponseWriter, _ *http.Request) {
	state, err := triggerMapSnapshot()
	if errors.Is(err, factorio.ErrMapSnapshotBusy) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if errors.Is(err, factorio.ErrMapSnapshotsDisabled) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to start map snapshot: %s", err), http.StatusUnprocessableEntity)
		return
	}
	writeMapSnapshotState(w, state, http.StatusAccepted)
}

func UpdateMapSnapshotSettings(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enabled                    *bool `json:"enabled"`
		IntervalMinutes            *int  `json:"interval_minutes"`
		AutomaticOnlyWhenNoPlayers *bool `json:"automatic_only_when_no_players"`
		IncludeSpacePlatforms      *bool `json:"include_space_platforms"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMapSnapshotSettingsRequestSize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse map snapshot settings: %s", err), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Unable to parse map snapshot settings: request must contain one JSON object", http.StatusBadRequest)
		return
	}
	if request.IntervalMinutes == nil {
		http.Error(w, "Unable to parse map snapshot settings: interval_minutes is required", http.StatusBadRequest)
		return
	}
	enabled := request.Enabled
	automaticOnlyWhenNoPlayers := request.AutomaticOnlyWhenNoPlayers
	includeSpacePlatforms := request.IncludeSpacePlatforms
	if enabled == nil || automaticOnlyWhenNoPlayers == nil || includeSpacePlatforms == nil {
		current, err := loadMapSnapshotSettings()
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to load map snapshot settings: %s", err), http.StatusInternalServerError)
			return
		}
		if enabled == nil {
			enabled = &current.Enabled
		}
		if automaticOnlyWhenNoPlayers == nil {
			automaticOnlyWhenNoPlayers = &current.AutomaticOnlyWhenNoPlayers
		}
		if includeSpacePlatforms == nil {
			includeSpacePlatforms = &current.IncludeSpacePlatforms
		}
	}
	settings, err := setMapSnapshotSettings(factorio.MapSnapshotSettings{
		Enabled:                    *enabled,
		IntervalMinutes:            *request.IntervalMinutes,
		AutomaticOnlyWhenNoPlayers: *automaticOnlyWhenNoPlayers,
		IncludeSpacePlatforms:      *includeSpacePlatforms,
	})
	if errors.Is(err, factorio.ErrInvalidMapSnapshotSettings) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to save map snapshot settings: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	WriteResponse(w, settings)
}

func GetMapSnapshotImage(w http.ResponseWriter, r *http.Request) {
	surfaceID := mux.Vars(r)["surface"]
	contents, generatedAt, err := readMapSnapshotImage(surfaceID)
	if errors.Is(err, factorio.ErrMapSnapshotNotFound) || errors.Is(err, factorio.ErrMapSnapshotSurfaceNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to load map snapshot image: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "surface-"+surfaceID+".png", generatedAt, bytes.NewReader(contents))
}

func GetMapSnapshotEntities(w http.ResponseWriter, r *http.Request) {
	surfaceID := mux.Vars(r)["surface"]
	contents, generatedAt, err := readMapSnapshotEntities(surfaceID)
	if errors.Is(err, factorio.ErrMapSnapshotNotFound) || errors.Is(err, factorio.ErrMapSnapshotSurfaceNotFound) || errors.Is(err, factorio.ErrMapSnapshotDetailsNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to load map snapshot entity details: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "surface-"+surfaceID+"-entities.jsonl", generatedAt, bytes.NewReader(contents))
}

func writeMapSnapshotState(w http.ResponseWriter, state factorio.MapSnapshotState, status int) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	WriteResponse(w, state)
}
