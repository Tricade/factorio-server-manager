package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlayerOverviewHandler(t *testing.T) {
	originalGet := getPlayerOverview
	getPlayerOverview = func() (factorio.PlayerOverview, error) {
		return factorio.PlayerOverview{
			ProfileID:     "0123456789abcdef",
			ServerRunning: true,
			LiveAvailable: true,
			OnlineCount:   1,
			OnlinePlayers: []string{"Ada"},
			Players: []factorio.PlayerOverviewPlayer{{
				MapSnapshotPlayer: factorio.MapSnapshotPlayer{Name: "Ada", OnlineTimeTicks: 7200, OnlineTimeSeconds: 120, Rank: 1},
				Online:            true,
			}},
		}, nil
	}
	t.Cleanup(func() { getPlayerOverview = originalGet })

	recorder := httptest.NewRecorder()
	GetPlayerOverview(recorder, httptest.NewRequest(http.MethodGet, "/api/server/players", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{
		"profile_id":"0123456789abcdef",
		"server_running":true,
		"live_available":true,
		"online_count":1,
		"online_players":["Ada"],
		"players":[{"name":"Ada","online_time_ticks":7200,"online_time_seconds":120,"rank":1,"online":true}]
	}`, recorder.Body.String())
}

func TestGetPlayerOverviewHandlerReportsBackendFailure(t *testing.T) {
	originalGet := getPlayerOverview
	getPlayerOverview = func() (factorio.PlayerOverview, error) {
		return factorio.PlayerOverview{}, errors.New("disk error")
	}
	t.Cleanup(func() { getPlayerOverview = originalGet })

	recorder := httptest.NewRecorder()
	GetPlayerOverview(recorder, httptest.NewRequest(http.MethodGet, "/api/server/players", nil))
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}
