package factorio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func backupTestZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	w := zip.NewWriter(&output)
	for name, data := range files {
		file, err := w.Create(name)
		require.NoError(t, err)
		_, err = file.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return output.Bytes()
}

func setupBackupTest(t *testing.T) (*profileTestEnvironment, ProfileState) {
	t.Helper()
	env := setupProfileTest(t)
	oldRoot := checkpointRootPath
	checkpointRootPath = func() string { return filepath.Join(env.root, "checkpoints") }
	t.Cleanup(func() { checkpointRootPath = oldRoot })
	require.NoError(t, os.WriteFile(filepath.Join(env.active["saves"], "current.zip"), backupTestZip(t, map[string][]byte{"world/level.dat": []byte("world contents")}), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(env.active["mods"], "current_1.0.0.zip"), backupTestZip(t, map[string][]byte{"current_1.0.0/info.json": []byte(`{"name":"current","version":"1.0.0","factorio_version":"2.1"}`)}), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(env.active["mods"], "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true},{"name":"current","enabled":false},{"name":"quality","enabled":true},{"name":"elevated-rails","enabled":true},{"name":"space-age","enabled":true}]}`), 0600))
	require.NoError(t, InitializeProfiles())
	state, err := ListProfiles()
	require.NoError(t, err)
	return env, state
}

func exportTestBackup(t *testing.T, ids []string, saves, checkpoints bool) []byte {
	t.Helper()
	path, err := ExportProfileBackup(ProfileBackupOptions{ProfileIDs: ids, IncludeSaves: saves, IncludeCheckpoints: checkpoints})
	require.NoError(t, err)
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func backupContents(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	files := map[string][]byte{}
	for _, file := range reader.File {
		r, err := file.Open()
		require.NoError(t, err)
		files[file.Name], err = io.ReadAll(r)
		r.Close()
		require.NoError(t, err)
	}
	return files
}

func TestProfileBackupRoundTripPreservesExistingInstallation(t *testing.T) {
	env, initial := setupBackupTest(t)
	id := initial.ActiveProfileID
	manifestBefore, err := os.ReadFile(filepath.Join(profileRootPath(), profileManifestName))
	require.NoError(t, err)
	// Change live data after the initial snapshot: exports must not use the stale copy.
	settings := []byte(`{"name":"Live factory","game_password":"do-not-export","username":"account","token":"secret-token","visibility":{"public":true}}`)
	require.NoError(t, os.WriteFile(filepath.Join(env.active["config"], "server-settings.json"), settings, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(env.active["config"], "factorio.auth"), []byte("private"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(env.active["config"], "config.ini"), []byte("host-specific"), 0600))
	saveTime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(env.active["saves"], "current.zip"), saveTime, saveTime))
	metadata := checkpointMetadata{SchemaVersion: 1, ID: "20260926T120000.000000000Z-manual", CreatedAt: time.Now().UTC(), Trigger: CheckpointTriggerManual, SourceSave: "current.zip"}
	require.NoError(t, persistCheckpoint(id, metadata, filepath.Join(env.active["saves"], "current.zip")))
	data := exportTestBackup(t, []string{id}, true, true)
	files := backupContents(t, data)
	assert.NotContains(t, string(files["profiles/"+id+"/config/server-settings.json"]), "secret-token")
	assert.NotContains(t, string(files["profiles/"+id+"/config/server-settings.json"]), "do-not-export")
	assert.NotContains(t, files, "profiles/"+id+"/config/factorio.auth")
	assert.NotContains(t, files, "profiles/"+id+"/config/config.ini")
	manifestAfter, err := os.ReadFile(filepath.Join(profileRootPath(), profileManifestName))
	require.NoError(t, err)
	assert.Equal(t, manifestBefore, manifestAfter, "export must be read-only")
	preview, err := PreviewProfileBackup(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, preview.Profiles, 1)
	assert.Equal(t, "Current setup (import 2)", preview.Profiles[0].Name)
	assert.Equal(t, GameModeSpaceAge, preview.Profiles[0].GameMode)
	state, err := ImportProfileBackup(bytes.NewReader(data), int64(len(data)), []ProfileBackupSelection{{ID: id, Name: preview.Profiles[0].Name}})
	require.NoError(t, err)
	require.Len(t, state.Profiles, 2)
	assert.Equal(t, id, state.ActiveProfileID)
	imported := findProfileByName(t, state, preview.Profiles[0].Name)
	assert.NotEqual(t, id, imported.ID)
	assert.False(t, imported.Active)
	assert.Equal(t, "2.1.14", imported.InstalledVersion)
	assert.Equal(t, "latest", imported.ReleaseTarget)
	assert.Equal(t, "current.zip", imported.SelectedSave)
	importedSave, err := os.Stat(filepath.Join(profileDirectory(imported.ID), "saves", "current.zip"))
	require.NoError(t, err)
	assert.True(t, saveTime.Equal(importedSave.ModTime()), "preserve save ordering for Load Latest")
	assert.Equal(t, GameModeSpaceAge, imported.GameMode)
	assert.Empty(t, env.installLog)
	assert.False(t, GetFactorioServer().IsBusy())
	actual, err := os.ReadFile(filepath.Join(env.active["config"], "server-settings.json"))
	require.NoError(t, err)
	assert.Equal(t, settings, actual, "active configuration must remain byte-identical")
	var restoredSettings map[string]interface{}
	require.NoError(t, readBackupJSON(filepath.Join(profileDirectory(imported.ID), "config", "server-settings.json"), &restoredSettings))
	assert.Equal(t, "Live factory", restoredSettings["name"])
	assert.NotContains(t, restoredSettings, "game_password")
	assert.Equal(t, map[string]interface{}{"public": false, "lan": false}, restoredSettings["visibility"])
	mods, err := validateBackupMods(filepath.Join(profileDirectory(imported.ID), "mods"))
	require.NoError(t, err)
	require.Len(t, mods.Mods, 1)
	assert.False(t, mods.Mods[0].Enabled)
	modSettings, err := os.ReadFile(filepath.Join(profileDirectory(imported.ID), "mods", "mod-settings.dat"))
	require.NoError(t, err)
	assert.Equal(t, "current profile mod settings", string(modSettings))
	checkpoints, err := listCheckpoints(imported.ID)
	require.NoError(t, err)
	require.Len(t, checkpoints, 1)
	assert.Equal(t, metadata.ID, checkpoints[0].ID)
	// Existing activation still understands imported data, with no schema change.
	_, err = ActivateProfile(imported.ID)
	require.NoError(t, err)
	assert.False(t, GetFactorioServer().IsBusy())
}

func TestProfileBackupAllProfilesWithoutSavesAndSubsetImport(t *testing.T) {
	_, initial := setupBackupTest(t)
	state, err := CreateProfile("Second", "inactive", ProfileSourceClone)
	require.NoError(t, err)
	second := findProfileByName(t, state, "Second")
	data := exportTestBackup(t, []string{initial.ActiveProfileID, second.ID}, false, false)
	preview, err := PreviewProfileBackup(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, preview.Profiles, 2)
	for _, item := range preview.Profiles {
		assert.Zero(t, item.SaveCount)
		assert.Empty(t, item.SelectedSave)
		assert.Empty(t, item.Checkpoints)
	}
	state, err = ImportProfileBackup(bytes.NewReader(data), int64(len(data)), []ProfileBackupSelection{{ID: second.ID, Name: "Only this one"}})
	require.NoError(t, err)
	assert.Len(t, state.Profiles, 3)
	assert.Equal(t, initial.ActiveProfileID, state.ActiveProfileID)
}

func TestProfileBackupSelectionsOnlyResolveKnownIDs(t *testing.T) {
	env, initial := setupBackupTest(t)
	id := initial.ActiveProfileID
	data := exportTestBackup(t, []string{id}, true, false)
	before, err := os.ReadFile(filepath.Join(profileRootPath(), profileManifestName))
	require.NoError(t, err)
	for _, requestedID := range []string{
		"", ".", "..", "../" + id, id + "/../" + id,
		filepath.Join(env.root, "profiles", id), "not-a-profile", "0123456789abcdef",
	} {
		t.Run(requestedID, func(t *testing.T) {
			path, err := ExportProfileBackup(ProfileBackupOptions{ProfileIDs: []string{requestedID}})
			require.ErrorIs(t, err, ErrInvalidBackup)
			assert.Empty(t, path)
			_, err = ImportProfileBackup(bytes.NewReader(data), int64(len(data)), []ProfileBackupSelection{{ID: requestedID, Name: "Imported"}})
			require.ErrorIs(t, err, ErrInvalidBackup)
		})
	}
	_, err = ExportProfileBackup(ProfileBackupOptions{ProfileIDs: []string{id, id}})
	require.ErrorIs(t, err, ErrInvalidBackup)
	_, err = ImportProfileBackup(bytes.NewReader(data), int64(len(data)), []ProfileBackupSelection{{ID: id, Name: "Imported"}, {ID: id, Name: "Duplicate"}})
	require.ErrorIs(t, err, ErrInvalidBackup)
	after, err := os.ReadFile(filepath.Join(profileRootPath(), profileManifestName))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	entries, err := os.ReadDir(profileRootPath())
	require.NoError(t, err)
	assert.Len(t, entries, 2, "only the original profile and manifest remain")
	assert.Empty(t, env.installLog)
}

func TestProfileBackupImportFailureIsAllOrNothing(t *testing.T) {
	_, initial := setupBackupTest(t)
	state, err := CreateProfile("Second", "", ProfileSourceClone)
	require.NoError(t, err)
	second := findProfileByName(t, state, "Second")
	data := exportTestBackup(t, []string{initial.ActiveProfileID, second.ID}, true, false)
	before, err := os.ReadFile(filepath.Join(profileRootPath(), profileManifestName))
	require.NoError(t, err)
	selections := []ProfileBackupSelection{{ID: initial.ActiveProfileID, Name: "Imported"}, {ID: second.ID, Name: "Second"}}
	_, err = ImportProfileBackup(bytes.NewReader(data), int64(len(data)), selections)
	require.ErrorIs(t, err, ErrProfileNameConflict)
	after, err := os.ReadFile(filepath.Join(profileRootPath(), profileManifestName))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	entries, err := os.ReadDir(profileRootPath())
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	selections[1].Name = "Imported second"
	renamed := 0
	profileRename = func(source, destination string) error {
		if strings.Contains(filepath.Base(source), ".profile-write-") && !strings.HasSuffix(source, ".previous") && validateProfileID(filepath.Base(destination)) == nil {
			renamed++
			if renamed == 2 {
				return errors.New("injected disk failure")
			}
		}
		return os.Rename(source, destination)
	}
	_, err = ImportProfileBackup(bytes.NewReader(data), int64(len(data)), selections)
	require.Error(t, err)
	after, err = os.ReadFile(filepath.Join(profileRootPath(), profileManifestName))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	entries, err = os.ReadDir(profileRootPath())
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestProfileBackupRejectsFutureFormatsAndCorruption(t *testing.T) {
	_, initial := setupBackupTest(t)
	data := exportTestBackup(t, []string{initial.ActiveProfileID}, true, false)
	for _, change := range []func(map[string][]byte){
		func(files map[string][]byte) {
			var manifest ProfileBackup
			require.NoError(t, json.Unmarshal(files["backup.json"], &manifest))
			manifest.FormatVersion++
			files["backup.json"], _ = json.Marshal(manifest)
		},
		func(files map[string][]byte) {
			files["profiles/"+initial.ActiveProfileID+"/saves/current.zip"] = []byte("not a ZIP")
		},
		func(files map[string][]byte) {
			files["profiles/"+initial.ActiveProfileID+"/config/conf.json"] = []byte(`{}`)
		},
	} {
		files := backupContents(t, data)
		change(files)
		invalid := backupTestZip(t, files)
		_, err := ImportProfileBackup(bytes.NewReader(invalid), int64(len(invalid)), []ProfileBackupSelection{{ID: initial.ActiveProfileID, Name: "Imported"}})
		require.Error(t, err)
		state, err := ListProfiles()
		require.NoError(t, err)
		assert.Len(t, state.Profiles, 1)
	}
}

func TestBackupZipRejectsUnsafeNamesDuplicatesAndBudgets(t *testing.T) {
	for _, name := range []string{"../escape.zip", "/absolute.zip", `C:\secret.zip`, "CON.zip", "folder/../x.zip", "mod.zip.", "a//x.zip"} {
		data := backupTestZip(t, map[string][]byte{name: []byte("x")})
		_, err := PreviewModBackup(bytes.NewReader(data), int64(len(data)))
		assert.Error(t, err, name)
	}
	for _, names := range [][]string{{"mod-list.json", "MOD-LIST.JSON"}, {"a", "a/x"}} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, name := range names {
			_, err := writer.Create(name)
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
		require.NoError(t, err)
		assert.ErrorIs(t, inspectBackupZip(reader, nil), ErrInvalidBackup)
	}
	file := &zip.File{FileHeader: zip.FileHeader{Name: "mod-list.json", UncompressedSize64: backupMaxExpandedBytes + 1}}
	assert.ErrorIs(t, inspectBackupZip(&zip.Reader{File: []*zip.File{file}}, nil), ErrInvalidBackup)
	file.UncompressedSize64 = 1
	file.SetMode(os.ModeSymlink | 0777)
	assert.ErrorIs(t, inspectBackupZip(&zip.Reader{File: []*zip.File{file}}, nil), ErrInvalidBackup)
}

func TestRestoreLegacyModBackupPreservesDisabledModsAndSettings(t *testing.T) {
	env, initial := setupBackupTest(t)
	path, err := zipBackupDirectory(env.active["mods"])
	require.NoError(t, err)
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	preview, err := PreviewModBackup(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, GameModeSpaceAge, preview.GameMode)
	assert.True(t, preview.HasSettings)
	assert.False(t, preview.Mods[0].Enabled)
	require.NoError(t, os.WriteFile(filepath.Join(env.active["mods"], "mod-settings.dat"), []byte("changed"), 0600))
	_, err = RestoreModBackup(bytes.NewReader(data), int64(len(data)), "wrong-profile")
	require.ErrorIs(t, err, ErrInvalidProfile)
	_, err = RestoreModBackup(bytes.NewReader(data), int64(len(data)), initial.ActiveProfileID)
	require.NoError(t, err)
	actual, err := os.ReadFile(filepath.Join(env.active["mods"], "mod-settings.dat"))
	require.NoError(t, err)
	assert.Equal(t, "current profile mod settings", string(actual))
	assert.Empty(t, env.installLog)
	state, err := ListProfiles()
	require.NoError(t, err)
	assert.Equal(t, initial.ActiveProfileID, state.ActiveProfileID)
	// Failed activation rolls entries back, including the original settings.
	profileRename = func(source, destination string) error {
		if strings.Contains(source, ".profile-staging-") {
			return errors.New("injected failure")
		}
		return os.Rename(source, destination)
	}
	_, err = RestoreModBackup(bytes.NewReader(data), int64(len(data)), initial.ActiveProfileID)
	require.Error(t, err)
	actual, err = os.ReadFile(filepath.Join(env.active["mods"], "mod-settings.dat"))
	require.NoError(t, err)
	assert.Equal(t, "current profile mod settings", string(actual))
}

func TestBackupsRequireStoppedServer(t *testing.T) {
	_, state := setupBackupTest(t)
	data := exportTestBackup(t, []string{state.ActiveProfileID}, true, false)
	SetFactorioServer(Server{Running: true})
	_, err := ExportProfileBackup(ProfileBackupOptions{ProfileIDs: []string{state.ActiveProfileID}})
	require.ErrorIs(t, err, ErrProfileServerActive)
	_, err = ImportProfileBackup(bytes.NewReader(data), int64(len(data)), []ProfileBackupSelection{{ID: state.ActiveProfileID, Name: "Imported"}})
	require.ErrorIs(t, err, ErrProfileServerActive)
}

func TestRestoreModBackupAtMountedDestination(t *testing.T) {
	mount := os.Getenv("FSM_TEST_MOUNTED_MODS_DIR")
	if mount == "" {
		t.Skip("mounted directory integration test")
	}
	env, initial := setupBackupTest(t)
	path, err := zipBackupDirectory(env.active["mods"])
	require.NoError(t, err)
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	env.active["mods"] = mount
	_, err = RestoreModBackup(bytes.NewReader(data), int64(len(data)), initial.ActiveProfileID)
	require.NoError(t, err)
	actual, err := os.ReadFile(filepath.Join(mount, "mod-settings.dat"))
	require.NoError(t, err)
	assert.Equal(t, "current profile mod settings", string(actual))
}
