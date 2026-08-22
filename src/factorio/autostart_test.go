package factorio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutostartSettingsPersistUIChoiceOverLegacyDefault(t *testing.T) {
	originalPath := autostartSettingsPath
	originalDefault := autostartSettingsDefault
	path := filepath.Join(t.TempDir(), autostartSettingsFileName)
	autostartSettingsPath = func() string { return path }
	autostartSettingsDefault = func() bool { return true }
	t.Cleanup(func() {
		autostartSettingsPath = originalPath
		autostartSettingsDefault = originalDefault
	})

	settings, err := LoadAutostartSettings()
	require.NoError(t, err)
	assert.True(t, settings.Enabled)

	settings, err = SetAutostartEnabled(false)
	require.NoError(t, err)
	assert.False(t, settings.Enabled)

	settings, err = LoadAutostartSettings()
	require.NoError(t, err)
	assert.False(t, settings.Enabled, "the persisted UI choice must override the legacy default")

	settings, err = SetAutostartEnabled(true)
	require.NoError(t, err)
	assert.True(t, settings.Enabled)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"enabled":true}`, string(contents))
}

func TestAutostartSettingsRejectMalformedState(t *testing.T) {
	originalPath := autostartSettingsPath
	path := filepath.Join(t.TempDir(), autostartSettingsFileName)
	autostartSettingsPath = func() string { return path }
	t.Cleanup(func() { autostartSettingsPath = originalPath })
	require.NoError(t, os.WriteFile(path, []byte(`{"enabled":`), 0600))

	_, err := LoadAutostartSettings()
	require.ErrorContains(t, err, "decode autostart settings")
}
