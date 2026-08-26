package factorio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDiscoverSaveModsWithFactorioUsesCurrentSaveState(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	for _, name := range append([]string{"base"}, spaceAgeFeatureMods...) {
		writeSaveImportBuiltIn(t, factorioDirectory, name)
	}
	savePath := filepath.Join(root, "Space Age First Run.zip")
	copySaveImportFixture(t, filepath.Join("..", "factorio_testfiles", "test_1_1_14.zip"), savePath)
	originalContents, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatal(err)
	}

	var workspace string
	credentials := Credentials{Username: "portal-user", Userkey: "portal-token"}
	runner := func(_ time.Duration, args []string) error {
		modsDirectory := saveImportArgument(t, args, "--mod-directory")
		synchronizedSave := saveImportArgument(t, args, "--sync-mods")
		configPath := saveImportArgument(t, args, "--config")
		workspace = filepath.Dir(modsDirectory)
		if synchronizedSave == savePath {
			t.Fatal("Factorio received the playable save instead of an isolated copy")
		}
		if filepath.Dir(synchronizedSave) != workspace || filepath.Dir(configPath) != workspace {
			t.Fatalf("inspection inputs escaped the isolated workspace: args=%#v", args)
		}
		if err := ValidateSaveArchive(synchronizedSave); err != nil {
			t.Fatalf("isolated save copy is invalid: %v", err)
		}
		playerDataPath := filepath.Join(workspace, "write-data", "player-data.json")
		playerDataContents, err := os.ReadFile(playerDataPath)
		if err != nil {
			t.Fatalf("read isolated Factorio credentials: %v", err)
		}
		playerDataInfo, err := os.Lstat(playerDataPath)
		if err != nil {
			t.Fatalf("inspect isolated Factorio credentials: %v", err)
		}
		if !playerDataInfo.Mode().IsRegular() || playerDataInfo.Mode()&os.ModeSymlink != 0 {
			t.Fatal("isolated Factorio credentials are not a regular file")
		}
		if runtime.GOOS != "windows" && playerDataInfo.Mode().Perm() != 0600 {
			t.Fatalf("isolated Factorio credentials have permissions %04o, want 0600", playerDataInfo.Mode().Perm())
		}
		var playerData isolatedFactorioPlayerData
		if err := json.Unmarshal(playerDataContents, &playerData); err != nil {
			t.Fatalf("decode isolated Factorio credentials: %v", err)
		}
		if playerData.ServiceUsername != credentials.Username || playerData.ServiceToken != credentials.Userkey {
			t.Fatal("isolated Factorio credentials do not match the stored Mod Portal credential")
		}
		for _, arg := range args {
			if strings.Contains(arg, credentials.Username) || strings.Contains(arg, credentials.Userkey) {
				t.Fatal("Mod Portal credentials were exposed in Factorio process arguments")
			}
		}
		contents := []byte(`{"mods":[` +
			`{"name":"base","enabled":true,"version":"2.0.77"},` +
			`{"name":"elevated-rails","enabled":true,"version":"2.0.77"},` +
			`{"name":"quality","enabled":true,"version":"2.0.77"},` +
			`{"name":"space-age","enabled":true,"version":"2.0.77"},` +
			`{"name":"community-mod","enabled":false,"version":"1.2.3"}` +
			`]}`)
		return os.WriteFile(filepath.Join(modsDirectory, "mod-list.json"), contents, 0600)
	}

	mods, err := discoverSaveModsWithFactorio(savePath, factorioDirectory, credentials, runner)
	if err != nil {
		t.Fatalf("discover current save mods: %v", err)
	}
	expected := []Mod{
		{Name: "base", Version: Version{2, 0, 77, 0}},
		{Name: "elevated-rails", Version: Version{2, 0, 77, 0}},
		{Name: "quality", Version: Version{2, 0, 77, 0}},
		{Name: "space-age", Version: Version{2, 0, 77, 0}},
		{Name: "community-mod", Version: Version{1, 2, 3, 0}},
	}
	assertSaveImportMods(t, mods, expected)
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("save-mod inspection workspace was not removed: %v", err)
	}
	currentContents, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentContents) != string(originalContents) {
		t.Fatal("save-mod inspection changed the source save")
	}
}

func TestReadSynchronizedSaveModsKeepsMissingCommunityModsAndDisabledBaseMode(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	for _, name := range append([]string{"base"}, spaceAgeFeatureMods...) {
		writeSaveImportBuiltIn(t, factorioDirectory, name)
	}
	modListPath := filepath.Join(root, "mod-list.json")
	contents := []byte(`{"mods":[` +
		`{"name":"base","enabled":true,"version":"2.1.16"},` +
		`{"name":"elevated-rails","enabled":false,"version":"2.1.16"},` +
		`{"name":"quality","enabled":false,"version":"2.1.16"},` +
		`{"name":"space-age","enabled":false,"version":"2.1.16"},` +
		`{"name":"missing-community-mod","enabled":false,"version":"4.5.6"}` +
		`]}`)
	if err := os.WriteFile(modListPath, contents, 0600); err != nil {
		t.Fatal(err)
	}

	mods, err := readSynchronizedSaveMods(modListPath, factorioDirectory)
	if err != nil {
		t.Fatalf("read synchronized mods: %v", err)
	}
	assertSaveImportMods(t, mods, []Mod{
		{Name: "base", Version: Version{2, 1, 16, 0}},
		{Name: "missing-community-mod", Version: Version{4, 5, 6, 0}},
	})
}

func TestDiscoverSaveModsWithFactorioFailureDoesNotLeaveWorkspace(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	writeSaveImportBuiltIn(t, factorioDirectory, "base")
	savePath := filepath.Join(root, "save.zip")
	copySaveImportFixture(t, filepath.Join("..", "factorio_testfiles", "test_1_1_14.zip"), savePath)
	var workspace string
	expected := errors.New("Factorio inspection failed")
	_, err := discoverSaveModsWithFactorio(savePath, factorioDirectory, Credentials{Username: "portal-user", Userkey: "portal-token"}, func(_ time.Duration, args []string) error {
		workspace = filepath.Dir(saveImportArgument(t, args, "--mod-directory"))
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("unexpected inspection error: %v", err)
	}
	if _, statErr := os.Stat(workspace); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed inspection workspace was not removed: %v", statErr)
	}
}

func TestDiscoverSaveModsWithFactorioRequiresCompletePortalCredentials(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	writeSaveImportBuiltIn(t, factorioDirectory, "base")
	savePath := filepath.Join(root, "save.zip")
	copySaveImportFixture(t, filepath.Join("..", "factorio_testfiles", "test_1_1_14.zip"), savePath)

	for name, credentials := range map[string]Credentials{
		"missing":  {},
		"username": {Username: "portal-user"},
		"token":    {Userkey: "portal-token"},
	} {
		t.Run(name, func(t *testing.T) {
			runnerCalled := false
			_, err := discoverSaveModsWithFactorio(savePath, factorioDirectory, credentials, func(_ time.Duration, _ []string) error {
				runnerCalled = true
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), "authentication is required") {
				t.Fatalf("incomplete credentials were accepted: %v", err)
			}
			if runnerCalled {
				t.Fatal("Factorio ran without complete Mod Portal credentials")
			}
		})
	}
}

func TestReadSynchronizedSaveModsRejectsIncompleteOrOversizedResults(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	writeSaveImportBuiltIn(t, factorioDirectory, "base")
	modListPath := filepath.Join(root, "mod-list.json")
	if err := os.WriteFile(modListPath, []byte(`{"mods":[{"name":"community-mod","enabled":false,"version":"1.2.3"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSynchronizedSaveMods(modListPath, factorioDirectory); err == nil || !strings.Contains(err.Error(), "base mod") {
		t.Fatalf("synchronized result without base was accepted: %v", err)
	}
	if err := os.WriteFile(modListPath, []byte(strings.Repeat("x", maximumSaveModSyncListBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSynchronizedSaveMods(modListPath, factorioDirectory); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized synchronized result was accepted: %v", err)
	}
}

func TestReplaceModsFromSaveEnablesInstalledSpaceAgeFeatures(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	activeMods := filepath.Join(factorioDirectory, "mods")
	if err := os.MkdirAll(activeMods, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range spaceAgeFeatureMods {
		directory := filepath.Join(factorioDirectory, "data", name)
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		contents := []byte(`{"name":"` + name + `","version":"2.0.72"}`)
		if err := os.WriteFile(filepath.Join(directory, "info.json"), contents, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(activeMods, "old-marker.txt"), []byte("old mods"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true},{"name":"elevated-rails","enabled":false},{"name":"quality","enabled":false},{"name":"space-age","enabled":false}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	requested := []Mod{
		{Name: "base", Version: Version{2, 0, 72, 0}},
		{Name: "elevated-rails", Version: Version{2, 0, 72, 0}},
		{Name: "quality", Version: Version{2, 0, 72, 0}},
		{Name: "space-age", Version: Version{2, 0, 72, 0}},
	}
	portalCalled := false
	result, err := replaceModsFromSave(factorioDirectory, activeMods, requested, func(_ *Mods, _ Mod) error {
		portalCalled = true
		return errors.New("built-in mod was sent to the portal")
	})
	if err != nil {
		t.Fatalf("replace mods from Space Age save: %v", err)
	}
	if portalCalled {
		t.Fatal("Space Age feature mods must not be downloaded from the Mod Portal")
	}
	if len(result.ModsResult) != 0 {
		t.Fatalf("built-in mods unexpectedly appeared as downloaded archives: %#v", result.ModsResult)
	}
	if _, err := os.Stat(filepath.Join(activeMods, "old-marker.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous mod entry was not replaced: %v", err)
	}
	list, err := newModSimpleList(activeMods)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range spaceAgeFeatureMods {
		if !list.IsEnabled(name) {
			t.Errorf("Space Age feature %s was not enabled", name)
		}
	}
	assertNoSaveImportTransactionEntries(t, activeMods)
}

func TestReplaceModsFromBaseSaveExplicitlyDisablesSpaceAgeFeatures(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	activeMods := filepath.Join(factorioDirectory, "mods")
	if err := os.MkdirAll(activeMods, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true},{"name":"elevated-rails","enabled":true},{"name":"quality","enabled":true},{"name":"space-age","enabled":true}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := replaceModsFromSave(factorioDirectory, activeMods, []Mod{
		{Name: "base", Version: Version{2, 0, 72, 0}},
	}, func(_ *Mods, _ Mod) error {
		return errors.New("base-only import called the Mod Portal")
	})
	if err != nil {
		t.Fatalf("replace mods from base-game save: %v", err)
	}
	list, err := newModSimpleList(activeMods)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range spaceAgeFeatureMods {
		if list.IsEnabled(name) {
			t.Errorf("Space Age feature %s remained enabled", name)
		}
	}
	assertNoSaveImportTransactionEntries(t, activeMods)
}

func TestReplaceModsFromSaveFailurePreservesActiveMods(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	activeMods := filepath.Join(factorioDirectory, "mods")
	if err := os.MkdirAll(activeMods, 0755); err != nil {
		t.Fatal(err)
	}
	originalList := []byte(`{"mods":[{"name":"base","enabled":true},{"name":"quality","enabled":true}]}`)
	if err := os.WriteFile(filepath.Join(activeMods, "mod-list.json"), originalList, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "old-marker.txt"), []byte("old mods"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := replaceModsFromSave(factorioDirectory, activeMods, []Mod{
		{Name: "base", Version: Version{2, 0, 72, 0}},
		{Name: "community-mod", Version: Version{1, 2, 3, 0}},
	}, func(_ *Mods, mod Mod) error {
		return errors.New("portal unavailable for " + mod.Name)
	})
	if err == nil {
		t.Fatal("expected staged portal installation to fail")
	}
	marker, readErr := os.ReadFile(filepath.Join(activeMods, "old-marker.txt"))
	if readErr != nil || string(marker) != "old mods" {
		t.Fatalf("previous mod archive was not preserved: contents=%q error=%v", marker, readErr)
	}
	list, readErr := os.ReadFile(filepath.Join(activeMods, "mod-list.json"))
	if readErr != nil || string(list) != string(originalList) {
		t.Fatalf("previous mod list was not preserved: contents=%q error=%v", list, readErr)
	}
	assertNoSaveImportTransactionEntries(t, activeMods)
}

func TestReplaceModsFromSaveSkipsUnavailablePortalMods(t *testing.T) {
	root := t.TempDir()
	factorioDirectory := filepath.Join(root, "factorio")
	activeMods := filepath.Join(factorioDirectory, "mods")
	if err := os.MkdirAll(activeMods, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "old-marker.txt"), []byte("old mods"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	requested := []Mod{
		{Name: "base", Version: Version{2, 0, 72, 0}},
		{Name: "available-mod", Version: Version{1, 2, 3, 0}},
		{Name: "Aircraft", Version: Version{1, 8, 6, 0}},
		{Name: "Laser_Tanks_kr", Version: Version{1, 0, 1, 0}},
	}
	result, err := replaceModsFromSave(factorioDirectory, activeMods, requested, func(staged *Mods, mod Mod) error {
		switch mod.Name {
		case "available-mod":
			return installSaveImportTestMod(t, staged, mod)
		case "Aircraft":
			return fmt.Errorf("%w: requested version", errModPortalReleaseUnavailable)
		case "Laser_Tanks_kr":
			return fmt.Errorf("%w: portal archive metadata", errModArchiveIdentityMismatch)
		default:
			return fmt.Errorf("unexpected portal mod %s", mod.Name)
		}
	})
	if err != nil {
		t.Fatalf("replace mods with unavailable portal releases: %v", err)
	}
	if len(result.ModsResult) != 1 || result.ModsResult[0].Name != "available-mod" {
		t.Fatalf("available mod was not activated: %#v", result.ModsResult)
	}
	expectedSkipped := []SaveModImportSkipped{
		{Name: "Aircraft", Version: Version{1, 8, 6, 0}, Reason: saveModImportSkipReleaseUnavailable},
		{Name: "Laser_Tanks_kr", Version: Version{1, 0, 1, 0}, Reason: saveModImportSkipArchiveIdentityMismatch},
	}
	if len(result.Skipped) != len(expectedSkipped) {
		t.Fatalf("unexpected skipped mods: %#v", result.Skipped)
	}
	for index, expected := range expectedSkipped {
		if result.Skipped[index] != expected {
			t.Errorf("skipped mod %d = %#v, want %#v", index, result.Skipped[index], expected)
		}
	}
	if _, err := os.Stat(filepath.Join(activeMods, "old-marker.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous mod entry was not replaced: %v", err)
	}
	assertNoSaveImportTransactionEntries(t, activeMods)
}

func TestInstallSaveModFromPortalClassifiesOnlyPermanentUnavailability(t *testing.T) {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/mods/Aircraft/full":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Aircraft","releases":[{"version":"1.9.0","download_url":"/download/Aircraft/current","file_name":"Aircraft_1.9.0.zip","info_json":{"factorio_version":"2.0"}}]}`))
		case "/api/mods/removed-mod/full":
			w.WriteHeader(http.StatusGone)
		case "/api/mods/portal-failure/full":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer portal.Close()

	originalBaseURL := modPortalBaseURL
	modPortalBaseURL = portal.URL
	defer func() { modPortalBaseURL = originalBaseURL }()

	for name, requested := range map[string]Mod{
		"missing exact version": {Name: "Aircraft", Version: Version{1, 8, 6, 0}},
		"removed mod":           {Name: "removed-mod", Version: Version{1, 0, 0, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			err := installSaveModFromPortal(nil, requested)
			if !errors.Is(err, errModPortalReleaseUnavailable) {
				t.Fatalf("unavailable release was not classified for skipping: %v", err)
			}
		})
	}

	err := installSaveModFromPortal(nil, Mod{Name: "portal-failure", Version: Version{1, 0, 0, 0}})
	if err == nil || errors.Is(err, errModPortalReleaseUnavailable) {
		t.Fatalf("transient portal failure was treated as skippable: %v", err)
	}
}

func TestSaveModImportResultEncodesSkippedVersionsForTheUI(t *testing.T) {
	result := SaveModImportResult{
		ModsResult: []ModsResult{},
		Skipped: []SaveModImportSkipped{{
			Name:    "Aircraft",
			Version: Version{1, 8, 6, 0},
			Reason:  saveModImportSkipReleaseUnavailable,
		}},
	}
	contents, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Mods    []ModsResult `json:"mods"`
		Skipped []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Reason  string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(contents, &response); err != nil {
		t.Fatal(err)
	}
	if response.Mods == nil {
		t.Fatal("mods encoded as null instead of an array")
	}
	if len(response.Skipped) != 1 || response.Skipped[0].Name != "Aircraft" || response.Skipped[0].Version != "1.8.6.0" || response.Skipped[0].Reason != saveModImportSkipReleaseUnavailable {
		t.Fatalf("unexpected save-mod import response: %s", contents)
	}
}

// This test is enabled by the Docker integration command. Its destination is
// a nested tmpfs mount matching the split /opt/factorio/mods mount used by the
// Unraid template.
func TestReplaceModsFromSaveAtMountedDestination(t *testing.T) {
	factorioDirectory := os.Getenv("FSM_TEST_MOUNTED_FACTORIO_DIR")
	activeMods := os.Getenv("FSM_TEST_MOUNTED_MODS_DIR")
	if factorioDirectory == "" || activeMods == "" {
		t.Skip("set the mounted Factorio and mods directories")
	}
	if err := replaceActiveModsWithEmptyDirectory(activeMods); err != nil {
		t.Fatalf("initialize mounted mods fixture: %v", err)
	}
	for _, name := range spaceAgeFeatureMods {
		directory := filepath.Join(factorioDirectory, "data", name)
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		contents := []byte(`{"name":"` + name + `","version":"2.0.72"}`)
		if err := os.WriteFile(filepath.Join(directory, "info.json"), contents, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(activeMods, "old-marker.txt"), []byte("old mods"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	requested := []Mod{{Name: "base", Version: Version{2, 0, 72, 0}}}
	for _, name := range spaceAgeFeatureMods {
		requested = append(requested, Mod{Name: name, Version: Version{2, 0, 72, 0}})
	}
	if _, err := replaceModsFromSave(factorioDirectory, activeMods, requested, func(_ *Mods, _ Mod) error {
		return errors.New("built-in mod was sent to the portal")
	}); err != nil {
		t.Fatalf("replace mounted mods from Space Age save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(activeMods, "old-marker.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous mounted mod entry was not replaced: %v", err)
	}
	list, err := newModSimpleList(activeMods)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range spaceAgeFeatureMods {
		if !list.IsEnabled(name) {
			t.Errorf("mounted Space Age feature %s was not enabled", name)
		}
	}
	assertNoSaveImportTransactionEntries(t, activeMods)
}

func assertNoSaveImportTransactionEntries(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".mods-save-import-") {
			t.Fatalf("save-import transaction entry was left behind: %s", entry.Name())
		}
	}
}

func writeSaveImportBuiltIn(t *testing.T, factorioDirectory, name string) {
	t.Helper()
	directory := filepath.Join(factorioDirectory, "data", name)
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	contents := []byte(`{"name":"` + name + `","version":"2.0.77"}`)
	if err := os.WriteFile(filepath.Join(directory, "info.json"), contents, 0600); err != nil {
		t.Fatal(err)
	}
}

func copySaveImportFixture(t *testing.T, source, destination string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, contents, 0600); err != nil {
		t.Fatal(err)
	}
}

func saveImportArgument(t *testing.T, args []string, name string) string {
	t.Helper()
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	t.Fatalf("Factorio argument %s is missing from %#v", name, args)
	return ""
}

func installSaveImportTestMod(t *testing.T, staged *Mods, mod Mod) error {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create(mod.Name + "/info.json")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(ModInfo{
		Name:    mod.Name,
		Version: mod.Version.ReleaseString(),
		Title:   mod.Name,
		Author:  "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(metadata); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return staged.createMod(mod.Name, mod.Name+"_"+mod.Version.ReleaseString()+".zip", bytes.NewReader(archive.Bytes()))
}

func assertSaveImportMods(t *testing.T, actual, expected []Mod) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("unexpected synchronized mods: got %#v, want %#v", actual, expected)
	}
	for index := range expected {
		if actual[index].Name != expected[index].Name || !actual[index].Version.Equals(expected[index].Version) {
			t.Fatalf("unexpected synchronized mod at %d: got %#v, want %#v", index, actual[index], expected[index])
		}
	}
}
