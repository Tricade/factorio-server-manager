package api

import (
	"archive/zip"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadSaveAcceptsArchiveAbovePreviousDefaultLimit(t *testing.T) {
	const archiveContentsSize = int64(21 * 1024 * 1024)
	archivePath := filepath.Join(t.TempDir(), "existing-save.zip")
	archive, err := os.Create(archivePath)
	require.NoError(t, err)
	zipWriter := zip.NewWriter(archive)
	header := &zip.FileHeader{Name: "existing-save/level.dat", Method: zip.Store}
	entry, err := zipWriter.CreateHeader(header)
	require.NoError(t, err)
	_, err = io.CopyN(entry, zeroReader{}, archiveContentsSize)
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())
	require.NoError(t, archive.Close())

	requestReader, requestWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(requestWriter)
	request := httptest.NewRequest(http.MethodPost, "/api/saves/upload", requestReader)
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	writeResult := make(chan error, 1)
	go func() {
		part, writeErr := multipartWriter.CreateFormFile("savefile", "existing-save.zip")
		if writeErr == nil {
			var source *os.File
			source, writeErr = os.Open(archivePath)
			if writeErr == nil {
				_, writeErr = io.Copy(part, source)
				if closeErr := source.Close(); writeErr == nil {
					writeErr = closeErr
				}
			}
		}
		if closeErr := multipartWriter.Close(); writeErr == nil {
			writeErr = closeErr
		}
		_ = requestWriter.CloseWithError(writeErr)
		writeResult <- writeErr
	}()

	destination := filepath.Join(bootstrap.GetConfig().FactorioSavesDir, "existing-save.zip")
	t.Cleanup(func() { _ = os.Remove(destination) })
	recorder := httptest.NewRecorder()
	UploadSave(recorder, request)
	require.NoError(t, <-writeResult)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "Uploading files successful")
	require.NoError(t, factorio.ValidateSaveArchive(destination))
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}
