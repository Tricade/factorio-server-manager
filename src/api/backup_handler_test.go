package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackupUploadLimitsAreAdminOnlyAndReadOnly(t *testing.T) {
	route, ok := findAPIRoute("BackupUploadLimits")
	require.True(t, ok)
	assert.Equal(t, http.MethodGet, route.Method)
	assert.False(t, route.ServerOff, "size metadata must also be available while Factorio runs")
	assert.True(t, routeRequiresAdministrator(route))
	router := mux.NewRouter()
	router.Handle(route.Pattern, RequireAdministrator(route.HandlerFunc)).Methods(route.Method)
	request := httptest.NewRequest(http.MethodGet, route.Pattern, nil)
	request = request.WithContext(context.WithValue(request.Context(), authenticatedUserContextKey{}, User{Username: "viewer", Role: UserRoleViewer}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	request = request.WithContext(context.WithValue(request.Context(), authenticatedUserContextKey{}, User{Username: "admin", Role: UserRoleAdmin}))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var data map[string]int64
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &data))
	require.Len(t, data, 2, "only size metadata, never manager configuration")
	assert.Equal(t, bootstrap.GetConfig().MaxUploadSize, data["max_upload_bytes"])
	assert.Equal(t, data["max_upload_bytes"]-maxProfileRequestSize-(4<<10), data["max_file_bytes"])
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, route.Pattern, nil))
	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
}

type backupUnreadBody struct{ read bool }

func (body *backupUnreadBody) Read([]byte) (int, error) { body.read = true; return 0, io.EOF }
func (*backupUnreadBody) Close() error                  { return nil }

func TestBackupRejectsOversizedContentLengthBeforeReading(t *testing.T) {
	for _, handler := range []http.HandlerFunc{PreviewProfileBackupHandler, ImportProfileBackupHandler, PreviewModBackupHandler, RestoreModBackupHandler} {
		body := &backupUnreadBody{}
		request := httptest.NewRequest(http.MethodPost, "/", body)
		request.ContentLength = bootstrap.GetConfig().MaxUploadSize + 1
		request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "FSM_MAX_UPLOAD")
		assert.Contains(t, recorder.Body.String(), "MiB")
		assert.False(t, body.read, "oversized requests must not fill temporary storage")
	}
}

func TestEffectiveBackupUploadLimitsPreserveConfiguredRequestCap(t *testing.T) {
	for _, tc := range []struct{ configured, want int64 }{
		{0, 512 << 20}, {-1, 512 << 20}, {512 << 20, 512 << 20},
		{2048 << 20, 2048 << 20}, {16 << 30, 16 << 30}, {32 << 30, 16 << 30}, {1, 1},
	} {
		limits := effectiveBackupUploadLimits(tc.configured)
		assert.Equal(t, tc.want, limits.MaxUploadBytes)
		assert.Equal(t, max(int64(0), tc.want-backupMultipartReserve), limits.MaxFileBytes)
	}
}

func TestBackupUploadStillAcceptsUnknownLengthMultipart(t *testing.T) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("backup", "backup.zip")
	require.NoError(t, err)
	_, err = part.Write([]byte("upload parser fixture"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/", &buffer)
	request.ContentLength = -1
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	file, size, cleanup, ok := backupUpload(recorder, request)
	defer cleanup()
	require.True(t, ok)
	assert.Equal(t, int64(len("upload parser fixture")), size)
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, "upload parser fixture", string(data))
}

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
