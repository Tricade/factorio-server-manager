package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

const maxWorldGenerationRequestSize = 1024 * 1024

func GetWorldGenerationOptions(w http.ResponseWriter, _ *http.Request) {
	options, err := factorio.GetWorldGenerationOptions()
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to inspect world generation options: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	WriteResponse(w, options)
}

func PreviewWorld(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeWorldGenerationRequest(w, r)
	if !ok {
		return
	}
	if err := factorio.ValidateWorldGenerationRequest(request, true); err != nil {
		http.Error(w, fmt.Sprintf("Invalid world settings: %s", err), http.StatusBadRequest)
		return
	}
	preview, err := factorio.GenerateWorldPreview(request)
	if err != nil {
		writeWorldGenerationError(w, "Unable to generate map preview", err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(preview)))
	w.Header().Set("Cache-Control", "no-store")
	planet := strings.TrimSpace(strings.ToLower(request.Planet))
	if planet == "" {
		planet = "nauvis"
	}
	w.Header().Set("X-Factorio-Planet", planet)
	w.Header().Set("X-Factorio-Seed", strconv.FormatUint(request.Seed, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(preview)
}

func CreateWorld(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeWorldGenerationRequest(w, r)
	if !ok {
		return
	}
	if err := factorio.ValidateWorldGenerationRequest(request, false); err != nil {
		http.Error(w, fmt.Sprintf("Invalid world settings: %s", err), http.StatusBadRequest)
		return
	}
	save, err := factorio.CreateWorld(request)
	if err != nil {
		writeWorldGenerationError(w, "Unable to create world", err)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	WriteResponse(w, save)
}

func decodeWorldGenerationRequest(w http.ResponseWriter, r *http.Request) (factorio.WorldGenerationRequest, bool) {
	var request factorio.WorldGenerationRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxWorldGenerationRequestSize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Unable to parse world settings: %s", err), http.StatusBadRequest)
		return request, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Unable to parse world settings: request must contain one JSON object", http.StatusBadRequest)
		return request, false
	}
	return request, true
}

func writeWorldGenerationError(w http.ResponseWriter, prefix string, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, factorio.ErrWorldGenerationBusy), errors.Is(err, factorio.ErrSaveAlreadyExists):
		status = http.StatusConflict
	case errors.Is(err, factorio.ErrFactorioMustBeStopped):
		status = http.StatusLocked
	}
	http.Error(w, fmt.Sprintf("%s: %s", prefix, err), status)
}
