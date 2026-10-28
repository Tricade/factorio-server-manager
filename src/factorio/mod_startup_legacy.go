package factorio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	legacyModPackPendingSuffix  = ".migrating"
	legacyModPackDoneSuffix     = ".migrated"
	maximumLegacyModPackEntries = 4096
)

// prepareModStartup ensures the standard mod directories exist and imports old
// flat mod-pack ZIPs. Legacy archives remain recoverable as .zip.migrated files.
func prepareModStartup(config bootstrap.Config) error {
	factorioDirInfo, err := os.Stat(config.FactorioDir)
	if err != nil {
		return fmt.Errorf("stat Factorio directory %q: %w", config.FactorioDir, err)
	}
	if !factorioDirInfo.IsDir() {
		return fmt.Errorf("the Factorio path %q is not a directory", config.FactorioDir)
	}
	if err := os.MkdirAll(config.FactorioModsDir, factorioDirInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("create mods directory %q: %w", config.FactorioModsDir, err)
	}
	if err := os.MkdirAll(config.FactorioModPackDir, factorioDirInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("create mod pack directory %q: %w", config.FactorioModPackDir, err)
	}

	legacyDir := filepath.Join(config.FactorioDir, "modpacks")
	info, err := os.Lstat(legacyDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy mod pack directory %q: %w", legacyDir, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("legacy mod pack path %q is not a directory", legacyDir)
	}
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		return fmt.Errorf("read legacy mod pack directory %q: %w", legacyDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		pending := strings.HasSuffix(name, ".zip"+legacyModPackPendingSuffix)
		if filepath.Ext(name) == ".zip" || pending {
			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf("inspect legacy mod pack source %q: %w", filepath.Join(legacyDir, name), err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("legacy mod pack source %q is not a regular file", filepath.Join(legacyDir, name))
			}
		}
		if pending {
			if err := recoverLegacyModPack(config.FactorioModPackDir, filepath.Join(legacyDir, name), name); err != nil {
				return err
			}
			continue
		}
		if filepath.Ext(name) != ".zip" {
			continue
		}
		if err := migrateLegacyModPack(config.FactorioModPackDir, filepath.Join(legacyDir, name), name); err != nil {
			return err
		}
	}
	return nil
}

func recoverLegacyModPack(packRoot, pendingPath, pendingName string) error {
	baseName := strings.TrimSuffix(pendingName, legacyModPackPendingSuffix)
	packName := strings.TrimSuffix(baseName, ".zip")
	if err := ValidatePathElement(packName); err != nil {
		return fmt.Errorf("invalid legacy mod pack name %q: %w", packName, err)
	}
	target := filepath.Join(packRoot, packName)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("both pending legacy source %q and destination %q exist; inspect and resolve manually", pendingPath, target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect migrated mod pack %q: %w", target, err)
	}
	donePath := strings.TrimSuffix(pendingPath, legacyModPackPendingSuffix) + legacyModPackDoneSuffix
	if _, err := os.Lstat(donePath); err == nil {
		return fmt.Errorf("legacy recovery archive %q already exists; pending source retained at %q", donePath, pendingPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy recovery archive %q: %w", donePath, err)
	}
	if err := migratePendingLegacyModPack(packRoot, pendingPath, packName); err != nil {
		return err
	}
	return markLegacyModPackMigrated(pendingPath, donePath)
}

func migrateLegacyModPack(packRoot, sourcePath, sourceName string) error {
	packName := strings.TrimSuffix(sourceName, ".zip")
	if err := ValidatePathElement(packName); err != nil {
		return fmt.Errorf("invalid legacy mod pack name %q: %w", packName, err)
	}
	target := filepath.Join(packRoot, packName)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("legacy mod pack %q already has a destination; source retained at %q", packName, sourcePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect mod pack destination %q: %w", target, err)
	}
	pendingPath := sourcePath + legacyModPackPendingSuffix
	donePath := sourcePath + legacyModPackDoneSuffix
	if _, err := os.Lstat(donePath); err == nil {
		return fmt.Errorf("legacy recovery archive %q already exists; source retained at %q", donePath, sourcePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy recovery archive %q: %w", donePath, err)
	}
	if err := os.Link(sourcePath, pendingPath); err != nil {
		return fmt.Errorf("mark legacy mod pack %q for migration: %w", sourcePath, err)
	}
	if err := os.Remove(sourcePath); err != nil {
		if cleanupErr := os.Remove(pendingPath); cleanupErr != nil {
			return errors.Join(fmt.Errorf("remove original legacy mod pack path %q: %w", sourcePath, err), fmt.Errorf("remove pending link %q: %w", pendingPath, cleanupErr))
		}
		return fmt.Errorf("remove original legacy mod pack path %q: %w", sourcePath, err)
	}
	if err := migratePendingLegacyModPack(packRoot, pendingPath, packName); err != nil {
		if restoreErr := os.Link(pendingPath, sourcePath); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore source archive from %q to %q: %w", pendingPath, sourcePath, restoreErr))
		}
		if removeErr := os.Remove(pendingPath); removeErr != nil {
			return errors.Join(err, fmt.Errorf("remove pending source alias %q: %w", pendingPath, removeErr))
		}
		return err
	}
	if err := markLegacyModPackMigrated(pendingPath, donePath); err != nil {
		return err
	}
	return nil
}

func markLegacyModPackMigrated(pendingPath, donePath string) error {
	if _, err := os.Lstat(donePath); err == nil {
		return fmt.Errorf("legacy recovery archive %q already exists; pending source retained at %q", donePath, pendingPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect legacy recovery archive %q: %w", donePath, err)
	}
	if err := os.Link(pendingPath, donePath); err != nil {
		return fmt.Errorf("preserve migrated legacy archive %q: %w", pendingPath, err)
	}
	if err := os.Remove(pendingPath); err != nil {
		return fmt.Errorf("remove pending legacy archive alias %q: %w", pendingPath, err)
	}
	return nil
}

func migratePendingLegacyModPack(packRoot, pendingPath, packName string) error {
	target := filepath.Join(packRoot, packName)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("mod pack destination %q already exists; pending legacy source retained at %q", target, pendingPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect mod pack destination %q: %w", target, err)
	}
	sourceInfo, err := os.Lstat(pendingPath)
	if err != nil {
		return fmt.Errorf("inspect pending legacy archive %q: %w", pendingPath, err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("pending legacy archive %q is not a regular file", pendingPath)
	}
	if sourceInfo.Size() > maximumModPortalArchiveBytes {
		return fmt.Errorf("pending legacy archive %q exceeds %d bytes", pendingPath, maximumModPortalArchiveBytes)
	}

	stage, err := os.MkdirTemp(packRoot, ".modpack-migration-")
	if err != nil {
		return fmt.Errorf("stage legacy mod pack %q: %w", packName, err)
	}
	defer os.RemoveAll(stage)

	modList := ModSimpleList{Mods: []ModSimple{{Name: "base", Enabled: true}}}
	contents, err := json.Marshal(modList)
	if err != nil {
		return fmt.Errorf("encode base mod list for %q: %w", packName, err)
	}
	if err := os.WriteFile(filepath.Join(stage, "mod-list.json"), contents, 0664); err != nil {
		return fmt.Errorf("write staged mod list for %q: %w", packName, err)
	}
	mods, err := NewMods(stage)
	if err != nil {
		return fmt.Errorf("initialize staged mod pack %q: %w", packName, err)
	}

	archive, err := zip.OpenReader(pendingPath)
	if err != nil {
		return fmt.Errorf("open legacy mod pack archive %q: %w", pendingPath, err)
	}
	if len(archive.File) > maximumLegacyModPackEntries {
		archive.Close()
		return fmt.Errorf("legacy mod pack %q has too many entries (%d)", packName, len(archive.File))
	}
	var totalExpanded uint64
	seen := make(map[string]struct{}, len(archive.File))
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			archive.Close()
			return fmt.Errorf("legacy mod pack contains directory entry %q; only flat archive files are supported", file.Name)
		}
		if !file.Mode().IsRegular() || !filepath.IsLocal(file.Name) || filepath.Base(file.Name) != file.Name || !strings.EqualFold(filepath.Ext(file.Name), ".zip") {
			archive.Close()
			return fmt.Errorf("legacy mod pack contains invalid entry %q", file.Name)
		}
		if err := ValidatePathElement(file.Name); err != nil {
			archive.Close()
			return fmt.Errorf("legacy mod pack contains invalid entry %q: %w", file.Name, err)
		}
		key := strings.ToLower(file.Name)
		if _, ok := seen[key]; ok {
			archive.Close()
			return fmt.Errorf("legacy mod pack contains duplicate entry %q", file.Name)
		}
		seen[key] = struct{}{}
		if file.UncompressedSize64 > uint64(maximumModPortalArchiveBytes) || totalExpanded > uint64(maximumModPortalArchiveBytes)-file.UncompressedSize64 {
			archive.Close()
			return fmt.Errorf("legacy mod pack entries exceed %d total bytes", maximumModPortalArchiveBytes)
		}
		totalExpanded += file.UncompressedSize64
		reader, err := file.Open()
		if err != nil {
			archive.Close()
			return fmt.Errorf("open legacy mod %q: %w", file.Name, err)
		}
		var buffer bytes.Buffer
		readErr := copyWithHardLimit(&buffer, reader, maximumModPortalArchiveBytes)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			archive.Close()
			return errors.Join(readErr, closeErr)
		}
		inner, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
		if err != nil {
			archive.Close()
			return fmt.Errorf("open nested mod archive %q: %w", file.Name, err)
		}
		var modInfo ModInfo
		if err := modInfo.getModInfo(inner); err != nil {
			archive.Close()
			return fmt.Errorf("read metadata from nested mod %q: %w", file.Name, err)
		}
		if err := mods.createModWithLimit(modInfo.Name, file.Name, bytes.NewReader(buffer.Bytes()), maximumModPortalArchiveBytes); err != nil {
			archive.Close()
			return fmt.Errorf("install nested mod %q: %w", file.Name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("close legacy mod pack archive %q: %w", pendingPath, err)
	}
	if err := commitLegacyModPack(stage, target); err != nil {
		return fmt.Errorf("activate migrated mod pack %q: %w", packName, err)
	}
	return nil
}

func commitLegacyModPack(stage, target string) error {
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(stage, target)
}
