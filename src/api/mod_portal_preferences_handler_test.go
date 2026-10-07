package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/stretchr/testify/assert"
)

func TestModPortalPreferencesHandlers(t *testing.T) {
	originalLoad, originalSet := loadModPortalPreferences, setModPortalPreferences
	t.Cleanup(func() { loadModPortalPreferences, setModPortalPreferences = originalLoad, originalSet })
	current := factorio.ModPortalPreferences{}
	loadModPortalPreferences = func() (factorio.ModPortalPreferences, error) { return current, nil }
	setModPortalPreferences = func(value factorio.ModPortalPreferences) (factorio.ModPortalPreferences, error) {
		current = value
		return current, nil
	}
	recorder := httptest.NewRecorder()
	GetModPortalPreferences(recorder, httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"preselect_optional":false}`, recorder.Body.String())
	for _, body := range []string{"{}", "null", `{"preselect_optional":null}`, `{"preselect_optional":"true"}`, `{"preselect_optional":true,"extra":0}`, `{"preselect_optional":true} {}`, strings.Repeat(" ", 4097)} {
		recorder = httptest.NewRecorder()
		UpdateModPortalPreferences(recorder, httptest.NewRequest("PUT", "/", strings.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.False(t, current.PreselectOptional)
	}
	recorder = httptest.NewRecorder()
	UpdateModPortalPreferences(recorder, httptest.NewRequest("PUT", "/", strings.NewReader(`{"preselect_optional":true}`)))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, current.PreselectOptional)
	loadModPortalPreferences = func() (factorio.ModPortalPreferences, error) { return current, errors.New("SECRET") }
	setModPortalPreferences = func(factorio.ModPortalPreferences) (factorio.ModPortalPreferences, error) {
		return current, errors.New("SECRET")
	}
	for _, handler := range []http.HandlerFunc{GetModPortalPreferences, UpdateModPortalPreferences} {
		recorder = httptest.NewRecorder()
		handler(recorder, httptest.NewRequest("PUT", "/", strings.NewReader(`{"preselect_optional":false}`)))
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "SECRET")
	}
}
