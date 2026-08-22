package factorio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type profileTestEnvironment struct {
	root       string
	active     map[string]string
	runtime    RuntimeState
	installLog []RuntimeState
}

func setupProfileTest(t *testing.T) *profileTestEnvironment {
	t.Helper()
	root := t.TempDir()
	activeRoot := filepath.Join(root, "active")
	active := map[string]string{
		"saves":  filepath.Join(activeRoot, "saves"),
		"mods":   filepath.Join(activeRoot, "mods"),
		"config": filepath.Join(activeRoot, "config"),
	}
	for _, directory := range active {
		require.NoError(t, os.MkdirAll(directory, 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(active["saves"], "current.zip"), []byte("current save"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(active["mods"], "current_1.0.0.zip"), []byte("current mod"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(active["mods"], "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(active["config"], "server-settings.json"), []byte(`{"name":"current"}`), 0644))

	environment := &profileTestEnvironment{
		root:    root,
		active:  active,
		runtime: RuntimeState{ReleaseTarget: "latest", InstalledVersion: "2.1.14"},
	}
	originalRootPath := profileRootPath
	originalActiveDirectories := profileActiveDirectories
	originalLoadRuntimeState := profileLoadRuntimeState
	originalPersistRuntimeState := profilePersistRuntimeState
	originalInstallRelease := profileInstallRelease
	originalReloadServer := profileReloadServer
	originalGameModeStatus := profileGetGameModeStatus
	originalSettingsPath := profileSettingsFilePath
	originalRename := profileRename
	originalNow := profileNow
	originalServer := GetFactorioServer().Snapshot()

	profileRootPath = func() string { return filepath.Join(root, "profiles") }
	profileActiveDirectories = func() map[string]string { return active }
	profileLoadRuntimeState = func() (RuntimeState, error) { return environment.runtime, nil }
	profilePersistRuntimeState = func(target, version string) error {
		environment.runtime = RuntimeState{ReleaseTarget: target, InstalledVersion: version}
		return nil
	}
	profileInstallRelease = func(version, target string, _ []string) error {
		environment.installLog = append(environment.installLog, RuntimeState{ReleaseTarget: target, InstalledVersion: version})
		environment.runtime = RuntimeState{ReleaseTarget: target, InstalledVersion: version}
		var parsed Version
		require.NoError(t, parsed.UnmarshalText([]byte(version)))
		stateMutex.Lock()
		instantiated.Version = parsed
		instantiated.BaseModVersion = version
		stateMutex.Unlock()
		return nil
	}
	profileReloadServer = func(bindIP string, port int, save string) error {
		settings, err := os.ReadFile(filepath.Join(active["config"], "server-settings.json"))
		if err != nil {
			return err
		}
		if !strings.Contains(string(settings), "name") {
			return errors.New("invalid settings")
		}
		GetFactorioServer().ConfigureStart(bindIP, port, save)
		return nil
	}
	profileGetGameModeStatus = func() (GameModeStatus, error) {
		return GameModeStatus{Mode: GameModeFactorio, Features: []GameModeFeature{
			{Name: "elevated-rails", Available: true},
			{Name: "quality", Available: true},
			{Name: "space-age", Available: true},
		}}, nil
	}
	profileSettingsFilePath = func() string { return filepath.Join(active["config"], "server-settings.json") }
	profileRename = os.Rename
	profileNow = time.Now
	SetFactorioServer(Server{
		Savefile:       "current.zip",
		BindIP:         "0.0.0.0",
		Port:           34197,
		Version:        Version{2, 1, 14, 0},
		BaseModVersion: "2.1.14",
		Settings:       map[string]interface{}{"name": "current"},
	})

	t.Cleanup(func() {
		profileRootPath = originalRootPath
		profileActiveDirectories = originalActiveDirectories
		profileLoadRuntimeState = originalLoadRuntimeState
		profilePersistRuntimeState = originalPersistRuntimeState
		profileInstallRelease = originalInstallRelease
		profileReloadServer = originalReloadServer
		profileGetGameModeStatus = originalGameModeStatus
		profileSettingsFilePath = originalSettingsPath
		profileRename = originalRename
		profileNow = originalNow
		SetFactorioServer(Server{
			Savefile:       originalServer.Savefile,
			BindIP:         originalServer.BindIP,
			Port:           originalServer.Port,
			Running:        originalServer.Running,
			Stopping:       originalServer.Stopping,
			Version:        originalServer.Version,
			BaseModVersion: originalServer.BaseModVersion,
			Settings:       map[string]interface{}{},
		})
	})
	return environment
}

func TestInitializeProfilesMigratesCurrentSetupIdempotently(t *testing.T) {
	environment := setupProfileTest(t)

	require.NoError(t, InitializeProfiles())
	require.NoError(t, InitializeProfiles())
	state, err := ListProfiles()
	require.NoError(t, err)
	require.Len(t, state.Profiles, 1)
	profile := state.Profiles[0]
	assert.True(t, profile.Active)
	assert.Equal(t, defaultProfileName, profile.Name)
	assert.Equal(t, "latest", profile.ReleaseTarget)
	assert.Equal(t, "2.1.14", profile.InstalledVersion)
	assert.Equal(t, 1, profile.SaveCount)
	assert.Equal(t, 1, profile.ModCount)
	assertFileContents(t, filepath.Join(environment.root, "profiles", profile.ID, "saves", "current.zip"), "current save")
	assertFileContents(t, filepath.Join(environment.root, "profiles", profile.ID, "mods", "current_1.0.0.zip"), "current mod")
}

func TestListProfilesClearsSelectedSaveAfterLastSaveIsDeleted(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	require.NoError(t, os.Remove(filepath.Join(environment.active["saves"], "current.zip")))

	state, err := ListProfiles()
	require.NoError(t, err)
	require.Len(t, state.Profiles, 1)
	assert.Zero(t, state.Profiles[0].SaveCount)
	assert.Empty(t, state.Profiles[0].SelectedSave)
}

func TestUpdateProfileStartupPersistsActiveRuntimeConfiguration(t *testing.T) {
	setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := ListProfiles()
	require.NoError(t, err)
	active := findActiveProfile(t, state)

	state, err = UpdateProfileStartup(active.ID, "127.0.0.1", 34201, "current.zip")
	require.NoError(t, err)
	updated := findActiveProfile(t, state)
	assert.Equal(t, "127.0.0.1", updated.BindIP)
	assert.Equal(t, 34201, updated.Port)
	assert.Equal(t, "current.zip", updated.SelectedSave)

	snapshot := GetFactorioServer().Snapshot()
	assert.Equal(t, "127.0.0.1", snapshot.BindIP)
	assert.Equal(t, 34201, snapshot.Port)
	assert.Equal(t, "current.zip", snapshot.Savefile)
}

func TestUpdateProfileStartupRejectsInvalidValuesWithoutChangingRuntime(t *testing.T) {
	setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := ListProfiles()
	require.NoError(t, err)
	active := findActiveProfile(t, state)
	before := GetFactorioServer().Snapshot()

	_, err = UpdateProfileStartup(active.ID, "not-an-ip", 0, "missing.zip")
	require.ErrorIs(t, err, ErrInvalidProfile)
	after := GetFactorioServer().Snapshot()
	assert.Equal(t, before.BindIP, after.BindIP)
	assert.Equal(t, before.Port, after.Port)
	assert.Equal(t, before.Savefile, after.Savefile)
}

func TestInitializeProfilesRestoresPersistedStartupConfiguration(t *testing.T) {
	setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := ListProfiles()
	require.NoError(t, err)
	active := findActiveProfile(t, state)
	_, err = UpdateProfileStartup(active.ID, "127.0.0.1", 34201, "current.zip")
	require.NoError(t, err)

	GetFactorioServer().ConfigureStart("", 0, "")
	require.NoError(t, InitializeProfiles())
	snapshot := GetFactorioServer().Snapshot()
	assert.Equal(t, "127.0.0.1", snapshot.BindIP)
	assert.Equal(t, 34201, snapshot.Port)
	assert.Equal(t, "current.zip", snapshot.Savefile)
}

func TestCreateCloneAndEmptyProfiles(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())

	state, err := CreateProfile("Modded copy", "Current mods and save", ProfileSourceClone)
	require.NoError(t, err)
	clone := findProfileByName(t, state, "Modded copy")
	assert.False(t, clone.Active)
	assert.Equal(t, 1, clone.SaveCount)
	assert.Equal(t, 1, clone.ModCount)
	assertFileContents(t, filepath.Join(environment.root, "profiles", clone.ID, "saves", "current.zip"), "current save")

	state, err = CreateProfile("Vanilla", "Fresh base game", ProfileSourceEmpty)
	require.NoError(t, err)
	empty := findProfileByName(t, state, "Vanilla")
	assert.Equal(t, 0, empty.SaveCount)
	assert.Equal(t, 0, empty.ModCount)
	entries, err := os.ReadDir(filepath.Join(environment.root, "profiles", empty.ID, "saves"))
	require.NoError(t, err)
	assert.Empty(t, entries)
	modList, err := os.ReadFile(filepath.Join(environment.root, "profiles", empty.ID, "mods", "mod-list.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"mods":[
		{"name":"base","enabled":true},
		{"name":"elevated-rails","enabled":false},
		{"name":"quality","enabled":false},
		{"name":"space-age","enabled":false}
	]}`, string(modList))
	assertFileContents(t, filepath.Join(environment.root, "profiles", empty.ID, "config", "server-settings.json"), `{"name":"current"}`)
}

func TestActivateProfileRestoresExplicitBuiltInModState(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := CreateProfile("Custom expansion", "", ProfileSourceClone)
	require.NoError(t, err)
	target := findProfileByName(t, state, "Custom expansion")
	prepareStoredTargetProfile(t, environment, target, "custom")

	manifest, err := loadProfileManifest()
	require.NoError(t, err)
	index := profileIndex(manifest, target.ID)
	manifest.Profiles[index].GameMode = GameModeCustom
	manifest.Profiles[index].EnabledBuiltInMods = []string{"quality"}
	require.NoError(t, saveProfileManifest(manifest))

	_, err = ActivateProfile(target.ID)
	require.NoError(t, err)
	modListData, err := os.ReadFile(filepath.Join(environment.active["mods"], "mod-list.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"mods":[
		{"name":"base","enabled":true},
		{"name":"elevated-rails","enabled":false},
		{"name":"quality","enabled":true},
		{"name":"space-age","enabled":false}
	]}`, string(modListData))
}

func TestActivateProfileSnapshotsCurrentDataAndDoesNotStartServer(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := CreateProfile("Vanilla", "", ProfileSourceClone)
	require.NoError(t, err)
	target := findProfileByName(t, state, "Vanilla")
	current := findActiveProfile(t, state)
	prepareStoredTargetProfile(t, environment, target, "vanilla")

	require.NoError(t, os.WriteFile(filepath.Join(environment.active["saves"], "current.zip"), []byte("current save after playing"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(environment.active["mods"], "new-mod_1.0.0.zip"), []byte("new active mod"), 0644))

	state, err = ActivateProfile(target.ID)
	require.NoError(t, err)
	assert.Equal(t, target.ID, state.ActiveProfileID)
	assert.False(t, GetFactorioServer().GetRunning())
	assertFileContents(t, filepath.Join(environment.active["saves"], "vanilla.zip"), "vanilla save")
	_, err = os.Stat(filepath.Join(environment.active["saves"], "current.zip"))
	assert.True(t, os.IsNotExist(err))
	assertFileContents(t, filepath.Join(environment.root, "profiles", current.ID, "saves", "current.zip"), "current save after playing")
	assertFileContents(t, filepath.Join(environment.root, "profiles", current.ID, "mods", "new-mod_1.0.0.zip"), "new active mod")

	state, err = ActivateProfile(current.ID)
	require.NoError(t, err)
	assert.Equal(t, current.ID, state.ActiveProfileID)
	assertFileContents(t, filepath.Join(environment.active["saves"], "current.zip"), "current save after playing")
	assertFileContents(t, filepath.Join(environment.active["mods"], "new-mod_1.0.0.zip"), "new active mod")
}

func TestActivateProfileRestoresPreviousDataWhenActivationFails(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := CreateProfile("Broken switch target", "", ProfileSourceClone)
	require.NoError(t, err)
	target := findProfileByName(t, state, "Broken switch target")
	current := findActiveProfile(t, state)
	prepareStoredTargetProfile(t, environment, target, "target")

	originalRename := profileRename
	failed := false
	profileRename = func(oldPath, newPath string) error {
		if !failed && strings.Contains(oldPath, ".profile-staging-") && filepath.Dir(newPath) == environment.active["config"] {
			failed = true
			return errors.New("simulated config activation failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { profileRename = originalRename })

	_, err = ActivateProfile(target.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "previous profile restored")
	assertFileContents(t, filepath.Join(environment.active["saves"], "current.zip"), "current save")
	assertFileContents(t, filepath.Join(environment.active["mods"], "current_1.0.0.zip"), "current mod")
	assertFileContents(t, filepath.Join(environment.active["config"], "server-settings.json"), `{"name":"current"}`)
	state, err = ListProfiles()
	require.NoError(t, err)
	assert.Equal(t, current.ID, state.ActiveProfileID)
}

func TestActivateProfileRestoresRecordedFactorioVersion(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := CreateProfile("Factorio 2.0", "", ProfileSourceClone)
	require.NoError(t, err)
	target := findProfileByName(t, state, "Factorio 2.0")
	prepareStoredTargetProfile(t, environment, target, "older")

	manifest, err := loadProfileManifest()
	require.NoError(t, err)
	index := profileIndex(manifest, target.ID)
	manifest.Profiles[index].InstalledVersion = "2.0.77"
	manifest.Profiles[index].ReleaseTarget = "stable"
	require.NoError(t, saveProfileManifest(manifest))

	state, err = ActivateProfile(target.ID)
	require.NoError(t, err)
	require.Len(t, environment.installLog, 1)
	assert.Equal(t, RuntimeState{ReleaseTarget: "stable", InstalledVersion: "2.0.77"}, environment.installLog[0])
	assert.Equal(t, "stable", environment.runtime.ReleaseTarget)
	assert.Equal(t, "2.0.77", environment.runtime.InstalledVersion)
	assert.Equal(t, target.ID, state.ActiveProfileID)
}

func TestProfileRenameAndDeleteRules(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	state, err := CreateProfile("Temporary", "", ProfileSourceClone)
	require.NoError(t, err)
	temporary := findProfileByName(t, state, "Temporary")
	active := findActiveProfile(t, state)

	state, err = UpdateProfile(temporary.ID, "Renamed", "Description")
	require.NoError(t, err)
	renamed := findProfileByName(t, state, "Renamed")
	assert.Equal(t, "Description", renamed.Description)

	_, err = UpdateProfile(temporary.ID, active.Name, "")
	assert.ErrorIs(t, err, ErrProfileNameConflict)
	_, err = DeleteProfile(active.ID)
	assert.ErrorIs(t, err, ErrActiveProfileDelete)

	state, err = DeleteProfile(temporary.ID)
	require.NoError(t, err)
	assert.Len(t, state.Profiles, 1)
	_, err = os.Stat(filepath.Join(environment.root, "profiles", temporary.ID))
	assert.True(t, os.IsNotExist(err))
}

func prepareStoredTargetProfile(t *testing.T, environment *profileTestEnvironment, profile Profile, marker string) {
	t.Helper()
	root := filepath.Join(environment.root, "profiles", profile.ID)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "saves")))
	require.NoError(t, os.RemoveAll(filepath.Join(root, "mods")))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "saves"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "mods"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "saves", marker+".zip"), []byte(marker+" save"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mods", "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "server-settings.json"), []byte(`{"name":"`+marker+`"}`), 0644))

	manifest, err := loadProfileManifest()
	require.NoError(t, err)
	index := profileIndex(manifest, profile.ID)
	require.NotEqual(t, -1, index)
	manifest.Profiles[index].SelectedSave = marker + ".zip"
	manifest.Profiles[index].SaveCount = 1
	manifest.Profiles[index].ModCount = 0
	require.NoError(t, saveProfileManifest(manifest))
}

func findProfileByName(t *testing.T, state ProfileState, name string) Profile {
	t.Helper()
	for _, profile := range state.Profiles {
		if profile.Name == name {
			return profile
		}
	}
	t.Fatalf("profile %q not found", name)
	return Profile{}
}

func findActiveProfile(t *testing.T, state ProfileState) Profile {
	t.Helper()
	for _, profile := range state.Profiles {
		if profile.Active {
			return profile
		}
	}
	t.Fatal("active profile not found")
	return Profile{}
}
