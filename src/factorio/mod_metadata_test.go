package factorio

import (
	"archive/zip"
	"bytes"
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
