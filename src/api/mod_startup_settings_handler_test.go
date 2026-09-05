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

func TestGetModStartupSettingsHandlerReturnsNoStoreAdminData(t *testing.T) {
	original := getModStartupSettings
	getModStartupSettings = func() (factorio.ModStartupSettingsView, error) {
		return factorio.ModStartupSettingsView{
			ProfileID: "0123456789abcdef", ProfileName: "Test", FactorioVersion: "2.0.77",
			Revision: strings.Repeat("a", 64), Groups: []factorio.ModStartupSettingsGroup{},
		}, nil
	}
	t.Cleanup(func() { getModStartupSettings = original })

	recorder := httptest.NewRecorder()
	GetModStartupSettingsHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/mods/startup-settings", nil))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"profile_id":"0123456789abcdef","profile_name":"Test","factorio_version":"2.0.77","revision":"`+strings.Repeat("a", 64)+`","groups":[]}`, recorder.Body.String())
}

func TestUpdateModStartupSettingsHandlerDecodesBoundedChanges(t *testing.T) {
	original := updateModStartupSettings
	updateModStartupSettings = func(request factorio.ModStartupSettingsUpdate) (factorio.ModStartupSettingsView, error) {
		require.Equal(t, strings.Repeat("b", 64), request.Revision)
		require.Len(t, request.Changes, 1)
		assert.Equal(t, "fixture-enabled", request.Changes[0].Name)
		assert.JSONEq(t, "false", string(request.Changes[0].Value))
		return factorio.ModStartupSettingsView{Revision: strings.Repeat("c", 64), Groups: []factorio.ModStartupSettingsGroup{}}, nil
	}
	t.Cleanup(func() { updateModStartupSettings = original })
	body := `{"revision":"` + strings.Repeat("b", 64) + `","changes":[{"name":"fixture-enabled","value":false}]}`
	recorder := httptest.NewRecorder()
	UpdateModStartupSettingsHandler(recorder, httptest.NewRequest(http.MethodPatch, "/api/mods/startup-settings", strings.NewReader(body)))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), strings.Repeat("c", 64))
}

func TestUpdateModStartupSettingsHandlerRejectsUnknownAndOversizedRequests(t *testing.T) {
	for name, test := range map[string]struct {
		body     string
		expected int
	}{
		"unknown field": {body: `{"revision":"` + strings.Repeat("a", 64) + `","changes":[],"extra":true}`, expected: http.StatusBadRequest},
		"trailing JSON": {body: `{"revision":"` + strings.Repeat("a", 64) + `","changes":[]} {}`, expected: http.StatusBadRequest},
		"oversized":     {body: strings.Repeat("x", int(maximumJSONRequestBodyBytes)+1), expected: http.StatusRequestEntityTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			UpdateModStartupSettingsHandler(recorder, httptest.NewRequest(http.MethodPatch, "/api/mods/startup-settings", strings.NewReader(test.body)))
			assert.Equal(t, test.expected, recorder.Code)
		})
	}
}

func TestModStartupSettingsErrorsNeverEchoSensitiveValues(t *testing.T) {
	for name, test := range map[string]struct {
		source error
		status int
	}{
		"stale":   {source: fmtSensitive(factorio.ErrModStartupSettingsStale), status: http.StatusConflict},
		"invalid": {source: fmtSensitive(factorio.ErrInvalidModStartupSettings), status: http.StatusUnprocessableEntity},
		"engine":  {source: errors.New("third-party mod logged super-secret-token"), status: http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeModStartupSettingsError(recorder, test.source)
			assert.Equal(t, test.status, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "super-secret-token")
		})
	}
}

func fmtSensitive(sentinel error) error {
	return errors.Join(sentinel, errors.New("super-secret-token"))
}
