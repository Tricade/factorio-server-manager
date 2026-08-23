package api

import (
	"fmt"
	"net/http"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

var getPlayerOverview = factorio.GetPlayerOverview

func GetPlayerOverview(w http.ResponseWriter, _ *http.Request) {
	overview, err := getPlayerOverview()
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to load player overview: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	WriteResponse(w, overview)
}
