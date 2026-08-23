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
	originalLoad := loadMapSnapshotSettings
	originalSet := setMapSnapshotSettings
	originalRead := readMapSnapshotImage
	originalReadEntities := readMapSnapshotEntities
	generatedAt := time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC)
	state := factorio.MapSnapshotState{
		Settings: factorio.MapSnapshotSettings{Enabled: true, IntervalMinutes: 60, AutomaticOnlyWhenNoPlayers: false, IncludeSpacePlatforms: true},
		Snapshot: &factorio.MapSnapshot{
			SchemaVersion: 1, ProfileID: "0123456789abcdef", GeneratedAt: generatedAt,
			Surfaces: []factorio.MapSnapshotSurface{{
				ID: "1", Index: 1, Name: "nauvis", ChunkCount: 1, Width: 32, Height: 32,
				ViewBoundsAvailable: true, ViewMinTileX: 0, ViewMinTileY: 0, ViewMaxTileX: 31, ViewMaxTileY: 31, PixelsPerTile: 1,
			}},
		},
	}
	getMapSnapshotState = func() (factorio.MapSnapshotState, error) { return state, nil }
	triggerMapSnapshot = func() (factorio.MapSnapshotState, error) {
		state.Running = true
		return state, nil
	}
	loadMapSnapshotSettings = func() (factorio.MapSnapshotSettings, error) { return state.Settings, nil }
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
	readMapSnapshotEntities = func(surface string) ([]byte, time.Time, error) {
		if surface != "1" {
			return nil, time.Time{}, factorio.ErrMapSnapshotSurfaceNotFound
		}
		return []byte("{\"name\":\"assembling-machine-3\"}\n"), generatedAt, nil
	}
	t.Cleanup(func() {
		getMapSnapshotState = originalGet
		triggerMapSnapshot = originalTrigger
		loadMapSnapshotSettings = originalLoad
		setMapSnapshotSettings = originalSet
		readMapSnapshotImage = originalRead
		readMapSnapshotEntities = originalReadEntities
	})

	getRecorder := httptest.NewRecorder()
	GetMapSnapshot(getRecorder, httptest.NewRequest(http.MethodGet, "/api/map-snapshot", nil))
	require.Equal(t, http.StatusOK, getRecorder.Code)
	assert.Equal(t, "no-store", getRecorder.Header().Get("Cache-Control"))
	assert.Contains(t, getRecorder.Body.String(), `"interval_minutes":60`)
	assert.Contains(t, getRecorder.Body.String(), `"enabled":true`)
	assert.Contains(t, getRecorder.Body.String(), `"automatic_only_when_no_players":false`)
	assert.Contains(t, getRecorder.Body.String(), `"include_space_platforms":true`)
	assert.Contains(t, getRecorder.Body.String(), `"view_bounds_available":true`)
	assert.Contains(t, getRecorder.Body.String(), `"pixels_per_tile":1`)

	refreshRecorder := httptest.NewRecorder()
	RefreshMapSnapshot(refreshRecorder, httptest.NewRequest(http.MethodPost, "/api/map-snapshot/refresh", nil))
	require.Equal(t, http.StatusAccepted, refreshRecorder.Code)
	assert.Contains(t, refreshRecorder.Body.String(), `"running":true`)

	settingsRecorder := httptest.NewRecorder()
	UpdateMapSnapshotSettings(settingsRecorder, httptest.NewRequest(http.MethodPut, "/api/map-snapshot/settings", strings.NewReader(`{"enabled":false,"interval_minutes":135,"automatic_only_when_no_players":true,"include_space_platforms":false}`)))
	require.Equal(t, http.StatusOK, settingsRecorder.Code)
	assert.JSONEq(t, `{"enabled":false,"interval_minutes":135,"automatic_only_when_no_players":true,"include_space_platforms":false}`, settingsRecorder.Body.String())

	legacySettingsRecorder := httptest.NewRecorder()
	UpdateMapSnapshotSettings(legacySettingsRecorder, httptest.NewRequest(http.MethodPut, "/api/map-snapshot/settings", strings.NewReader(`{"interval_minutes":90}`)))
	require.Equal(t, http.StatusOK, legacySettingsRecorder.Code)
	assert.JSONEq(t, `{"enabled":false,"interval_minutes":90,"automatic_only_when_no_players":true,"include_space_platforms":false}`, legacySettingsRecorder.Body.String())

	imageRecorder := httptest.NewRecorder()
	imageRequest := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/map-snapshot/surfaces/1", nil), map[string]string{"surface": "1"})
	GetMapSnapshotImage(imageRecorder, imageRequest)
	require.Equal(t, http.StatusOK, imageRecorder.Code)
	assert.Equal(t, "image/png", imageRecorder.Header().Get("Content-Type"))
	assert.Equal(t, "png", imageRecorder.Body.String())

	entitiesRecorder := httptest.NewRecorder()
	entitiesRequest := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/map-snapshot/surfaces/1/entities", nil), map[string]string{"surface": "1"})
	GetMapSnapshotEntities(entitiesRecorder, entitiesRequest)
	require.Equal(t, http.StatusOK, entitiesRecorder.Code)
	assert.Equal(t, "application/x-ndjson", entitiesRecorder.Header().Get("Content-Type"))
	assert.Contains(t, entitiesRecorder.Body.String(), "assembling-machine-3")
}

func TestMapSnapshotHandlersValidateRequestsAndErrors(t *testing.T) {
	originalTrigger := triggerMapSnapshot
	originalLoad := loadMapSnapshotSettings
	originalSet := setMapSnapshotSettings
	originalRead := readMapSnapshotImage
	originalReadEntities := readMapSnapshotEntities
	t.Cleanup(func() {
		triggerMapSnapshot = originalTrigger
		loadMapSnapshotSettings = originalLoad
		setMapSnapshotSettings = originalSet
		readMapSnapshotImage = originalRead
		readMapSnapshotEntities = originalReadEntities
	})
	loadMapSnapshotSettings = func() (factorio.MapSnapshotSettings, error) {
		return factorio.MapSnapshotSettings{Enabled: true, IntervalMinutes: 60, AutomaticOnlyWhenNoPlayers: false, IncludeSpacePlatforms: true}, nil
	}

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

	triggerMapSnapshot = func() (factorio.MapSnapshotState, error) {
		return factorio.MapSnapshotState{}, factorio.ErrMapSnapshotsDisabled
	}
	disabledRecorder := httptest.NewRecorder()
	RefreshMapSnapshot(disabledRecorder, httptest.NewRequest(http.MethodPost, "/api/map-snapshot/refresh", nil))
	assert.Equal(t, http.StatusConflict, disabledRecorder.Code)

	readMapSnapshotImage = func(string) ([]byte, time.Time, error) {
		return nil, time.Time{}, errors.New("disk error")
	}
	imageRecorder := httptest.NewRecorder()
	imageRequest := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/map-snapshot/surfaces/1", nil), map[string]string{"surface": "1"})
	GetMapSnapshotImage(imageRecorder, imageRequest)
	assert.Equal(t, http.StatusInternalServerError, imageRecorder.Code)

	readMapSnapshotEntities = func(string) ([]byte, time.Time, error) {
		return nil, time.Time{}, factorio.ErrMapSnapshotDetailsNotFound
	}
	entitiesRecorder := httptest.NewRecorder()
	entitiesRequest := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/map-snapshot/surfaces/1/entities", nil), map[string]string{"surface": "1"})
	GetMapSnapshotEntities(entitiesRecorder, entitiesRequest)
	assert.Equal(t, http.StatusNotFound, entitiesRecorder.Code)
}
