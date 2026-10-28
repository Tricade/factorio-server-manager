package factorio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func activeModStartupSettingsState() (Profile, string, modSettingsDocument, []byte, string, error) {
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return Profile{}, "", modSettingsDocument{}, nil, "", err
	}
	profile, err = captureActiveProfile(profile)
	if err != nil {
		return Profile{}, "", modSettingsDocument{}, nil, "", fmt.Errorf("inspect active profile: %w", err)
	}
	version, parsedVersion, err := normalizeModStartupFactorioVersion(profile.InstalledVersion)
	if err != nil {
		return Profile{}, "", modSettingsDocument{}, nil, "", err
	}
	document, contents, err := loadModStartupSettingsDocument(filepath.Join(profileActiveDirectories()["mods"], "mod-settings.dat"), parsedVersion)
	if err != nil {
		return Profile{}, "", modSettingsDocument{}, nil, "", err
	}
	revision, err := modStartupSettingsRevision(profile, version, contents)
	if err != nil {
		return Profile{}, "", modSettingsDocument{}, nil, "", err
	}
	return profile, version, document, contents, revision, nil
}

func normalizeModStartupFactorioVersion(value string) (string, Version, error) {
	normalized, err := NormalizeExactReleaseVersion(value)
	if err != nil {
		return "", Version{}, fmt.Errorf("%w: invalid installed Factorio version", ErrModStartupSettingsUnsupported)
	}
	var version Version
	if err := version.UnmarshalText([]byte(normalized)); err != nil {
		return "", Version{}, fmt.Errorf("%w: invalid installed Factorio version", ErrModStartupSettingsUnsupported)
	}
	if version.Less(Version{0, 18, 47, 0}) {
		return "", Version{}, fmt.Errorf("%w: Factorio 0.18.47 or newer is required", ErrModStartupSettingsUnsupported)
	}
	return normalized, version, nil
}

func loadModStartupSettingsDocument(path string, version Version) (modSettingsDocument, []byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return newModSettingsDocument(version), nil, nil
	}
	if err != nil {
		return modSettingsDocument{}, nil, fmt.Errorf("inspect mod-settings.dat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return modSettingsDocument{}, nil, fmt.Errorf("%w: mod-settings.dat is not a regular file", ErrInvalidModSettingsFile)
	}
	if info.Size() <= 0 || info.Size() > maximumModSettingsFileBytes {
		return modSettingsDocument{}, nil, fmt.Errorf("%w: mod-settings.dat has an invalid size", ErrInvalidModSettingsFile)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return modSettingsDocument{}, nil, fmt.Errorf("read mod-settings.dat: %w", err)
	}
	document, err := decodeModSettingsDocument(contents)
	if err != nil {
		return modSettingsDocument{}, nil, err
	}
	document.Version = version
	return document, contents, nil
}

func modStartupSettingsRevision(profile Profile, version string, settingsContents []byte) (string, error) {
	modsDirectory := profileActiveDirectories()["mods"]
	modListPath := filepath.Join(modsDirectory, "mod-list.json")
	modListContents, err := readBoundedRegularFile(modListPath, maximumModStartupModListBytes, true)
	if err != nil {
		return "", fmt.Errorf("read active mod list: %w", err)
	}
	enabled, err := decodeEnabledModNames(modListContents)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	writeModStartupHashPart(digest, []byte(profile.ID))
	writeModStartupHashPart(digest, []byte(version))
	writeModStartupHashPart(digest, modListContents)
	writeModStartupHashPart(digest, settingsContents)

	entries, err := os.ReadDir(modsDirectory)
	if err != nil {
		return "", fmt.Errorf("list active mods: %w", err)
	}
	entryNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if mapSnapshotModEntryRequired(entry.Name(), enabled) && entry.Name() != "mod-list.json" && entry.Name() != "mod-settings.dat" {
			entryNames = append(entryNames, entry.Name())
		}
	}
	sort.Strings(entryNames)
	visited := 0
	for _, name := range entryNames {
		if err := hashModStartupEntryMetadata(digest, modsDirectory, filepath.Join(modsDirectory, name), &visited); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func decodeEnabledModNames(contents []byte) (map[string]struct{}, error) {
	if len(contents) == 0 {
		return map[string]struct{}{}, nil
	}
	var document struct {
		Mods []ModSimple `json:"mods"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, fmt.Errorf("decode active mod list: %w", err)
	}
	if len(document.Mods) > maximumSaveModImportItems*4 {
		return nil, errors.New("active mod list contains too many entries")
	}
	enabled := make(map[string]struct{}, len(document.Mods))
	for _, mod := range document.Mods {
		if len(mod.Name) > 255 || strings.IndexFunc(mod.Name, unicode.IsControl) >= 0 {
			return nil, errors.New("active mod list contains an invalid name")
		}
		if mod.Enabled && mod.Name != "" {
			enabled[mod.Name] = struct{}{}
		}
	}
	return enabled, nil
}

func hashModStartupEntryMetadata(digest hash.Hash, root, path string, visited *int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse symbolic link in enabled mod set: %s", info.Name())
	}
	(*visited)++
	if *visited > maximumModStartupFingerprintSet {
		return fmt.Errorf("enabled mod set exceeds %d filesystem entries", maximumModStartupFingerprintSet)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	writeModStartupHashPart(digest, []byte(filepath.ToSlash(relative)))
	writeModStartupHashPart(digest, []byte(strconv.FormatInt(info.Size(), 10)))
	writeModStartupHashPart(digest, []byte(strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse non-regular entry in enabled mod set: %s", info.Name())
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		copyErr := copyWithHardLimit(digest, file, maximumModPortalArchiveBytes)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("hash enabled mod entry %s: %w", info.Name(), copyErr)
		}
		return closeErr
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := hashModStartupEntryMetadata(digest, root, filepath.Join(path, entry.Name()), visited); err != nil {
			return err
		}
	}
	return nil
}

func writeModStartupHashPart(digest hash.Hash, contents []byte) {
	var length [8]byte
	for index := range length {
		length[index] = byte(uint64(len(contents)) >> (index * 8))
	}
	_, _ = digest.Write(length[:])
	_, _ = digest.Write(contents)
}

func readBoundedRegularFile(path string, maximum int64, missingAllowed bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && missingAllowed {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular file")
	}
	if info.Size() < 0 || info.Size() > maximum {
		return nil, fmt.Errorf("file exceeds %d bytes", maximum)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var output bytes.Buffer
	if err := copyWithHardLimit(&output, file, maximum); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
