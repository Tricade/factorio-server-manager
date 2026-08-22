package factorio

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetReleaseStatusDetectsInstalledChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels":
			_, _ = w.Write([]byte(`{"experimental":{"headless":"2.1.14"},"stable":{"headless":"2.0.77"}}`))
		case "/versions":
			_, _ = w.Write([]byte(`{"core-linux_headless64":[{"from":"2.1.13","to":"2.1.14"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	originalChannelURL := releaseChannelMetadataURL
	originalVersionURL := releaseVersionMetadataURL
	originalStatePath := runtimeStatePath
	originalServer := GetFactorioServer().Snapshot()
	releaseChannelMetadataURL = server.URL + "/channels"
	releaseVersionMetadataURL = server.URL + "/versions"
	statePath := filepath.Join(t.TempDir(), runtimeStateFileName)
	runtimeStatePath = func() string { return statePath }
	SetFactorioServer(Server{Version: Version{2, 1, 14, 0}, BaseModVersion: "2.1.14"})
	t.Cleanup(func() {
		releaseChannelMetadataURL = originalChannelURL
		releaseVersionMetadataURL = originalVersionURL
		runtimeStatePath = originalStatePath
		SetFactorioServer(Server{Version: originalServer.Version, BaseModVersion: originalServer.BaseModVersion})
	})

	status, err := GetReleaseStatus()
	require.NoError(t, err)
	assert.Equal(t, "latest", status.InstalledChannel)
	assert.Equal(t, "2.1.14", status.LatestVersion)
	assert.Equal(t, "2.0.77", status.StableVersion)
	assert.Contains(t, status.AvailableVersions, "2.1.13")
}

func TestReleaseDownloadURL(t *testing.T) {
	stable, err := ReleaseDownloadURL("stable")
	require.NoError(t, err)
	assert.Equal(t, "https://www.factorio.com/get-download/stable/headless/linux64", stable)

	latest, err := ReleaseDownloadURL("latest")
	require.NoError(t, err)
	assert.Equal(t, "https://www.factorio.com/get-download/latest/headless/linux64", latest)

	exact, err := ReleaseDownloadURL("2.1.14")
	require.NoError(t, err)
	assert.Equal(t, "https://www.factorio.com/get-download/2.1.14/headless/linux64", exact)
}

func TestReleaseDownloadURLRejectsUnsupportedChannel(t *testing.T) {
	_, err := ReleaseDownloadURL("https://example.invalid/factorio.tar.xz")
	assert.Error(t, err)
}

func TestRuntimeStateRoundTripPreservesExactInstalledVersion(t *testing.T) {
	originalStatePath := runtimeStatePath
	statePath := filepath.Join(t.TempDir(), runtimeStateFileName)
	runtimeStatePath = func() string { return statePath }
	t.Cleanup(func() { runtimeStatePath = originalStatePath })

	require.NoError(t, persistRuntimeState("latest", "2.1.14.0"))
	state, err := LoadRuntimeState()
	require.NoError(t, err)
	assert.Equal(t, RuntimeState{ReleaseTarget: "latest", InstalledVersion: "2.1.14"}, state)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(statePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestGetReleaseStatusUsesPersistedTargetWhenMetadataIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	originalChannelURL := releaseChannelMetadataURL
	originalVersionURL := releaseVersionMetadataURL
	originalStatePath := runtimeStatePath
	originalServer := GetFactorioServer().Snapshot()
	statePath := filepath.Join(t.TempDir(), runtimeStateFileName)
	releaseChannelMetadataURL = server.URL
	releaseVersionMetadataURL = server.URL
	runtimeStatePath = func() string { return statePath }
	SetFactorioServer(Server{Version: Version{2, 1, 14, 0}, BaseModVersion: "2.1.14"})
	t.Cleanup(func() {
		releaseChannelMetadataURL = originalChannelURL
		releaseVersionMetadataURL = originalVersionURL
		runtimeStatePath = originalStatePath
		SetFactorioServer(Server{Version: originalServer.Version, BaseModVersion: originalServer.BaseModVersion})
	})
	require.NoError(t, persistRuntimeState("latest", "2.1.14"))

	status, err := GetReleaseStatus()
	require.NoError(t, err)
	assert.Equal(t, "latest", status.InstalledChannel)
	assert.Equal(t, "latest", status.InstalledTarget)
	assert.NotEmpty(t, status.MetadataError)
}

func TestGetReleaseStatusDoesNotCallOutdatedRollingTargetInstalled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels":
			_, _ = w.Write([]byte(`{"experimental":{"headless":"2.1.14"},"stable":{"headless":"2.0.77"}}`))
		case "/versions":
			_, _ = w.Write([]byte(`{"core-linux_headless64":[{"from":"2.1.13","to":"2.1.14"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	originalChannelURL := releaseChannelMetadataURL
	originalVersionURL := releaseVersionMetadataURL
	originalStatePath := runtimeStatePath
	originalServer := GetFactorioServer().Snapshot()
	statePath := filepath.Join(t.TempDir(), runtimeStateFileName)
	releaseChannelMetadataURL = server.URL + "/channels"
	releaseVersionMetadataURL = server.URL + "/versions"
	runtimeStatePath = func() string { return statePath }
	SetFactorioServer(Server{Version: Version{2, 1, 13, 0}, BaseModVersion: "2.1.13"})
	t.Cleanup(func() {
		releaseChannelMetadataURL = originalChannelURL
		releaseVersionMetadataURL = originalVersionURL
		runtimeStatePath = originalStatePath
		SetFactorioServer(Server{Version: originalServer.Version, BaseModVersion: originalServer.BaseModVersion})
	})
	require.NoError(t, persistRuntimeState("latest", "2.1.13"))

	status, err := GetReleaseStatus()
	require.NoError(t, err)
	assert.Equal(t, "latest", status.InstalledTarget)
	assert.Equal(t, "custom", status.InstalledChannel)
	assert.Equal(t, "2.1.13", status.InstalledVersion)
}

func TestCopyReleaseFilesPreservesServerData(t *testing.T) {
	sourceDir := t.TempDir()
	destinationDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "bin", "x64"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "config"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "mods"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "bin", "x64", "factorio"), []byte("new binary"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "config", "config.ini"), []byte("new config"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "mods", "mod-list.json"), []byte("new mods"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "config"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "mods"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "config", "config.ini"), []byte("saved config"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "mods", "mod-list.json"), []byte("saved mods"), 0644))

	require.NoError(t, replaceReleaseFiles(sourceDir, destinationDir))
	binary, err := os.ReadFile(filepath.Join(destinationDir, "bin", "x64", "factorio"))
	require.NoError(t, err)
	assert.Equal(t, "new binary", string(binary))
	config, err := os.ReadFile(filepath.Join(destinationDir, "config", "config.ini"))
	require.NoError(t, err)
	assert.Equal(t, "saved config", string(config))
	mods, err := os.ReadFile(filepath.Join(destinationDir, "mods", "mod-list.json"))
	require.NoError(t, err)
	assert.Equal(t, "saved mods", string(mods))
}

func TestReplaceReleaseFilesRemovesObsoleteProgramFiles(t *testing.T) {
	sourceDir := t.TempDir()
	destinationDir := filepath.Join(t.TempDir(), "factorio")
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "bin", "factorio"), []byte("new"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "bin", "obsolete"), []byte("old"), 0755))

	require.NoError(t, replaceReleaseFiles(sourceDir, destinationDir))
	_, err := os.Stat(filepath.Join(destinationDir, "bin", "obsolete"))
	assert.True(t, os.IsNotExist(err))
}

func TestReplaceReleaseFilesRestoresPreviousReleaseWhenActivationFails(t *testing.T) {
	sourceDir := t.TempDir()
	destinationDir := filepath.Join(t.TempDir(), "factorio")
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "bin", "factorio"), []byte("new"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "bin", "factorio"), []byte("old"), 0755))

	originalRename := releaseRename
	defer func() { releaseRename = originalRename }()
	activationAttempted := false
	releaseRename = func(oldPath, newPath string) error {
		if strings.Contains(oldPath, ".factorio-release-staging-") && filepath.Dir(newPath) == destinationDir && !activationAttempted {
			activationAttempted = true
			return errors.New("simulated activation failure")
		}
		return os.Rename(oldPath, newPath)
	}

	err := replaceReleaseFiles(sourceDir, destinationDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "previous release restored")
	binary, readErr := os.ReadFile(filepath.Join(destinationDir, "bin", "factorio"))
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(binary))
}

func TestReplaceReleaseFilesRestoresPreviousReleaseWhenBackupFailsPartway(t *testing.T) {
	sourceDir := t.TempDir()
	destinationDir := filepath.Join(t.TempDir(), "factorio")
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "bin", "factorio"), []byte("new"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "bin"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "bin", "factorio"), []byte("old"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "data.dat"), []byte("old data"), 0644))

	originalRename := releaseRename
	defer func() { releaseRename = originalRename }()
	releaseRename = func(oldPath, newPath string) error {
		if filepath.Base(oldPath) == "data.dat" && strings.Contains(newPath, ".factorio-release-backup-") {
			return errors.New("simulated backup failure")
		}
		return os.Rename(oldPath, newPath)
	}

	err := replaceReleaseFiles(sourceDir, destinationDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "previous release restored")
	binary, readErr := os.ReadFile(filepath.Join(destinationDir, "bin", "factorio"))
	require.NoError(t, readErr)
	assert.Equal(t, "old", string(binary))
	data, readErr := os.ReadFile(filepath.Join(destinationDir, "data.dat"))
	require.NoError(t, readErr)
	assert.Equal(t, "old data", string(data))
}

// This test is enabled by the Docker integration command. Its destination is a
// tmpfs mount at /opt/factorio, reproducing the EBUSY failure seen on Unraid.
func TestReplaceReleaseFilesAtMountedDestination(t *testing.T) {
	destinationDir := os.Getenv("FSM_TEST_MOUNTED_FACTORIO_DIR")
	if destinationDir == "" {
		t.Skip("set FSM_TEST_MOUNTED_FACTORIO_DIR to a dedicated mount point")
	}

	sourceDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "bin", "x64"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "data", "base"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "bin", "x64", "factorio"), []byte("new binary"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "data", "base", "info.json"), []byte("new data"), 0644))

	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "bin", "x64"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "saves"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "mods"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(destinationDir, "config"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "bin", "x64", "factorio"), []byte("old binary"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "obsolete.txt"), []byte("obsolete"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "saves", "factory.zip"), []byte("save data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "mods", "cargo-ships.zip"), []byte("mod data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(destinationDir, "config", "server-settings.json"), []byte("config data"), 0644))

	require.NoError(t, replaceReleaseFiles(sourceDir, destinationDir))

	assertFileContents(t, filepath.Join(destinationDir, "bin", "x64", "factorio"), "new binary")
	assertFileContents(t, filepath.Join(destinationDir, "data", "base", "info.json"), "new data")
	assertFileContents(t, filepath.Join(destinationDir, "saves", "factory.zip"), "save data")
	assertFileContents(t, filepath.Join(destinationDir, "mods", "cargo-ships.zip"), "mod data")
	assertFileContents(t, filepath.Join(destinationDir, "config", "server-settings.json"), "config data")
	_, err := os.Stat(filepath.Join(destinationDir, "obsolete.txt"))
	assert.True(t, os.IsNotExist(err))

	entries, err := os.ReadDir(destinationDir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".factorio-release-"), "temporary release directory was left behind")
	}
}

func assertFileContents(t *testing.T, path, expected string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(contents))
}
