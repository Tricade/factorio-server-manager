package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapSnapshotHandlers(t *testing.T) {
	originalGet := getMapSnapshotState
	originalTrigger := triggerMapSnapshot
	originalSet := setMapSnapshotSettings
	originalRead := readMapSnapshotImage
	generatedAt := time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC)
	state := factorio.MapSnapshotState{
		Settings: factorio.MapSnapshotSettings{IntervalMinutes: 60},
		Snapshot: &factorio.MapSnapshot{
			SchemaVersion: 1, ProfileID: "0123456789abcdef", GeneratedAt: generatedAt,
			Surfaces: []factorio.MapSnapshotSurface{{ID: "1", Index: 1, Name: "nauvis", Width: 32, Height: 32}},
		},
	}
	getMapSnapshotState = func() (factorio.MapSnapshotState, error) { return state, nil }
	triggerMapSnapshot = func() (factorio.MapSnapshotState, error) {
		state.Running = true
		return state, nil
	}
	setMapSnapshotSettings = func(settings factorio.MapSnapshotSettings) (factorio.MapSnapshotSettings, error) {
		state.Settings = settings
		return settings, nil
	}
	readMapSnapshotImage = func(surface string) ([]byte, time.Time, error) {
		if surface != "1" {
			return nil, time.Time{}, factorio.ErrMapSnapshotSurfaceNotFound
		}
		return []byte("png"), generatedAt, nil
	}
	t.Cleanup(func() {
		getMapSnapshotState = originalGet
		triggerMapSnapshot = originalTrigger
		setMapSnapshotSettings = originalSet
		readMapSnapshotImage = originalRead
	})

	getRecorder := httptest.NewRecorder()
	GetMapSnapshot(getRecorder, httptest.NewRequest(http.MethodGet, "/api/map-snapshot", nil))
	require.Equal(t, http.StatusOK, getRecorder.Code)
	assert.Equal(t, "no-store", getRecorder.Header().Get("Cache-Control"))
	assert.Contains(t, getRecorder.Body.String(), `"interval_minutes":60`)

	refreshRecorder := httptest.NewRecorder()
	RefreshMapSnapshot(refreshRecorder, httptest.NewRequest(http.MethodPost, "/api/map-snapshot/refresh", nil))
	require.Equal(t, http.StatusAccepted, refreshRecorder.Code)
	assert.Contains(t, refreshRecorder.Body.String(), `"running":true`)

	settingsRecorder := httptest.NewRecorder()
	UpdateMapSnapshotSettings(settingsRecorder, httptest.NewRequest(http.MethodPut, "/api/map-snapshot/settings", strings.NewReader(`{"interval_minutes":135}`)))
	require.Equal(t, http.StatusOK, settingsRecorder.Code)
	assert.JSONEq(t, `{"interval_minutes":135}`, settingsRecorder.Body.String())

	imageRecorder := httptest.NewRecorder()
	imageRequest := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/map-snapshot/surfaces/1", nil), map[string]string{"surface": "1"})
	GetMapSnapshotImage(imageRecorder, imageRequest)
	require.Equal(t, http.StatusOK, imageRecorder.Code)
	assert.Equal(t, "image/png", imageRecorder.Header().Get("Content-Type"))
	assert.Equal(t, "png", imageRecorder.Body.String())
}

func TestMapSnapshotHandlersValidateRequestsAndErrors(t *testing.T) {
	originalTrigger := triggerMapSnapshot
	originalSet := setMapSnapshotSettings
	originalRead := readMapSnapshotImage
	t.Cleanup(func() {
		triggerMapSnapshot = originalTrigger
		setMapSnapshotSettings = originalSet
		readMapSnapshotImage = originalRead
	})

	invalidRecorder := httptest.NewRecorder()
	UpdateMapSnapshotSettings(invalidRecorder, httptest.NewRequest(http.MethodPut, "/api/map-snapshot/settings", strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)

	setMapSnapshotSettings = func(factorio.MapSnapshotSettings) (factorio.MapSnapshotSettings, error) {
		return factorio.MapSnapshotSettings{}, factorio.ErrInvalidMapSnapshotSettings
	}
	invalidIntervalRecorder := httptest.NewRecorder()
	UpdateMapSnapshotSettings(invalidIntervalRecorder, httptest.NewRequest(http.MethodPut, "/api/map-snapshot/settings", strings.NewReader(`{"interval_minutes":-1}`)))
	assert.Equal(t, http.StatusBadRequest, invalidIntervalRecorder.Code)

	triggerMapSnapshot = func() (factorio.MapSnapshotState, error) {
		return factorio.MapSnapshotState{}, factorio.ErrMapSnapshotBusy
	}
	busyRecorder := httptest.NewRecorder()
	RefreshMapSnapshot(busyRecorder, httptest.NewRequest(http.MethodPost, "/api/map-snapshot/refresh", nil))
	assert.Equal(t, http.StatusConflict, busyRecorder.Code)

	readMapSnapshotImage = func(string) ([]byte, time.Time, error) {
		return nil, time.Time{}, errors.New("disk error")
	}
	imageRecorder := httptest.NewRecorder()
	imageRequest := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/map-snapshot/surfaces/1", nil), map[string]string{"surface": "1"})
	GetMapSnapshotImage(imageRecorder, imageRequest)
	assert.Equal(t, http.StatusInternalServerError, imageRecorder.Code)
}
