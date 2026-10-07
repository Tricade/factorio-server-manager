package api

import (
	"context"
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

func TestModPackMergeHandlersValidationAndErrors(t *testing.T) {
	originalMerge, originalPreview := mergeModPack, previewModPackMerge
	t.Cleanup(func() { mergeModPack, previewModPackMerge = originalMerge, originalPreview })
	calls := 0
	mergeModPack = func(name, revision string) (factorio.ModPackMergePlan, error) {
		calls++
		assert.Equal(t, "QoL", name)
		assert.Equal(t, strings.Repeat("a", 64), revision)
		return factorio.ModPackMergePlan{Revision: revision}, nil
	}
	for _, body := range []string{"{}", `{"revision":null}`, `{"revision":1}`, `{"revision":"a","extra":true}`, `{"revision":"a"} {}`, `{"revision":"` + strings.Repeat("a", 4096) + `"}`} {
		recorder := httptest.NewRecorder()
		MergeModPackHandler(recorder, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	}
	assert.Zero(t, calls)
	recorder := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest("POST", "/", strings.NewReader(`{"revision":"`+strings.Repeat("a", 64)+`"}`)), map[string]string{"modpack": "QoL"})
	MergeModPackHandler(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, 1, calls)
	for _, test := range []struct {
		err    error
		status int
	}{
		{factorio.ErrServerActive, 423}, {factorio.ErrModPackMergeStale, 409}, {factorio.ErrModPackMergeConflict, 409}, {factorio.ErrInvalidModPackMerge, 400}, {errors.New("SECRET private path"), 422},
	} {
		previewModPackMerge = func(string) (factorio.ModPackMergePlan, error) { return factorio.ModPackMergePlan{}, test.err }
		recorder := httptest.NewRecorder()
		PreviewModPackMergeHandler(recorder, httptest.NewRequest("POST", "/", nil))
		assert.Equal(t, test.status, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "SECRET")
	}
}

func TestModPackMergeRoutesRequireAdminStoppedAndOwnProfileLock(t *testing.T) {
	for _, name := range []string{"PreviewModPackMerge", "MergeModPack", "UpdateModPortalPreferences"} {
		route, found := findAPIRoute(name)
		require.True(t, found)
		assert.True(t, routeRequiresAdministrator(route))
		assert.Equal(t, name != "UpdateModPortalPreferences", route.ServerOff)
		request := httptest.NewRequest(route.Method, "/api"+route.Pattern, strings.NewReader("{}"))
		request = request.WithContext(context.WithValue(request.Context(), authenticatedUserContextKey{}, User{Username: "viewer", Role: UserRoleViewer}))
		recorder := httptest.NewRecorder()
		RequireAdministrator(route.HandlerFunc).ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		router := mux.NewRouter()
		router.Path("/api" + route.Pattern).Methods(route.Method).Handler(route.HandlerFunc)
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("DELETE", "/api"+route.Pattern, nil))
		assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	}
	for _, path := range []string{"/api/mods/packs/QoL/merge/preview", "/api/mods/packs/QoL/merge"} {
		done := make(chan struct{})
		handler := ProfileDataMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// The real operation acquires the exclusive gate itself.
			_, _ = factorio.PreviewModPackMerge("../invalid")
			close(done)
		}))
		go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", path, nil))
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("merge route recursively acquired profile lock")
		}
	}
}
