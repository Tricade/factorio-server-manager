package factorio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/stretchr/testify/require"
)

func configureSettingsFixture(t *testing.T, contents string) bootstrap.Config {
	t.Helper()
	previousConfig := bootstrap.GetConfig()
	stateMutex.RLock()
	previousServer := instantiated
	stateMutex.RUnlock()
	root := t.TempDir()
	previousJSON, err := json.Marshal(previousConfig)
	require.NoError(t, err)
	restorePath := filepath.Join(root, "restore.json")
	require.NoError(t, os.WriteFile(restorePath, previousJSON, 0600))
	t.Cleanup(func() {
		bootstrap.NewConfig([]string{"--conf", restorePath})
		SetFactorioServer(previousServer)
	})
	settings := filepath.Join(root, "server-settings.json")
	admins := filepath.Join(root, "server-adminlist.json")
	require.NoError(t, os.WriteFile(settings, []byte(contents), 0600))
	require.NoError(t, os.WriteFile(admins, []byte(`[]`), 0600))
	configPath := filepath.Join(root, "conf.json")
	encoded, err := json.Marshal(bootstrap.Config{SettingsFile: settings, FactorioAdminFile: admins, FactorioConfigDir: root})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, encoded, 0600))
	config := bootstrap.NewConfig([]string{"--conf", configPath, "--dir", root})
	SetFactorioServer(Server{Version: Version{2, 1, 20}, Settings: map[string]interface{}{"name": "original"}})
	return config
}

func TestProfileReloadRejectsNonObjectSettingsWithoutChangingRuntime(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `{} {"name":"trailing"}`} {
		t.Run(input, func(t *testing.T) {
			configureSettingsFixture(t, input)
			var err error
			require.NotPanics(t, func() { err = ReloadFactorioServerProfile("127.0.0.1", 34197, "next.zip") })
			require.Error(t, err)
			require.Equal(t, "original", GetFactorioServer().SettingsSnapshot()["name"])
			require.Empty(t, GetFactorioServer().Snapshot().Savefile)
		})
	}
}

func TestUpdatingNilServerSettingsDoesNotCommitNull(t *testing.T) {
	config := configureSettingsFixture(t, `{"name":"original"}`)
	require.Error(t, GetFactorioServer().UpdateServerSettings(nil))
	contents, err := os.ReadFile(config.SettingsFile)
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"original"}`, string(contents))
}

func TestProfileReloadRejectsInvalidAdminsWithoutChangingRuntime(t *testing.T) {
	for _, input := range []string{`null`, `{}`, `"administrator"`, `[1]`, `[] []`} {
		t.Run(input, func(t *testing.T) {
			config := configureSettingsFixture(t, `{"name":"next"}`)
			require.NoError(t, os.WriteFile(config.FactorioAdminFile, []byte(input), 0600))
			require.Error(t, ReloadFactorioServerProfile("127.0.0.1", 34197, "next.zip"))
			require.Equal(t, "original", GetFactorioServer().SettingsSnapshot()["name"])
			require.Empty(t, GetFactorioServer().Snapshot().Savefile)
		})
	}
}
