package factorio

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupModPackMergeTest(t *testing.T) (*profileTestEnvironment, string) {
	t.Helper()
	env := setupProfileTest(t)
	require.NoError(t, os.Remove(filepath.Join(env.active["mods"], "current_1.0.0.zip")))
	require.NoError(t, InitializeProfiles())
	pack := t.TempDir()
	originalSource, originalBuiltIn := modPackMergeSource, modPackMergeBuiltIn
	modPackMergeSource = func(string) string { return pack }
	modPackMergeBuiltIn = func(name string) (builtInModInfo, bool, error) {
		return builtInModInfo{Name: name, Version: Version{2, 1, 14, 0}, Dependencies: []string{"base >= 2.1"}}, true, nil
	}
	t.Cleanup(func() { modPackMergeSource, modPackMergeBuiltIn = originalSource, originalBuiltIn })
	writeMergeList(t, pack, map[string]bool{"base": true})
	return env, pack
}

func writeMergeMod(t *testing.T, directory, name, version string, dependencies ...string) {
	t.Helper()
	file, err := os.Create(filepath.Join(directory, name+"_"+version+".zip"))
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	entry, err := writer.Create(name + "_" + version + "/info.json")
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(entry).Encode(ModInfo{Name: name, Version: version, FactorioVersion: Version{2, 1, 0, 0}, Dependencies: dependencies}))
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
}

func writeMergeList(t *testing.T, directory string, states map[string]bool) {
	t.Helper()
	mods := []ModSimple{}
	for name, enabled := range states {
		mods = append(mods, ModSimple{Name: name, Enabled: enabled})
	}
	data, err := json.Marshal(map[string]interface{}{"mods": mods})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "mod-list.json"), data, 0600))
}

func mergeFilesSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		files[relative] = string(data)
		return err
	}))
	return files
}

func TestMergeModPackPreservesExistingProfileAndPack(t *testing.T) {
	env, pack := setupModPackMergeTest(t)
	active := env.active["mods"]
	writeMergeMod(t, active, "existing", "1.0.0")
	writeMergeMod(t, pack, "existing", "1.0.0")
	writeMergeMod(t, pack, "addon", "1.2.3", "base >= 2.1", "quality")
	writeMergeList(t, pack, map[string]bool{"base": true, "existing": true, "addon": true, "quality": false})
	require.NoError(t, os.WriteFile(filepath.Join(active, "mod-list.json"), []byte(`{"custom":{"keep":true},"mods":[{"name":"base","enabled":true},{"name":"existing","enabled":false,"version":"1.0.0","custom":"preserved"},{"name":"quality","enabled":true}]}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pack, "mod-settings.dat"), []byte("do not import these settings"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(pack, "extra.txt"), []byte("not a mod"), 0600))
	before, packBefore := mergeFilesSnapshot(t, env.root), mergeFilesSnapshot(t, pack)
	plan, err := PreviewModPackMerge("QoL")
	require.NoError(t, err)
	assert.Empty(t, plan.Conflicts)
	assert.Equal(t, []ModPackMergeItem{{Name: "addon", Version: "1.2.3", Enabled: true}}, plan.Additions)
	assert.Equal(t, []ModPackMergeItem{{Name: "existing", Version: "1.0.0", Enabled: false}}, plan.Kept)
	assert.Equal(t, before, mergeFilesSnapshot(t, env.root), "preview must not write")
	_, err = MergeModPack("QoL", plan.Revision)
	require.NoError(t, err)
	after := mergeFilesSnapshot(t, env.root)
	for path, data := range before {
		if path != filepath.Join("active", "mods", "mod-list.json") {
			assert.Equal(t, data, after[path], path)
		}
	}
	assert.Len(t, after, len(before)+1)
	assert.Equal(t, packBefore, mergeFilesSnapshot(t, pack))
	var list map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(after[filepath.Join("active", "mods", "mod-list.json")]), &list))
	assert.JSONEq(t, `{"keep":true}`, string(list["custom"]))
	assert.JSONEq(t, `[{"name":"base","enabled":true},{"name":"existing","enabled":false,"version":"1.0.0","custom":"preserved"},{"name":"quality","enabled":true},{"name":"addon","enabled":true}]`, string(list["mods"]))
	plan, err = PreviewModPackMerge("QoL")
	require.NoError(t, err)
	assert.Empty(t, plan.Additions)
	_, err = MergeModPack("QoL", plan.Revision)
	require.NoError(t, err)
	assert.Equal(t, after, mergeFilesSnapshot(t, env.root), "adding an already-installed pack must be a no-op")
}

func TestMergeModPackRejectsConflictsWithoutWriting(t *testing.T) {
	for _, test := range []struct {
		name                        string
		installedDeps, incomingDeps []string
		installedEnabled            bool
		incomingVersion             string
	}{
		{"required absent", nil, []string{"missing >= 1.0"}, true, "1.0.0"},
		{"required disabled", nil, []string{"existing"}, false, "1.0.0"},
		{"required wrong version", nil, []string{"existing >= 2.0"}, true, "1.0.0"},
		{"incoming incompatible", nil, []string{"! existing"}, true, "1.0.0"},
		{"installed incompatible", []string{"! addon"}, nil, true, "1.0.0"},
		{"optional constraint", nil, []string{"? existing >= 2.0"}, true, "1.0.0"},
		{"expansion disabled", nil, []string{"space-age"}, true, "1.0.0"},
		{"base constraint", nil, []string{"base >= 2.2"}, true, "1.0.0"},
		{"overlap version", nil, nil, true, "2.0.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			env, pack := setupModPackMergeTest(t)
			writeMergeMod(t, env.active["mods"], "existing", "1.0.0", test.installedDeps...)
			writeMergeList(t, env.active["mods"], map[string]bool{"base": true, "existing": test.installedEnabled})
			writeMergeMod(t, pack, "addon", "1.0.0", test.incomingDeps...)
			if test.name == "overlap version" {
				writeMergeMod(t, pack, "existing", test.incomingVersion)
			}
			writeMergeList(t, pack, map[string]bool{"base": true, "addon": true})
			before := mergeFilesSnapshot(t, env.root)
			plan, err := PreviewModPackMerge("pack")
			require.NoError(t, err)
			require.NotEmpty(t, plan.Conflicts)
			_, err = MergeModPack("pack", plan.Revision)
			assert.ErrorIs(t, err, ErrModPackMergeConflict)
			assert.Equal(t, before, mergeFilesSnapshot(t, env.root))
		})
	}
}

func TestMergeModPackRejectsStalePreview(t *testing.T) {
	for _, change := range []string{"active settings", "pack", "profile", "game version"} {
		t.Run(change, func(t *testing.T) {
			env, pack := setupModPackMergeTest(t)
			writeMergeMod(t, pack, "addon", "1.0.0")
			writeMergeList(t, pack, map[string]bool{"base": true, "addon": true})
			plan, err := PreviewModPackMerge("pack")
			require.NoError(t, err)
			switch change {
			case "active settings":
				require.NoError(t, os.WriteFile(filepath.Join(env.active["mods"], "mod-settings.dat"), []byte("new settings"), 0600))
			case "pack":
				writeMergeList(t, pack, map[string]bool{"base": true, "addon": false})
			case "profile":
				manifest, err := loadProfileManifest()
				require.NoError(t, err)
				require.NoError(t, copyProfileDirectory(profileDirectory(manifest.ActiveProfileID), profileDirectory("0123456789abcdef")))
				manifest.Profiles[0].ID = "0123456789abcdef"
				manifest.ActiveProfileID = "0123456789abcdef"
				require.NoError(t, saveProfileManifest(manifest))
			case "game version":
				SetFactorioServer(Server{Version: Version{2, 1, 20, 0}})
			}
			before := mergeFilesSnapshot(t, env.root)
			_, err = MergeModPack("pack", plan.Revision)
			assert.ErrorIs(t, err, ErrModPackMergeStale)
			assert.Equal(t, before, mergeFilesSnapshot(t, env.root))
		})
	}
}

func TestMergeModPackRollsBackFailedActivation(t *testing.T) {
	env, pack := setupModPackMergeTest(t)
	writeMergeMod(t, pack, "addon", "1.0.0")
	writeMergeList(t, pack, map[string]bool{"base": true, "addon": true})
	plan, err := PreviewModPackMerge("pack")
	require.NoError(t, err)
	before := mergeFilesSnapshot(t, env.root)
	profileRename = func(from, to string) error {
		if strings.Contains(from, ".mods-merge-staging-") && filepath.Base(from) == "mod-list.json" {
			return errors.New("simulated disk error")
		}
		return os.Rename(from, to)
	}
	_, err = MergeModPack("pack", plan.Revision)
	require.ErrorContains(t, err, "simulated disk error")
	assert.Equal(t, before, mergeFilesSnapshot(t, env.root))
	entries, err := os.ReadDir(env.active["mods"])
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".mods-merge-"))
	}
}

func TestMergeModPackPreservesRememberedStatesAndPins(t *testing.T) {
	env, pack := setupModPackMergeTest(t)
	writeMergeMod(t, pack, "addon", "1.0.0")
	writeMergeList(t, pack, map[string]bool{"base": true, "addon": true, "space-age": true})
	activeList := filepath.Join(env.active["mods"], "mod-list.json")
	require.NoError(t, os.WriteFile(activeList, []byte(`{"mods":[{"name":"base","enabled":true},{"name":"addon","enabled":false,"version":"2.0.0"}]}`), 0600))
	plan, err := PreviewModPackMerge("pack")
	require.NoError(t, err)
	assert.NotEmpty(t, plan.Conflicts)
	require.NoError(t, os.WriteFile(activeList, []byte(`{"mods":[{"name":"base","enabled":true},{"name":"addon","enabled":false,"version":"1.0.0"}]}`), 0600))
	plan, err = PreviewModPackMerge("pack")
	require.NoError(t, err)
	assert.Empty(t, plan.Conflicts)
	assert.False(t, plan.Additions[0].Enabled)
	_, err = MergeModPack("pack", plan.Revision)
	require.NoError(t, err)
	list, err := os.ReadFile(activeList)
	require.NoError(t, err)
	assert.NotContains(t, string(list), "space-age", "the pack must not turn on the expansion")
	assert.Contains(t, string(list), `"enabled": false`)
}

func TestMergeModPackDoesNotChangeLegacyReplacement(t *testing.T) {
	env, pack := setupModPackMergeTest(t)
	writeMergeMod(t, env.active["mods"], "existing", "1.0.0")
	writeMergeMod(t, pack, "addon", "1.0.0")
	writeMergeList(t, pack, map[string]bool{"base": true, "addon": true})
	require.NoError(t, os.WriteFile(filepath.Join(pack, "mod-settings.dat"), []byte("pack settings"), 0600))
	mods, err := NewMods(pack)
	require.NoError(t, err)
	saved := ModPack{Mods: mods}
	require.NoError(t, saved.LoadModPack())
	assert.NoFileExists(t, filepath.Join(env.active["mods"], "existing_1.0.0.zip"))
	assert.Equal(t, mergeFilesSnapshot(t, pack), mergeFilesSnapshot(t, env.active["mods"]))
}

func TestMergeModPackHonoursLifecycleAndProfileGate(t *testing.T) {
	setupModPackMergeTest(t)
	for _, server := range []Server{{Running: true}, {Stopping: true}} {
		SetFactorioServer(server)
		_, err := PreviewModPackMerge("pack")
		assert.ErrorIs(t, err, ErrServerActive)
		_, err = MergeModPack("pack", strings.Repeat("0", 64))
		assert.ErrorIs(t, err, ErrServerActive)
	}
	SetFactorioServer(Server{Version: Version{2, 1, 14, 0}})
	profileDataGate.RLock()
	done := make(chan error, 1)
	go func() { _, err := PreviewModPackMerge("pack"); done <- err }()
	select {
	case <-done:
		t.Error("preview bypassed profile-data gate")
	case <-time.After(25 * time.Millisecond):
	}
	profileDataGate.RUnlock()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("profile gate deadlocked")
	}
}

func TestMergeModPackRejectsUnsafeOrAmbiguousInputs(t *testing.T) {
	for _, invalid := range []string{"multiple versions", "malformed zip", "directory", "duplicate list", "null list", "bad pin", "symlink"} {
		t.Run(invalid, func(t *testing.T) {
			env, pack := setupModPackMergeTest(t)
			switch invalid {
			case "multiple versions":
				writeMergeMod(t, pack, "addon", "1.0.0")
				writeMergeMod(t, pack, "addon", "2.0.0")
			case "malformed zip":
				require.NoError(t, os.WriteFile(filepath.Join(pack, "broken.zip"), []byte("not a zip"), 0600))
			case "directory":
				require.NoError(t, os.Mkdir(filepath.Join(pack, "unpacked"), 0700))
			case "duplicate list":
				require.NoError(t, os.WriteFile(filepath.Join(pack, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true},{"name":"base","enabled":false}]}`), 0600))
			case "null list":
				require.NoError(t, os.WriteFile(filepath.Join(pack, "mod-list.json"), []byte(`{"mods":null}`), 0600))
			case "bad pin":
				writeMergeMod(t, pack, "addon", "1.0.0")
				require.NoError(t, os.WriteFile(filepath.Join(pack, "mod-list.json"), []byte(`{"mods":[{"name":"addon","enabled":true,"version":"9.0.0"}]}`), 0600))
			case "symlink":
				if err := os.Symlink(filepath.Join(env.active["mods"], "mod-settings.dat"), filepath.Join(pack, "linked.dat")); err != nil {
					t.Skip("symlinks unavailable")
				}
			}
			before := mergeFilesSnapshot(t, env.root)
			_, err := PreviewModPackMerge("pack")
			require.Error(t, err)
			assert.Equal(t, before, mergeFilesSnapshot(t, env.root))
		})
	}
	setupModPackMergeTest(t)
	for _, name := range []string{"../escape", "CON", "bad/name", ""} {
		_, err := PreviewModPackMerge(name)
		assert.ErrorIs(t, err, ErrInvalidModPackMerge)
	}
	for _, revision := range []string{"", "bogus", strings.Repeat("z", 64)} {
		_, err := MergeModPack("pack", revision)
		assert.ErrorIs(t, err, ErrInvalidModPackMerge)
	}
}

func TestMergeModPackAtMountedDestination(t *testing.T) {
	mount := os.Getenv("FSM_TEST_MOUNTED_MODS_DIR")
	if mount == "" {
		t.Skip("mounted directory integration test")
	}
	require.Equal(t, "/opt/factorio/mods", filepath.ToSlash(filepath.Clean(mount)), "use only the dedicated CI mount")
	env, pack := setupModPackMergeTest(t)
	// Other mounted tests may have left fixtures. Only use this dedicated mount.
	entries, err := os.ReadDir(mount)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NoError(t, os.RemoveAll(filepath.Join(mount, entry.Name())))
	}
	require.NoError(t, copyProfileDirectory(env.active["mods"], mount))
	env.active["mods"] = mount
	before, err := os.Stat(mount)
	require.NoError(t, err)
	writeMergeMod(t, pack, "addon", "1.0.0")
	writeMergeList(t, pack, map[string]bool{"base": true, "addon": true})
	plan, err := PreviewModPackMerge("pack")
	require.NoError(t, err)
	_, err = MergeModPack("pack", plan.Revision)
	require.NoError(t, err)
	after, err := os.Stat(mount)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "mount root must not be replaced")
	assert.FileExists(t, filepath.Join(mount, "addon_1.0.0.zip"))
	data, err := os.ReadFile(filepath.Join(mount, "mod-settings.dat"))
	require.NoError(t, err)
	assert.Equal(t, "current profile mod settings", string(data))
}
