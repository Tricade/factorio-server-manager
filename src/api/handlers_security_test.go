package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadRequestBodyRejectsOversizedJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(strings.Repeat("x", int(maximumJSONRequestBodyBytes)+1)))
	body, _, err := ReadRequestBody(recorder, request)
	require.Error(t, err)
	assert.Nil(t, body)
	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestMultipartRequestLimitAllowsEncodingOverhead(t *testing.T) {
	assert.Equal(t, int64(21*1024*1024), maximumMultipartRequestSize(20*1024*1024))
	assert.Equal(t, "Save file exceeds the configured upload limit of 20 MiB", uploadLimitMessage("Save file", 20*1024*1024))
}

func TestServerSettingsForViewerRedactsCredentials(t *testing.T) {
	settings := map[string]interface{}{
		"name":                   "Public factory",
		"max_players":            float64(16),
		"visibility":             map[string]interface{}{"public": true},
		"username":               "factorio-account",
		"password":               "account-password",
		"token":                  "account-token",
		"game_password":          "multiplayer-password",
		"_comment_game_password": "sensitive field description",
		"api_key":                "future-sensitive-value",
	}

	viewer := serverSettingsForRequest(settings, false)
	assert.Equal(t, "Public factory", viewer["name"])
	assert.Equal(t, float64(16), viewer["max_players"])
	assert.Contains(t, viewer, "visibility")
	for _, key := range []string{"username", "password", "token", "game_password", "_comment_game_password", "api_key"} {
		assert.NotContains(t, viewer, key)
	}

	administratorSettings := map[string]interface{}{"token": "administrator-visible-token"}
	assert.Contains(t, serverSettingsForRequest(administratorSettings, true), "token")
}

func TestConfigForViewerRedactsSecretsAndFilesystemLocations(t *testing.T) {
	config := map[string]map[string]string{
		"path": {
			"read-data":  "/private/read-data",
			"write-data": "/private/write-data",
		},
		"general": {
			"locale":                "en",
			"rcon-password":         "rcon-secret",
			"cookie_encryption_key": "cookie-secret",
			"proxy":                 "https://user:password@example.invalid",
			"log-file":              "/private/factorio.log",
		},
	}

	viewer := configForRequest(config, false)
	assert.NotContains(t, viewer, "path")
	assert.Equal(t, "en", viewer["general"]["locale"])
	for _, key := range []string{"rcon-password", "cookie_encryption_key", "proxy", "log-file"} {
		assert.NotContains(t, viewer["general"], key)
	}

	administratorConfig := map[string]map[string]string{"general": {"token": "administrator-visible-token"}}
	assert.Contains(t, configForRequest(administratorConfig, true)["general"], "token")
}

func TestReleaseStatusForViewerHidesInternalMetadataErrors(t *testing.T) {
	status := factorio.ReleaseStatus{InstalledVersion: "2.0.72", MetadataError: `open C:\private\runtime-state.json: access denied`}
	viewer := releaseStatusForRequest(status, false)
	assert.Equal(t, "2.0.72", viewer.InstalledVersion)
	assert.Equal(t, "Official release metadata is temporarily unavailable.", viewer.MetadataError)
	assert.Equal(t, status.MetadataError, releaseStatusForRequest(status, true).MetadataError)
}
