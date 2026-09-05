package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormerMutatingGETRoutesRejectGET(t *testing.T) {
	expected := map[string]string{
		"/api/server/stop":            http.MethodPost,
		"/api/server/kill":            http.MethodPost,
		"/api/saves/rm/world.zip":     http.MethodDelete,
		"/api/saves/create/world.zip": http.MethodPost,
		"/api/logout":                 http.MethodPost,
		"/api/mods/portal/logout":     http.MethodDelete,
	}
	router := mux.NewRouter()
	for _, route := range apiRoutes {
		router.Path("/api" + route.Pattern).Methods(route.Method).HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}
	for path, method := range expected {
		t.Run(path, func(t *testing.T) {
			getRecorder := httptest.NewRecorder()
			router.ServeHTTP(getRecorder, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusMethodNotAllowed, getRecorder.Code)

			methodRecorder := httptest.NewRecorder()
			router.ServeHTTP(methodRecorder, httptest.NewRequest(method, path, nil))
			assert.Equal(t, http.StatusNoContent, methodRecorder.Code)
		})
	}
}

func TestNewRouterReturnsMethodNotAllowedForFormerMutatingGETs(t *testing.T) {
	originalStore := sessionStore
	originalAuth := auth
	originalCredentialKey := sessionCredentialKey
	sessionStore = sessions.NewCookieStore(bytes.Repeat([]byte{0x55}, 32))
	sessionCredentialKey = bytes.Repeat([]byte{0x56}, 32)
	auth = newAuthorizationTestAuth(t)
	require.NoError(t, auth.db.Create(&User{Username: "admin", Password: "hash", Role: UserRoleAdmin}).Error)
	seedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	seedRecorder := httptest.NewRecorder()
	session, err := sessionStore.Get(seedRequest, "authentication")
	require.NoError(t, err)
	session.Values["username"] = "admin"
	credentialVersion, err := sessionCredentialVersion("hash")
	require.NoError(t, err)
	session.Values["credential_version"] = credentialVersion
	require.NoError(t, session.Save(seedRequest, seedRecorder))
	cookie := seedRecorder.Result().Cookies()[0]
	t.Cleanup(func() {
		sessionStore = originalStore
		auth = originalAuth
		sessionCredentialKey = originalCredentialKey
	})
	router := NewRouter()
	for _, path := range []string{
		"/api/server/stop", "/api/server/kill", "/api/saves/rm/world.zip",
		"/api/saves/create/world.zip", "/api/logout", "/api/mods/portal/logout",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(cookie)
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code, path)
	}
	startupRecorder := httptest.NewRecorder()
	startupRequest := httptest.NewRequest(http.MethodPost, "/api/mods/startup-settings", nil)
	startupRequest.AddCookie(cookie)
	router.ServeHTTP(startupRecorder, startupRequest)
	assert.Equal(t, http.StatusMethodNotAllowed, startupRecorder.Code)
}

func TestRouteAdministratorPolicyIsDefaultDenyForMutations(t *testing.T) {
	for _, route := range apiRoutes {
		method := route.Method
		unsafe := method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
		if unsafe && route.Name != "LogoutUser" && route.Name != "ChangePassword" {
			assert.Truef(t, routeRequiresAdministrator(route), "%s %s must require admin", method, route.Pattern)
		}
	}
	for _, name := range []string{"StartServer", "StopServer", "KillServer", "InstallFactorioRelease", "AddUser", "RemoveUser", "CreateProfile", "UploadMod"} {
		route, ok := findAPIRoute(name)
		require.True(t, ok, name)
		assert.True(t, routeRequiresAdministrator(route), name)
	}
	for _, name := range []string{"LogoutUser", "ChangePassword"} {
		route, ok := findAPIRoute(name)
		require.True(t, ok, name)
		assert.False(t, routeRequiresAdministrator(route), name)
	}
	listUsers, ok := findAPIRoute("ListUsers")
	require.True(t, ok)
	assert.True(t, routeRequiresAdministrator(listUsers))
	startupSettings, ok := findAPIRoute("GetModStartupSettings")
	require.True(t, ok)
	assert.True(t, routeRequiresAdministrator(startupSettings))
}

func TestRequireAdministratorRoleMatrix(t *testing.T) {
	handler := RequireAdministrator(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		name string
		role string
		want int
	}{
		{name: "admin", role: UserRoleAdmin, want: http.StatusNoContent},
		{name: "viewer", role: UserRoleViewer, want: http.StatusForbidden},
		{name: "legacy user", role: "user", want: http.StatusForbidden},
		{name: "missing principal", role: "", want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/action", nil)
			if test.role != "" {
				ctx := context.WithValue(request.Context(), authenticatedUserContextKey{}, User{Username: "tester", Role: test.role})
				request = request.WithContext(ctx)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assert.Equal(t, test.want, recorder.Code)
		})
	}
}

func TestSecurityHeadersProtectUIAndAPI(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/", "/api/server/status"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
		assert.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
		assert.Contains(t, recorder.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
		assert.Contains(t, recorder.Header().Get("Content-Security-Policy"), "connect-src 'self' ws: wss:")
		if path == "/api/server/status" {
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		}
	}
}

func findAPIRoute(name string) (Route, bool) {
	for _, route := range apiRoutes {
		if route.Name == name {
			return route, true
		}
	}
	return Route{}, false
}
