package api

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupRoutesAreAdminOnlyPOSTAndStopped(t *testing.T) {
	for _, name := range []string{"ExportProfileBackup", "PreviewProfileBackup", "ImportProfileBackup", "PreviewModBackup", "RestoreModBackup"} {
		route, ok := findAPIRoute(name)
		require.True(t, ok)
		assert.True(t, routeRequiresAdministrator(route))
		assert.True(t, route.ServerOff)
		assert.Equal(t, "POST", route.Method)
		router := mux.NewRouter()
		router.Path("/api" + route.Pattern).Methods(route.Method).Handler(RequireAdministrator(route.HandlerFunc))
		request := httptest.NewRequest("POST", "/api"+route.Pattern, strings.NewReader("{}"))
		request = request.WithContext(context.WithValue(request.Context(), authenticatedUserContextKey{}, User{Username: "viewer", Role: UserRoleViewer}))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("GET", "/api"+route.Pattern, nil))
		assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	}
}

func TestBackupUploadRejectsMalformedDataWithoutEchoingContents(t *testing.T) {
	for _, handler := range []http.HandlerFunc{PreviewProfileBackupHandler, ImportProfileBackupHandler, PreviewModBackupHandler, RestoreModBackupHandler} {
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		part, err := writer.CreateFormFile("backup", "backup.zip")
		require.NoError(t, err)
		_, err = part.Write([]byte("SECRET malformed content"))
		require.NoError(t, err)
		require.NoError(t, writer.WriteField("selection", `[{"id":"0123456789abcdef","name":"test"}]`))
		require.NoError(t, writer.Close())
		request := httptest.NewRequest("POST", "/", &buffer)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "SECRET")
	}
}
