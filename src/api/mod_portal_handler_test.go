package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModPortalLoginStatusReturnsBooleanWithoutCredentialDetails(t *testing.T) {
	original := loadModPortalAuthenticationStatus
	t.Cleanup(func() { loadModPortalAuthenticationStatus = original })

	loadModPortalAuthenticationStatus = func() (bool, error) { return true, nil }
	recorder := httptest.NewRecorder()
	ModPortalLoginStatusHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/mods/portal/loginstatus", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "true\n" {
		t.Fatalf("unexpected successful login-status response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}

	loadModPortalAuthenticationStatus = func() (bool, error) {
		return false, errors.New(`open C:\\private\\factorio.auth: access denied`)
	}
	recorder = httptest.NewRecorder()
	ModPortalLoginStatusHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/mods/portal/loginstatus", nil))
	if recorder.Code != http.StatusInternalServerError || recorder.Body.String() != "\"Unable to read Factorio mod portal login status\"\n" {
		t.Fatalf("credential error leaked into login-status response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestModPortalInstallHandler(t *testing.T) {
	CheckShort(t)

	method := "POST"
	route := "/api/mods/portal/install"
	handlerFunc := ModPortalInstallHandler

	t.Run("success", func(t *testing.T) {
		SetupMods(t, true)
		defer CleanupMods(t)

		requestBody := strings.NewReader(`{"modName": "belt-balancer", "downloadUrl": "/download/belt-balancer/5fc1aca2bfe1b005c6943bf1", "fileName": "belt-balancer_3.0.0.zip"}`)

		expected := `{"mods":[{"name":"belt-balancer","version":"3.0.0","title":"Belt Balancer","author":"knoxfighter","file_name":"belt-balancer_3.0.0.zip","factorio_version":"1.1.0.0","dependencies":null,"compatibility":true,"enabled":true}]}`

		CallRoute(t, method, route, route, requestBody, handlerFunc, http.StatusOK, expected)
	})

	ModEmptyBodyTest(t, method, route, handlerFunc)

	ModInvalidJsonTest(t, method, route, handlerFunc)

	t.Run("wrong download link", func(t *testing.T) {
		SetupMods(t, true)
		defer CleanupMods(t)

		requestBody := strings.NewReader(`{"modName": "belt-balancer", "downloadUrl": "/download/belt-balancer/95bcf4f000b96b22c", "fileName": "belt-balancer_3.0.0.zip"}`)

		CallRoute(t, method, route, route, requestBody, handlerFunc, http.StatusInternalServerError, "")
	})
}

func TestModPortalInstallMultipleHandler(t *testing.T) {
	CheckShort(t)

	method := "POST"
	route := "/api/mods/portal/install/multiple"
	handlerFunc := ModPortalInstallMultipleHandler

	t.Run("success", func(t *testing.T) {
		SetupMods(t, true)
		defer CleanupMods(t)

		requestBody := strings.NewReader(`[{"name": "belt-balancer", "version": "3.0.0"}, {"name": "train-station-overview", "version": "3.0.0"}]`)

		expected := `{"mods":[{"name":"belt-balancer","version":"3.0.0","title":"Belt Balancer","author":"knoxfighter","file_name":"belt-balancer_3.0.0.zip","factorio_version":"1.1.0.0","dependencies":null,"compatibility":true,"enabled":true},{"name":"train-station-overview","version":"3.0.0","title":"Train Station Overview","author":"knoxfighter","file_name":"train-station-overview_3.0.0.zip","factorio_version":"1.1.0.0","dependencies":null,"compatibility":true,"enabled":true}]}`

		CallRoute(t, method, route, route, requestBody, handlerFunc, http.StatusOK, expected)
	})

	t.Run("unknown mod", func(t *testing.T) {
		SetupMods(t, true)
		defer CleanupMods(t)

		requestBody := strings.NewReader(`[{"name": "askdhcb", "version": "2.1.2"}]`)

		CallRoute(t, method, route, route, requestBody, handlerFunc, http.StatusInternalServerError, "")
	})

	t.Run("unknown version", func(t *testing.T) {
		SetupMods(t, true)
		defer CleanupMods(t)

		requestBody := strings.NewReader(`[{"name": "belt-balancer", "version": "0.1.12"}]`)

		CallRoute(t, method, route, route, requestBody, handlerFunc, http.StatusInternalServerError, "")
	})

	ModEmptyBodyTest(t, method, route, handlerFunc)

	ModInvalidJsonTest(t, method, route, handlerFunc)
}
