package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutostartSettingsHandlers(t *testing.T) {
	originalLoad := loadAutostartSettings
	originalSet := setAutostartEnabled
	current := factorio.AutostartSettings{Enabled: false}
	loadAutostartSettings = func() (factorio.AutostartSettings, error) { return current, nil }
	setAutostartEnabled = func(enabled bool) (factorio.AutostartSettings, error) {
		current.Enabled = enabled
		return current, nil
	}
	t.Cleanup(func() {
		loadAutostartSettings = originalLoad
		setAutostartEnabled = originalSet
	})

	getRecorder := httptest.NewRecorder()
	GetAutostartSettings(getRecorder, httptest.NewRequest(http.MethodGet, "/api/server/autostart", nil))
	require.Equal(t, http.StatusOK, getRecorder.Code)
	assert.Equal(t, "no-store", getRecorder.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"enabled":false}`, getRecorder.Body.String())

	updateRecorder := httptest.NewRecorder()
	UpdateAutostartSettings(updateRecorder, httptest.NewRequest(http.MethodPut, "/api/server/autostart", strings.NewReader(`{"enabled":true}`)))
	require.Equal(t, http.StatusOK, updateRecorder.Code)
	assert.JSONEq(t, `{"enabled":true}`, updateRecorder.Body.String())
}

func TestUpdateAutostartSettingsValidatesRequestAndSaveErrors(t *testing.T) {
	originalSet := setAutostartEnabled
	t.Cleanup(func() { setAutostartEnabled = originalSet })

	invalidRecorder := httptest.NewRecorder()
	UpdateAutostartSettings(invalidRecorder, httptest.NewRequest(http.MethodPut, "/api/server/autostart", strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)

	setAutostartEnabled = func(bool) (factorio.AutostartSettings, error) {
		return factorio.AutostartSettings{}, errors.New("disk full")
	}
	errorRecorder := httptest.NewRecorder()
	UpdateAutostartSettings(errorRecorder, httptest.NewRequest(http.MethodPut, "/api/server/autostart", strings.NewReader(`{"enabled":true}`)))
	assert.Equal(t, http.StatusInternalServerError, errorRecorder.Code)
}
