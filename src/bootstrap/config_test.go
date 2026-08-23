package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateConfigFileRejectsInvalidJSONWithoutOverwritingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.json")
	original := []byte(`{"rcon_pass":`)
	require.NoError(t, os.WriteFile(path, original, 0644))

	config := Config{ConfFile: path}
	assert.Panics(t, config.updateConfigFile)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, contents)
}

func TestUpdateConfigFileGeneratesSecretsAndRestrictsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"rcon_pass":"","cookie_encryption_key":""}`), 0644))

	config := Config{ConfFile: path}
	config.updateConfigFile()

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	var stored Config
	require.NoError(t, json.Unmarshal(contents, &stored))
	assert.NotEmpty(t, stored.FactorioRconPass)
	assert.NotEmpty(t, stored.CookieEncryptionKey)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}
