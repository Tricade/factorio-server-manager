package factorio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
