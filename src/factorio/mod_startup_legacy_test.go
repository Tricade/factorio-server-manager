package factorio

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

func TestMalformedLegacyModPackLeavesNoVisiblePackAndPreservesSource(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	source := filepath.Join(legacyDir, "broken.zip")
	original := []byte("not a zip archive")
	if err := os.WriteFile(source, original, 0600); err != nil {
		t.Fatal(err)
	}

	if err := prepareModStartup(config); err == nil {
		t.Fatal("expected malformed archive error")
	}
	if _, err := os.Stat(filepath.Join(config.FactorioModPackDir, "broken")); !os.IsNotExist(err) {
		t.Fatalf("failed conversion left visible pack: stat error = %v", err)
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("legacy source was not restored: %v", err)
	}
	if !bytes.Equal(contents, original) {
		t.Fatal("legacy source contents changed")
	}
}

func TestLegacyModPackMigrationPreservesRecoverableArchiveAndUnrelatedFiles(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	source := filepath.Join(legacyDir, "factory.zip")
	inner := makeTestModArchive(t, `{"name":"example","version":"1.0.0"}`)
	writeLegacyModPack(t, source, map[string][]byte{"example_1.0.0.zip": inner})
	unrelated := filepath.Join(legacyDir, "notes.txt")
	if err := os.WriteFile(unrelated, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := prepareModStartup(config); err != nil {
		t.Fatalf("prepare startup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.FactorioModPackDir, "factory", "example_1.0.0.zip")); err != nil {
		t.Fatalf("migrated mod archive missing: %v", err)
	}
	recovery := source + ".migrated"
	if _, err := os.Stat(recovery); err != nil {
		t.Fatalf("recoverable source archive missing: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("original source path should be renamed: %v", err)
	}
	if contents, err := os.ReadFile(unrelated); err != nil || string(contents) != "keep me" {
		t.Fatalf("unrelated legacy file changed: contents=%q error=%v", contents, err)
	}
	if err := prepareModStartup(config); err != nil {
		t.Fatalf("second startup should be idempotent: %v", err)
	}
}

func TestModStartupReturnsPreparationPathErrors(t *testing.T) {
	config, _ := newLegacyModStartupConfig(t)
	if err := os.WriteFile(config.FactorioModPackDir, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareModStartup(config); err == nil {
		t.Fatal("expected error for an invalid mod-pack destination")
	}
}

func TestLegacyPackExistingDestinationRetainsOriginalSource(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	if err := os.MkdirAll(config.FactorioModPackDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(config.FactorioModPackDir, "existing"), 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(legacyDir, "existing.zip")
	contents := []byte("source archive")
	if err := os.WriteFile(source, contents, 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareModStartup(config); err == nil {
		t.Fatal("expected conflict with existing destination")
	}
	if actual, err := os.ReadFile(source); err != nil || !bytes.Equal(actual, contents) {
		t.Fatalf("original archive not preserved: contents=%q error=%v", actual, err)
	}
}

func TestInterruptedLegacyPackCommitFinalizesPendingSourceOnRetry(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	if err := os.MkdirAll(config.FactorioModPackDir, 0700); err != nil {
		t.Fatal(err)
	}
	activatedPack := filepath.Join(config.FactorioModPackDir, "factory")
	if err := os.Mkdir(activatedPack, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activatedPack, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(legacyDir, "factory.zip.migrating")
	if err := os.WriteFile(pending, []byte("preserved original archive"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := prepareModStartup(config); err == nil {
		t.Fatal("expected explicit recovery error when pending source and destination both exist")
	}
	if _, err := os.Stat(pending); err != nil {
		t.Fatalf("pending source was not preserved: %v", err)
	}
	entries, err := os.ReadDir(activatedPack)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "mod-list.json" {
		t.Fatalf("recovery inspection mutated destination: entries = %v", entries)
	}
}

func TestLegacyPackNameCanContainZipSuffix(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	source := filepath.Join(legacyDir, "factory.zip.zip")
	inner := makeTestModArchive(t, `{"name":"example","version":"1.0.0"}`)
	writeLegacyModPack(t, source, map[string][]byte{"example_1.0.0.zip": inner})
	if err := prepareModStartup(config); err != nil {
		t.Fatalf("migrate archive whose pack name ends in .zip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.FactorioModPackDir, "factory.zip")); err != nil {
		t.Fatalf("pack name was incorrectly truncated: %v", err)
	}
	if _, err := os.Stat(source + ".migrated"); err != nil {
		t.Fatalf("recoverable source archive missing: %v", err)
	}
}

func TestLegacyPackPendingCollisionPreservesBothSources(t *testing.T) {
	config, legacyDir := newLegacyModStartupConfig(t)
	source := filepath.Join(legacyDir, "factory.zip")
	pending := source + ".migrating"
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pending, []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareModStartup(config); err == nil {
		t.Fatal("expected collision with existing pending archive")
	}
	for path, want := range map[string]string{source: "original", pending: "pending"} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != want {
			t.Fatalf("archive %q changed: contents=%q error=%v", path, contents, err)
		}
	}
}

func newLegacyModStartupConfig(t *testing.T) (bootstrap.Config, string) {
	t.Helper()
	root := t.TempDir()
	factorioDir := filepath.Join(root, "factorio")
	legacyDir := filepath.Join(factorioDir, "modpacks")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatal(err)
	}
	return bootstrap.Config{
		FactorioDir:        factorioDir,
		FactorioModsDir:    filepath.Join(factorioDir, "mods"),
		FactorioModPackDir: filepath.Join(root, "mod_packs"),
	}, legacyDir
}

func makeTestModArchive(t *testing.T, infoJSON string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("info.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(infoJSON)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func writeLegacyModPack(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, contents := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
