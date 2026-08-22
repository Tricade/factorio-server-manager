package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

const maxProfileRequestSize = 64 * 1024

func ListProfilesHandler(w http.ResponseWriter, _ *http.Request) {
	state, err := factorio.ListProfiles()
	if err != nil {
		writeProfileError(w, "Unable to list profiles", err)
		return
	}
	writeProfileState(w, state)
}

func CreateProfileHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
	}
	if !decodeProfileRequest(w, r, &request) {
		return
	}
	state, err := factorio.CreateProfile(request.Name, request.Description, request.Source)
	if err != nil {
		writeProfileError(w, "Unable to create profile", err)
		return
	}
	writeProfileStateWithStatus(w, state, http.StatusCreated)
}

func UpdateProfileHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeProfileRequest(w, r, &request) {
		return
	}
	state, err := factorio.UpdateProfile(mux.Vars(r)["profile"], request.Name, request.Description)
	if err != nil {
		writeProfileError(w, "Unable to update profile", err)
		return
	}
	writeProfileState(w, state)
}

func UpdateProfileStartupHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		BindIP       string `json:"bind_ip"`
		Port         int    `json:"port"`
		SelectedSave string `json:"selected_save"`
	}
	if !decodeProfileRequest(w, r, &request) {
		return
	}
	state, err := factorio.UpdateProfileStartup(mux.Vars(r)["profile"], request.BindIP, request.Port, request.SelectedSave)
	if err != nil {
		writeProfileError(w, "Unable to update profile startup settings", err)
		return
	}
	writeProfileState(w, state)
}

func DeleteProfileHandler(w http.ResponseWriter, r *http.Request) {
	state, err := factorio.DeleteProfile(mux.Vars(r)["profile"])
	if err != nil {
		writeProfileError(w, "Unable to delete profile", err)
		return
	}
	writeProfileState(w, state)
}

func ActivateProfileHandler(w http.ResponseWriter, r *http.Request) {
	state, err := factorio.ActivateProfile(mux.Vars(r)["profile"])
	if err != nil {
		writeProfileError(w, "Unable to activate profile", err)
		return
	}
	writeProfileState(w, state)
}

func decodeProfileRequest(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxProfileRequestSize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse profile request: %s", err), http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Unable to parse profile request: request must contain one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func writeProfileState(w http.ResponseWriter, state factorio.ProfileState) {
	writeProfileStateWithStatus(w, state, http.StatusOK)
}

func writeProfileStateWithStatus(w http.ResponseWriter, state factorio.ProfileState, status int) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	WriteResponse(w, state)
}

func writeProfileError(w http.ResponseWriter, prefix string, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, factorio.ErrProfileNotFound):
		status = http.StatusNotFound
	case errors.Is(err, factorio.ErrInvalidProfile):
		status = http.StatusBadRequest
	case errors.Is(err, factorio.ErrProfileNameConflict), errors.Is(err, factorio.ErrActiveProfileDelete):
		status = http.StatusConflict
	case errors.Is(err, factorio.ErrProfileServerActive):
		status = http.StatusLocked
	}
	http.Error(w, fmt.Sprintf("%s: %s", prefix, err), status)
}
