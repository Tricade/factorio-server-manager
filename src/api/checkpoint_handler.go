package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

const maxCheckpointRequestSize = 8 * 1024

var getCheckpointState = factorio.GetCheckpointState
var updateCheckpointSettings = factorio.UpdateCheckpointSettings
var createCheckpoint = factorio.CreateCheckpoint
var restoreCheckpoint = factorio.RestoreCheckpoint
var deleteCheckpoint = factorio.DeleteCheckpoint
var findCheckpointFile = factorio.FindCheckpointFile

func ListCheckpoints(w http.ResponseWriter, _ *http.Request) {
	state, err := getCheckpointState()
	if err != nil {
		writeCheckpointError(w, "Unable to load checkpoints", err)
		return
	}
	writeCheckpointResponse(w, state, http.StatusOK)
}

func SaveCheckpointSettings(w http.ResponseWriter, r *http.Request) {
	var settings factorio.CheckpointSettings
	if !decodeCheckpointRequest(w, r, &settings) {
		return
	}
	state, err := updateCheckpointSettings(settings)
	if err != nil {
		writeCheckpointError(w, "Unable to save checkpoint settings", err)
		return
	}
	writeCheckpointResponse(w, state, http.StatusOK)
}

func CreateCheckpoint(w http.ResponseWriter, _ *http.Request) {
	state, err := createCheckpoint(factorio.CheckpointTriggerManual)
	if err != nil {
		writeCheckpointError(w, "Unable to create checkpoint", err)
		return
	}
	writeCheckpointResponse(w, state, http.StatusCreated)
}

func RestoreCheckpoint(w http.ResponseWriter, r *http.Request) {
	save, err := restoreCheckpoint(mux.Vars(r)["checkpoint"])
	if err != nil {
		writeCheckpointError(w, "Unable to restore checkpoint", err)
		return
	}
	writeCheckpointResponse(w, save, http.StatusCreated)
}

func DeleteCheckpoint(w http.ResponseWriter, r *http.Request) {
	state, err := deleteCheckpoint(mux.Vars(r)["checkpoint"])
	if err != nil {
		writeCheckpointError(w, "Unable to delete checkpoint", err)
		return
	}
	writeCheckpointResponse(w, state, http.StatusOK)
}

func DownloadCheckpoint(w http.ResponseWriter, r *http.Request) {
	checkpoint, path, err := findCheckpointFile(mux.Vars(r)["checkpoint"])
	if err != nil {
		writeCheckpointError(w, "Unable to download checkpoint", err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": checkpoint.FileName}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

func decodeCheckpointRequest(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxCheckpointRequestSize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse checkpoint settings: %s", err), http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Unable to parse checkpoint settings: request must contain one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func writeCheckpointResponse(w http.ResponseWriter, value interface{}, status int) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	WriteResponse(w, value)
}

func writeCheckpointError(w http.ResponseWriter, prefix string, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, factorio.ErrCheckpointNotFound):
		status = http.StatusNotFound
	case errors.Is(err, factorio.ErrInvalidCheckpointSetting):
		status = http.StatusBadRequest
	case errors.Is(err, factorio.ErrCheckpointServerActive):
		status = http.StatusLocked
	}
	http.Error(w, fmt.Sprintf("%s: %s", prefix, err), status)
}
