package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/stretchr/testify/assert"
)

func TestInstallFactorioReleaseRejectsUnsupportedChannel(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/server/release/install", strings.NewReader(`{"channel":"nightly"}`))

	InstallFactorioRelease(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "unsupported Factorio release target")
}

func TestInstallFactorioReleaseRejectsMalformedRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/server/release/install", strings.NewReader(`{`))

	InstallFactorioRelease(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Unable to parse the request body")
}

func TestInstallFactorioReleaseRejectsRunningServer(t *testing.T) {
	server := factorio.GetFactorioServer()
	wasRunning := server.Running
	server.Running = true
	t.Cleanup(func() { server.Running = wasRunning })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/server/release/install", strings.NewReader(`{"channel":"stable"}`))

	InstallFactorioRelease(recorder, request)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Stop the Factorio server")
}
