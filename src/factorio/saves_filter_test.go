package factorio

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUsableSave(t *testing.T) {
	tests := []struct {
		name string
		size int
		want bool
	}{
		{name: "world.zip", size: 4, want: true},
		{name: "WORLD.ZIP", size: 4, want: true},
		{name: "world.tmp.zip", size: 4, want: false},
		{name: "world.zip", size: 0, want: false},
		{name: "notes.txt", size: 4, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name)
			require.NoError(t, os.WriteFile(path, make([]byte, test.size), 0600))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, test.want, isUsableSave(info))
		})
	}
}

func TestIsUsableSaveRejectsSymbolicLinks(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.zip")
	require.NoError(t, os.WriteFile(target, []byte("outside"), 0600))
	link := filepath.Join(directory, "linked.zip")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.False(t, isUsableSave(info))
}

func TestValidateSaveArchiveRequiresFactorioLevelData(t *testing.T) {
	directory := t.TempDir()
	valid := filepath.Join(directory, "valid.zip")
	writeSaveFilterTestZip(t, valid, "world/level.dat", "level")
	require.NoError(t, ValidateSaveArchive(valid))

	missingLevel := filepath.Join(directory, "missing.zip")
	writeSaveFilterTestZip(t, missingLevel, "notes.txt", "not a save")
	assert.Error(t, ValidateSaveArchive(missingLevel))

	corrupt := filepath.Join(directory, "corrupt.zip")
	require.NoError(t, os.WriteFile(corrupt, []byte("not a zip"), 0600))
	assert.Error(t, ValidateSaveArchive(corrupt))
}

func TestActivateSaveUploadReplacesExistingArchive(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "world.zip")
	writeSaveFilterTestZip(t, destination, "world/level.dat", "old")
	temporary := filepath.Join(directory, ".save-upload-new.zip")
	writeSaveFilterTestZip(t, temporary, "world/level.dat", "new")

	require.NoError(t, activateSaveUploadInDirectory(directory, temporary, "world.zip"))
	assert.NoFileExists(t, temporary)
	require.NoError(t, ValidateSaveArchive(destination))
}

func TestActivateSaveUploadRejectsTemporaryFileOutsideSaveDirectory(t *testing.T) {
	directory := t.TempDir()
	temporary := filepath.Join(t.TempDir(), "upload.zip")
	writeSaveFilterTestZip(t, temporary, "world/level.dat", "new")
	assert.Error(t, activateSaveUploadInDirectory(directory, temporary, "world.zip"))
}

func writeSaveFilterTestZip(t *testing.T, path, entryName, contents string) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	entry, err := writer.Create(entryName)
	require.NoError(t, err)
	_, err = entry.Write([]byte(contents))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
}
