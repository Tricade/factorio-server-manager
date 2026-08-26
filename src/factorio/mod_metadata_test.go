package factorio

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModInfoReturnsInvalidJSONError(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	info, err := writer.Create("broken-mod/info.json")
	require.NoError(t, err)
	_, err = info.Write([]byte(`{"name":`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	require.NoError(t, err)
	var metadata ModInfo
	err = metadata.getModInfo(reader)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid info.json")
}

func TestCreateModRejectsPathFilename(t *testing.T) {
	list := ModInfoList{Destination: t.TempDir()}
	err := list.createMod("example", "../example.zip", strings.NewReader("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mod filename")
}

func TestCreateModClassifiesArchiveIdentityMismatch(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	info, err := writer.Create("different-mod/info.json")
	require.NoError(t, err)
	_, err = info.Write([]byte(`{"name":"different-mod","version":"1.0.1"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	list := ModInfoList{Destination: t.TempDir()}
	err = list.createMod("requested-mod", "requested-mod_1.0.1.zip", bytes.NewReader(archive.Bytes()))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errModArchiveIdentityMismatch))
}

func TestDownloadModClassifiesMissingPortalArchive(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/download/example/release", r.URL.Path)
		assert.Equal(t, "portal-user", r.URL.Query().Get("username"))
		assert.Equal(t, "portal-key", r.URL.Query().Get("token"))
		w.WriteHeader(http.StatusGone)
	}))
	defer portal.Close()

	originalBaseURL := modPortalBaseURL
	modPortalBaseURL = portal.URL
	defer func() { modPortalBaseURL = originalBaseURL }()
	originalCredentialsFilePath := credentialsFilePath
	credentialsPath := filepath.Join(t.TempDir(), "factorio.auth")
	require.NoError(t, os.WriteFile(credentialsPath, []byte(`{"username":"portal-user","userkey":"portal-key"}`), 0600))
	credentialsFilePath = func() string { return credentialsPath }
	defer func() { credentialsFilePath = originalCredentialsFilePath }()

	mods, err := NewMods(t.TempDir())
	require.NoError(t, err)
	err = mods.DownloadMod("/download/example/release", "example_1.0.0.zip", "example")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errModPortalReleaseUnavailable))
}
