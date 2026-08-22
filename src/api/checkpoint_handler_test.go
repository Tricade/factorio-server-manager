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

func TestCheckpointListSettingsAndCreateHandlers(t *testing.T) {
	originalGet := getCheckpointState
	originalUpdate := updateCheckpointSettings
	originalCreate := createCheckpoint
	current := factorio.CheckpointState{
		ProfileID:   "0123456789abcdef",
		Settings:    factorio.CheckpointSettings{IntervalMinutes: 30, RetentionCount: 10},
		Checkpoints: []factorio.Checkpoint{},
	}
	getCheckpointState = func() (factorio.CheckpointState, error) { return current, nil }
	updateCheckpointSettings = func(settings factorio.CheckpointSettings) (factorio.CheckpointState, error) {
		current.Settings = settings
		return current, nil
	}
	createCheckpoint = func(trigger string) (factorio.CheckpointState, error) {
		require.Equal(t, factorio.CheckpointTriggerManual, trigger)
		current.Checkpoints = append(current.Checkpoints, factorio.Checkpoint{ID: "checkpoint", Trigger: trigger, CreatedAt: time.Now()})
		return current, nil
	}
	t.Cleanup(func() {
		getCheckpointState = originalGet
		updateCheckpointSettings = originalUpdate
		createCheckpoint = originalCreate
	})

	listRecorder := httptest.NewRecorder()
	ListCheckpoints(listRecorder, httptest.NewRequest(http.MethodGet, "/api/checkpoints", nil))
	require.Equal(t, http.StatusOK, listRecorder.Code)
	assert.Equal(t, "no-store", listRecorder.Header().Get("Cache-Control"))
	assert.Contains(t, listRecorder.Body.String(), `"retention_count":10`)

	settingsRecorder := httptest.NewRecorder()
	SaveCheckpointSettings(settingsRecorder, httptest.NewRequest(http.MethodPut, "/api/checkpoints/settings", strings.NewReader(`{
		"interval_enabled":true,
		"interval_minutes":60,
		"last_player_enabled":true,
		"clean_stop_enabled":true,
		"retention_count":20
	}`)))
	require.Equal(t, http.StatusOK, settingsRecorder.Code)
	assert.True(t, current.Settings.CleanStopEnabled)
	assert.Equal(t, 20, current.Settings.RetentionCount)

	createRecorder := httptest.NewRecorder()
	CreateCheckpoint(createRecorder, httptest.NewRequest(http.MethodPost, "/api/checkpoints", nil))
	require.Equal(t, http.StatusCreated, createRecorder.Code)
	assert.Contains(t, createRecorder.Body.String(), `"trigger":"manual"`)
}

func TestCheckpointHandlersValidateRequestsAndMapErrors(t *testing.T) {
	originalRestore := restoreCheckpoint
	originalDelete := deleteCheckpoint
	restoreCheckpoint = func(string) (factorio.Save, error) { return factorio.Save{}, factorio.ErrCheckpointServerActive }
	deleteCheckpoint = func(string) (factorio.CheckpointState, error) {
		return factorio.CheckpointState{}, factorio.ErrCheckpointNotFound
	}
	t.Cleanup(func() {
		restoreCheckpoint = originalRestore
		deleteCheckpoint = originalDelete
	})

	invalidRecorder := httptest.NewRecorder()
	SaveCheckpointSettings(invalidRecorder, httptest.NewRequest(http.MethodPut, "/api/checkpoints/settings", strings.NewReader(`{"unexpected":true}`)))
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)

	restoreRecorder := httptest.NewRecorder()
	restoreRequest := mux.SetURLVars(httptest.NewRequest(http.MethodPost, "/api/checkpoints/id/restore", nil), map[string]string{"checkpoint": "id"})
	RestoreCheckpoint(restoreRecorder, restoreRequest)
	assert.Equal(t, http.StatusLocked, restoreRecorder.Code)

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := mux.SetURLVars(httptest.NewRequest(http.MethodDelete, "/api/checkpoints/id", nil), map[string]string{"checkpoint": "id"})
	DeleteCheckpoint(deleteRecorder, deleteRequest)
	assert.Equal(t, http.StatusNotFound, deleteRecorder.Code)

	originalGet := getCheckpointState
	getCheckpointState = func() (factorio.CheckpointState, error) {
		return factorio.CheckpointState{}, errors.New("disk failure")
	}
	t.Cleanup(func() { getCheckpointState = originalGet })
	listRecorder := httptest.NewRecorder()
	ListCheckpoints(listRecorder, httptest.NewRequest(http.MethodGet, "/api/checkpoints", nil))
	assert.Equal(t, http.StatusInternalServerError, listRecorder.Code)
}
