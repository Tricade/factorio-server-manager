package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteInitialAdminCredential(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "sqlite.db")

	credentialPath, err := writeInitialAdminCredential(databasePath, "admin", "generated-password")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(directory, "initial-admin-password.txt"), credentialPath)

	contents, err := os.ReadFile(credentialPath)
	require.NoError(t, err)
	assert.Equal(t, "username=admin\npassword=generated-password\n", string(contents))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(credentialPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}
