package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

var loadModPortalPreferences = factorio.LoadModPortalPreferences
var setModPortalPreferences = factorio.SetModPortalPreferences

func GetModPortalPreferences(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	preferences, err := loadModPortalPreferences()
	if err != nil {
		log.Printf("Load mod portal preferences: %v", err)
		http.Error(w, "Unable to load mod portal preferences", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, preferences)
}

func UpdateModPortalPreferences(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var request struct {
		PreselectOptional *bool `json:"preselect_optional"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.PreselectOptional == nil {
		http.Error(w, "A boolean preselect_optional value is required", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Expected one JSON object", http.StatusBadRequest)
		return
	}
	preferences, err := setModPortalPreferences(factorio.ModPortalPreferences{PreselectOptional: *request.PreselectOptional})
	if err != nil {
		log.Printf("Save mod portal preferences: %v", err)
		http.Error(w, "Unable to save mod portal preferences", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	WriteResponse(w, preferences)
}
