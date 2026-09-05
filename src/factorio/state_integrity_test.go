package factorio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAtomicFileReplacementRestoresOriginalWhenActivationFails(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "mod-list.json")
	temporary := filepath.Join(directory, "replacement.tmp")
	if err := os.WriteFile(destination, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}

	originalRename := atomicFileRename
	activationAttempts := 0
	atomicFileRename = func(oldPath, newPath string) error {
		if oldPath == temporary && newPath == destination {
			activationAttempts++
			return errors.New("simulated activation failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { atomicFileRename = originalRename })

	err := replaceFileWithRollback(temporary, destination)
	if err == nil || activationAttempts != 2 {
		t.Fatalf("expected failed activation with rollback, attempts=%d error=%v", activationAttempts, err)
	}
	contents, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "original" {
		t.Fatalf("rollback did not preserve original contents: %q", contents)
	}
}

func TestModPackActivationFailureRestoresActiveMods(t *testing.T) {
	root := t.TempDir()
	activeMods := filepath.Join(root, "active-mods")
	packDirectory := filepath.Join(root, "pack")
	for _, directory := range []string{activeMods, packDirectory} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(activeMods, "old-marker.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDirectory, "new-marker.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDirectory, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	pack, err := newModPack(packDirectory)
	if err != nil {
		t.Fatal(err)
	}
	originalActiveDirectories := profileActiveDirectories
	originalRename := profileRename
	originalServer := GetFactorioServer().Snapshot()
	profileActiveDirectories = func() map[string]string { return map[string]string{"mods": activeMods} }
	failed := false
	profileRename = func(oldPath, newPath string) error {
		if !failed && strings.Contains(filepath.Base(filepath.Dir(oldPath)), ".mods-modpack-staging-") && filepath.Dir(newPath) == activeMods {
			failed = true
			return errors.New("simulated staged copy failure")
		}
		return os.Rename(oldPath, newPath)
	}
	SetFactorioServer(Server{})
	t.Cleanup(func() {
		profileActiveDirectories = originalActiveDirectories
		profileRename = originalRename
		SetFactorioServer(Server{
			Savefile: originalServer.Savefile, BindIP: originalServer.BindIP, Port: originalServer.Port,
			Running: originalServer.Running, Stopping: originalServer.Stopping,
			Version: originalServer.Version, BaseModVersion: originalServer.BaseModVersion,
			Settings: map[string]interface{}{},
		})
	})

	if err := pack.LoadModPack(); err == nil {
		t.Fatal("expected staged activation failure")
	}
	contents, err := os.ReadFile(filepath.Join(activeMods, "old-marker.txt"))
	if err != nil || string(contents) != "old" {
		t.Fatalf("active mods were not restored: contents=%q error=%v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(activeMods, "new-marker.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partially activated mod pack remained visible: %v", err)
	}
}

func TestModPackCreationAndLoadingCarryProfileModSettings(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source-mods")
	pack := filepath.Join(root, "pack")
	destination := filepath.Join(root, "active-mods")
	for _, directory := range []string{source, destination} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, directory := range []string{source, destination} {
		if err := os.WriteFile(filepath.Join(directory, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	packSettings := []byte("settings stored with the mod pack")
	if err := os.WriteFile(filepath.Join(source, "mod-settings.dat"), packSettings, 0644); err != nil {
		t.Fatal(err)
	}
	// CreateModPack uses the same complete-directory copy so each reusable pack
	// keeps the setting values that belong to its saved mod combination.
	if err := copyProfileDirectory(source, pack); err != nil {
		t.Fatal(err)
	}
	createdSettings, err := os.ReadFile(filepath.Join(pack, "mod-settings.dat"))
	if err != nil || string(createdSettings) != string(packSettings) {
		t.Fatalf("mod pack creation did not copy settings: contents=%q error=%v", createdSettings, err)
	}
	if err := os.WriteFile(filepath.Join(destination, "mod-settings.dat"), []byte("old active settings"), 0644); err != nil {
		t.Fatal(err)
	}

	swap, err := prepareModPackDirectorySwap(pack, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer swap.cleanup()
	if err := swap.activate(); err != nil {
		t.Fatal(err)
	}
	if err := swap.commit(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(destination, "mod-settings.dat"))
	if err != nil || string(contents) != string(packSettings) {
		t.Fatalf("mod pack settings were not activated: contents=%q error=%v", contents, err)
	}
}

func TestDeleteAllModsActivationFailureRestoresActiveDirectory(t *testing.T) {
	root := t.TempDir()
	activeMods := filepath.Join(root, "mods")
	if err := os.MkdirAll(activeMods, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "keep.zip"), []byte("existing archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeMods, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	originalRename := profileRename
	failed := false
	profileRename = func(oldPath, newPath string) error {
		if !failed && strings.Contains(filepath.Base(filepath.Dir(oldPath)), ".mods-empty-staging-") && filepath.Dir(newPath) == activeMods {
			failed = true
			return errors.New("simulated empty-directory activation failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { profileRename = originalRename })

	if err := replaceActiveModsWithEmptyDirectory(activeMods); err == nil {
		t.Fatal("expected transactional empty-directory activation to fail")
	}
	contents, err := os.ReadFile(filepath.Join(activeMods, "keep.zip"))
	if err != nil || string(contents) != "existing archive" {
		t.Fatalf("failed delete-all activation did not restore existing mods: contents=%q error=%v", contents, err)
	}
}

// This test is enabled by the Docker integration command. Its destination is
// a nested tmpfs mount matching the split /opt/factorio/mods mount used by the
// Unraid template.
func TestDeleteAllModsAtMountedDestination(t *testing.T) {
	activeMods := os.Getenv("FSM_TEST_MOUNTED_MODS_DIR")
	if activeMods == "" {
		t.Skip("set FSM_TEST_MOUNTED_MODS_DIR to a dedicated mount point")
	}
	if err := os.WriteFile(filepath.Join(activeMods, "remove-me.zip"), []byte("old mod"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := replaceActiveModsWithEmptyDirectory(activeMods); err != nil {
		t.Fatalf("replace mods at mounted destination: %v", err)
	}
	if _, err := os.Stat(filepath.Join(activeMods, "remove-me.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old mod was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(activeMods, "mod-list.json")); err != nil {
		t.Fatalf("base-only mod list was not activated: %v", err)
	}
	entries, err := os.ReadDir(activeMods)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".mods-empty-") {
			t.Fatalf("temporary mods directory was left behind: %s", entry.Name())
		}
	}
}

// This test is enabled by the Docker integration command. It verifies the
// exact file mutation used by the startup-settings editor inside the split
// /opt/factorio/mods mount.
func TestModStartupSettingsAtomicWriteAtMountedDestination(t *testing.T) {
	activeMods := os.Getenv("FSM_TEST_MOUNTED_MODS_DIR")
	if activeMods == "" {
		t.Skip("set FSM_TEST_MOUNTED_MODS_DIR to a dedicated mount point")
	}
	path := filepath.Join(activeMods, "mod-settings.dat")
	if err := os.WriteFile(path, []byte("previous settings"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomically(path, []byte("validated replacement settings"), 0600); err != nil {
		t.Fatalf("atomically replace mounted mod settings: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "validated replacement settings" {
		t.Fatalf("mounted mod settings were not committed: contents=%q error=%v", contents, err)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("inspect mounted mod settings permissions: %v", err)
	} else if info.Mode().Perm() != 0600 {
		t.Fatalf("mounted mod settings permissions are %o, want 600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(activeMods)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".fsm-write-") || strings.HasPrefix(entry.Name(), ".fsm-backup-") {
			t.Fatalf("startup-settings transaction file was left behind: %s", entry.Name())
		}
	}
}

func TestActiveModMutationSerializesAtomicServerStartClaim(t *testing.T) {
	root := t.TempDir()
	activeMods := filepath.Join(root, "mods")
	if err := os.MkdirAll(activeMods, 0755); err != nil {
		t.Fatal(err)
	}
	modListPath := filepath.Join(activeMods, "mod-list.json")
	if err := os.WriteFile(modListPath, []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	list, err := newModSimpleList(activeMods)
	if err != nil {
		t.Fatal(err)
	}

	originalActiveDirectories := profileActiveDirectories
	originalAtomicRename := atomicFileRename
	stateMutex.RLock()
	originalServer := instantiated
	stateMutex.RUnlock()
	profileActiveDirectories = func() map[string]string { return map[string]string{"mods": activeMods} }
	SetFactorioServer(Server{Settings: map[string]interface{}{"name": "serialized-mod-mutation"}})
	t.Cleanup(func() {
		atomicFileRename = originalAtomicRename
		profileActiveDirectories = originalActiveDirectories
		SetFactorioServer(originalServer)
	})

	mutationReachedDisk := make(chan struct{})
	releaseMutation := make(chan struct{})
	var barrierOnce sync.Once
	var releaseOnce sync.Once
	releaseMutationBarrier := func() { releaseOnce.Do(func() { close(releaseMutation) }) }
	t.Cleanup(releaseMutationBarrier)
	atomicFileRename = func(oldPath, newPath string) error {
		if newPath == modListPath && strings.Contains(filepath.Base(oldPath), ".fsm-write-") {
			barrierOnce.Do(func() {
				close(mutationReachedDisk)
				<-releaseMutation
			})
		}
		return os.Rename(oldPath, newPath)
	}

	mutationResult := make(chan error, 1)
	go func() { mutationResult <- list.SetModEnabled("base", false) }()
	select {
	case <-mutationReachedDisk:
	case <-time.After(2 * time.Second):
		t.Fatal("active mod mutation did not reach its atomic activation barrier")
	}

	type startOutcome struct {
		result <-chan error
		err    error
	}
	startClaim := make(chan startOutcome, 1)
	server := GetFactorioServer()
	go func() {
		result, startErr := server.Start("127.0.0.1", 34197, "world.zip")
		startClaim <- startOutcome{result: result, err: startErr}
	}()
	select {
	case outcome := <-startClaim:
		t.Fatalf("server start claimed state before the active mod mutation committed: %v", outcome.err)
	case <-time.After(100 * time.Millisecond):
	}

	releaseMutationBarrier()
	if err := <-mutationResult; err != nil {
		t.Fatalf("active mod mutation failed: %v", err)
	}
	var outcome startOutcome
	select {
	case outcome = <-startClaim:
	case <-time.After(2 * time.Second):
		t.Fatal("server start did not claim state after the mod mutation committed")
	}
	if outcome.err != nil {
		t.Fatalf("server start claim failed after serialized mutation: %v", outcome.err)
	}
	select {
	case <-outcome.result:
	case <-time.After(2 * time.Second):
		t.Fatal("test server start did not release its lifecycle reservation")
	}
}

func TestPortalBoundsAndDownloadPathValidation(t *testing.T) {
	if _, err := readBoundedBody(strings.NewReader("1234"), 3); err == nil {
		t.Fatal("oversized portal response was accepted")
	}
	if _, err := authenticatedModDownloadURL("https://example.invalid/steal", Credentials{Username: "u", Userkey: "k"}); err == nil {
		t.Fatal("absolute portal download URL was accepted")
	}
	if _, err := authenticatedModDownloadURL("/download/example/release?token=attacker", Credentials{Username: "u", Userkey: "k"}); err == nil {
		t.Fatal("portal download URL with caller-supplied query was accepted")
	}
}
