package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
)

var previewModPackMerge = factorio.PreviewModPackMerge
var mergeModPack = factorio.MergeModPack

func PreviewModPackMergeHandler(w http.ResponseWriter, r *http.Request) {
	plan, err := previewModPackMerge(mux.Vars(r)["modpack"])
	writeModPackMergeResult(w, plan, err)
}

func MergeModPackHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Revision string `json:"revision"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.Revision == "" {
		http.Error(w, "A preview revision is required", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Expected one JSON object", http.StatusBadRequest)
		return
	}
	plan, err := mergeModPack(mux.Vars(r)["modpack"], request.Revision)
	writeModPackMergeResult(w, plan, err)
}

func writeModPackMergeResult(w http.ResponseWriter, plan factorio.ModPackMergePlan, err error) {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	if err == nil {
		WriteResponse(w, plan)
		return
	}
	status := http.StatusUnprocessableEntity
	message := "Unable to read or combine these mod files. Check the manager log; ZIP mods with one version per mod are required."
	switch {
	case errors.Is(err, factorio.ErrServerActive):
		status, message = http.StatusLocked, "Stop Factorio before adding a mod pack."
	case errors.Is(err, factorio.ErrModPackMergeStale), errors.Is(err, factorio.ErrModPackMergeConflict):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, factorio.ErrInvalidModPackMerge):
		status, message = http.StatusBadRequest, err.Error()
	default:
		log.Printf("Mod pack merge: %v", err)
	}
	w.WriteHeader(status)
	WriteResponse(w, map[string]interface{}{"error": message, "plan": plan})
}
