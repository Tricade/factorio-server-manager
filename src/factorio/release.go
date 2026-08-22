package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

// ReleaseChannel identifies one of the official rolling Factorio headless releases.
type ReleaseChannel string

const (
	ReleaseChannelStable ReleaseChannel = "stable"
	ReleaseChannelLatest ReleaseChannel = "latest"
)

var releaseDownloadURLs = map[ReleaseChannel]string{
	ReleaseChannelStable: "https://www.factorio.com/get-download/stable/headless/linux64",
	ReleaseChannelLatest: "https://www.factorio.com/get-download/latest/headless/linux64",
}

var releaseInstallLock sync.Mutex
var factorioProgramFilesGate sync.RWMutex
var releaseRename = os.Rename
var releaseChannelMetadataURL = "https://factorio.com/api/latest-releases"
var releaseVersionMetadataURL = "https://updater.factorio.com/get-available-versions"
var releaseMetadataHTTPClient = &http.Client{Timeout: 15 * time.Second}

var exactReleasePattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:\.0)?$`)

var releaseDataDirectories = map[string]bool{
	"saves":  true,
	"mods":   true,
	"config": true,
}

type ReleaseStatus struct {
	InstalledVersion  string   `json:"installed_version"`
	InstalledChannel  string   `json:"installed_channel"`
	InstalledTarget   string   `json:"installed_target,omitempty"`
	StableVersion     string   `json:"stable_version,omitempty"`
	LatestVersion     string   `json:"latest_version,omitempty"`
	AvailableVersions []string `json:"available_versions"`
	MetadataError     string   `json:"metadata_error,omitempty"`
}

type officialReleaseResponse struct {
	Updates []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"core-linux_headless64"`
}

type officialReleaseChannels struct {
	Experimental json.RawMessage `json:"experimental"`
	Stable       json.RawMessage `json:"stable"`
}

// NormalizeExactReleaseVersion validates and canonicalizes an exact public
// Factorio version. A trailing build component of .0 is accepted because the
// manager exposes Factorio's internal four-component version elsewhere.
func NormalizeExactReleaseVersion(version string) (string, error) {
	version = strings.TrimSpace(version)
	if !exactReleasePattern.MatchString(version) {
		return "", fmt.Errorf("unsupported Factorio release target %q; use stable, latest or an exact version such as 2.1.14", version)
	}
	version = strings.TrimSuffix(version, ".0")
	var parsed Version
	if err := parsed.UnmarshalText([]byte(version)); err != nil {
		return "", fmt.Errorf("invalid Factorio release version %q: %w", version, err)
	}
	return parsed.ReleaseString(), nil
}

// NormalizeReleaseTarget accepts the two official rolling channels and exact
// version identifiers. It never accepts a URL supplied by the browser.
func NormalizeReleaseTarget(target string) (string, error) {
	target = strings.TrimSpace(strings.ToLower(target))
	if _, ok := releaseDownloadURLs[ReleaseChannel(target)]; ok {
		return target, nil
	}
	return NormalizeExactReleaseVersion(target)
}

// ReleaseDownloadURL returns an official Factorio download URL for a supported
// rolling channel or an exact, strictly validated version.
func ReleaseDownloadURL(target string) (string, error) {
	normalized, err := NormalizeReleaseTarget(target)
	if err != nil {
		return "", err
	}
	if downloadURL, ok := releaseDownloadURLs[ReleaseChannel(normalized)]; ok {
		return downloadURL, nil
	}
	downloadURL, err := url.JoinPath("https://www.factorio.com/get-download", normalized, "headless/linux64")
	if err != nil {
		return "", fmt.Errorf("build Factorio release URL: %w", err)
	}
	return downloadURL, nil
}

func parseOfficialRelease(raw json.RawMessage) (string, error) {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil && direct != "" {
		return NormalizeExactReleaseVersion(direct)
	}
	var branch struct {
		Headless string `json:"headless"`
	}
	if err := json.Unmarshal(raw, &branch); err != nil {
		return "", err
	}
	return NormalizeExactReleaseVersion(branch.Headless)
}

func sortedAvailableVersions(official officialReleaseResponse, stable, latest string) []string {
	unique := map[string]bool{}
	for _, candidate := range []string{stable, latest} {
		if candidate != "" {
			unique[candidate] = true
		}
	}
	for _, update := range official.Updates {
		for _, candidate := range []string{update.From, update.To} {
			if normalized, err := NormalizeExactReleaseVersion(candidate); err == nil {
				unique[normalized] = true
			}
		}
	}
	versions := make([]string, 0, len(unique))
	for candidate := range unique {
		versions = append(versions, candidate)
	}
	sort.Slice(versions, func(i, j int) bool {
		var left, right Version
		_ = left.UnmarshalText([]byte(versions[i]))
		_ = right.UnmarshalText([]byte(versions[j]))
		return left.Greater(right)
	})
	return versions
}

func versionsEqual(left, right string) bool {
	var parsedLeft, parsedRight Version
	if parsedLeft.UnmarshalText([]byte(left)) != nil || parsedRight.UnmarshalText([]byte(right)) != nil {
		return false
	}
	return parsedLeft.Equals(parsedRight)
}

func decodeOfficialMetadata(metadataURL string, target interface{}) error {
	response, err := releaseMetadataHTTPClient.Get(metadataURL)
	if err != nil {
		return fmt.Errorf("load %s: %w", metadataURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("load %s: unexpected status %s", metadataURL, response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", metadataURL, err)
	}
	return nil
}

// SupportedReleaseChannels returns channels that may be installed through the manager.
func SupportedReleaseChannels() []ReleaseChannel {
	return []ReleaseChannel{ReleaseChannelStable, ReleaseChannelLatest}
}

func GetReleaseStatus() (ReleaseStatus, error) {
	snapshot := GetFactorioServer().Snapshot()
	installedVersion := snapshot.BaseModVersion
	if installedVersion == "" {
		installedVersion = snapshot.Version.ReleaseString()
	} else if normalized, err := NormalizeExactReleaseVersion(installedVersion); err == nil {
		installedVersion = normalized
	}
	status := ReleaseStatus{
		InstalledVersion:  installedVersion,
		InstalledChannel:  "custom",
		AvailableVersions: []string{},
	}
	if state, err := LoadRuntimeState(); err == nil {
		status.InstalledTarget = state.ReleaseTarget
		if state.InstalledVersion != "" && versionsEqual(state.InstalledVersion, installedVersion) &&
			(state.ReleaseTarget == string(ReleaseChannelStable) || state.ReleaseTarget == string(ReleaseChannelLatest)) {
			status.InstalledChannel = state.ReleaseTarget
		}
	} else {
		status.MetadataError = err.Error()
	}

	var channels officialReleaseChannels
	if err := decodeOfficialMetadata(releaseChannelMetadataURL, &channels); err != nil {
		status.MetadataError = fmt.Sprintf("load official Factorio release channels: %v", err)
		return status, nil
	}
	var err error
	status.StableVersion, err = parseOfficialRelease(channels.Stable)
	if err != nil {
		status.MetadataError = fmt.Sprintf("decode official stable release: %v", err)
		return status, nil
	}
	status.LatestVersion, err = parseOfficialRelease(channels.Experimental)
	if err != nil {
		status.MetadataError = fmt.Sprintf("decode official experimental release: %v", err)
		return status, nil
	}

	var official officialReleaseResponse
	if err := decodeOfficialMetadata(releaseVersionMetadataURL, &official); err != nil {
		status.MetadataError = fmt.Sprintf("load official Factorio version archive: %v", err)
		status.AvailableVersions = sortedAvailableVersions(official, status.StableVersion, status.LatestVersion)
		return status, nil
	}
	status.AvailableVersions = sortedAvailableVersions(official, status.StableVersion, status.LatestVersion)

	// From this point the official channel versions are authoritative. The
	// persisted rolling target remains available separately, but it must not make
	// an older binary look like the current Stable or Experimental release.
	status.InstalledChannel = "custom"
	var installed, stable, latest Version
	if err := installed.UnmarshalText([]byte(status.InstalledVersion)); err != nil {
		status.MetadataError = fmt.Sprintf("parse installed Factorio version: %v", err)
		return status, nil
	}
	if err := stable.UnmarshalText([]byte(status.StableVersion)); err != nil {
		status.MetadataError = fmt.Sprintf("parse stable Factorio version: %v", err)
		return status, nil
	}
	if err := latest.UnmarshalText([]byte(status.LatestVersion)); err != nil {
		status.MetadataError = fmt.Sprintf("parse latest Factorio version: %v", err)
		return status, nil
	}
	switch {
	case installed.Equals(stable):
		status.InstalledChannel = string(ReleaseChannelStable)
	case installed.Equals(latest):
		status.InstalledChannel = string(ReleaseChannelLatest)
	}
	return status, nil
}

// InstallRelease downloads a supported official Linux headless release and updates the
// Factorio program files. Existing saves, mods, and configuration remain untouched.
func InstallRelease(target string) error {
	status, err := GetGameModeStatus()
	if err != nil {
		return fmt.Errorf("read current game mode before release change: %w", err)
	}
	requiredBuiltIns := make([]string, 0, len(status.Features))
	for _, feature := range status.Features {
		if feature.Enabled {
			requiredBuiltIns = append(requiredBuiltIns, feature.Name)
		}
	}
	return installRelease(target, target, requiredBuiltIns)
}

// InstallProfileRelease restores the exact binary recorded by a profile while
// preserving its rolling Stable/Latest target as metadata. Required built-in
// expansion mods are validated against the target profile rather than whatever
// profile happens to be active during the download.
func InstallProfileRelease(installedVersion, releaseTarget string, requiredBuiltIns []string) error {
	version, err := NormalizeExactReleaseVersion(installedVersion)
	if err != nil {
		return fmt.Errorf("invalid profile Factorio version: %w", err)
	}
	if releaseTarget == "" {
		releaseTarget = version
	}
	target, err := NormalizeReleaseTarget(releaseTarget)
	if err != nil {
		return fmt.Errorf("invalid profile release target: %w", err)
	}
	return installRelease(version, target, requiredBuiltIns)
}

func installRelease(downloadTarget, persistedTarget string, requiredBuiltIns []string) error {
	if !releaseInstallLock.TryLock() {
		return errors.New("another Factorio release installation is already running")
	}
	defer releaseInstallLock.Unlock()

	normalizedTarget, err := NormalizeReleaseTarget(downloadTarget)
	if err != nil {
		return err
	}
	normalizedPersistedTarget, err := NormalizeReleaseTarget(persistedTarget)
	if err != nil {
		return err
	}
	downloadURL, err := ReleaseDownloadURL(normalizedTarget)
	if err != nil {
		return err
	}

	archive, err := os.CreateTemp("", "factorio-release-*.tar.xz")
	if err != nil {
		return fmt.Errorf("create temporary download: %w", err)
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)

	client := &http.Client{Timeout: 5 * time.Minute}
	response, err := client.Get(downloadURL)
	if err != nil {
		archive.Close()
		return fmt.Errorf("download Factorio %s release: %w", normalizedTarget, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		archive.Close()
		return fmt.Errorf("download Factorio %s release: unexpected status %s", normalizedTarget, response.Status)
	}
	if _, err := io.Copy(archive, response.Body); err != nil {
		archive.Close()
		return fmt.Errorf("save Factorio %s release: %w", normalizedTarget, err)
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("close Factorio release archive: %w", err)
	}

	extractDir, err := os.MkdirTemp("", "factorio-release-extract-*")
	if err != nil {
		return fmt.Errorf("create temporary extraction directory: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if output, err := exec.Command("tar", "-xJf", archivePath, "-C", extractDir).CombinedOutput(); err != nil {
		return fmt.Errorf("extract Factorio %s release: %w: %s", normalizedTarget, err, output)
	}

	sourceDir := filepath.Join(extractDir, "factorio")
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return fmt.Errorf("Factorio %s release archive does not contain a factorio directory", normalizedTarget)
	}
	installedVersion, err := releaseVersionFromDirectory(sourceDir)
	if err != nil {
		return err
	}
	if err := validateRequiredBuiltInMods(sourceDir, requiredBuiltIns); err != nil {
		return err
	}

	// A map snapshot may be loading an isolated copy of a save with the
	// currently installed engine. Wait for that read-only operation before
	// atomically replacing the shared Factorio program files.
	factorioProgramFilesGate.Lock()
	defer factorioProgramFilesGate.Unlock()

	if err := replaceReleaseFiles(sourceDir, bootstrap.GetConfig().FactorioDir); err != nil {
		return err
	}
	if err := RefreshFactorioVersion(); err != nil {
		return fmt.Errorf("reload installed Factorio version: %w", err)
	}
	return persistRuntimeState(normalizedPersistedTarget, installedVersion)
}

func releaseVersionFromDirectory(directory string) (string, error) {
	contents, err := os.ReadFile(filepath.Join(directory, "data", "base", "info.json"))
	if err != nil {
		return "", fmt.Errorf("read downloaded Factorio base version: %w", err)
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(contents, &info); err != nil {
		return "", fmt.Errorf("decode downloaded Factorio base version: %w", err)
	}
	normalized, err := NormalizeExactReleaseVersion(info.Version)
	if err != nil {
		return "", fmt.Errorf("validate downloaded Factorio base version: %w", err)
	}
	return normalized, nil
}

func validateRequiredBuiltInMods(sourceDir string, requiredBuiltIns []string) error {
	allowed := make(map[string]bool, len(spaceAgeFeatureMods))
	for _, name := range spaceAgeFeatureMods {
		allowed[name] = true
	}
	seen := make(map[string]bool, len(requiredBuiltIns))
	for _, name := range requiredBuiltIns {
		if !allowed[name] || seen[name] {
			continue
		}
		seen[name] = true
		if _, err := os.Stat(filepath.Join(sourceDir, "data", name, "info.json")); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("release does not contain enabled built-in mod %s; switch this profile to base Factorio mode before installing this version", name)
		} else if err != nil {
			return fmt.Errorf("inspect downloaded built-in mod %s: %w", name, err)
		}
	}
	return nil
}

// replaceReleaseFiles stages the new program tree inside destinationDir and swaps its
// top-level program entries into place. destinationDir itself must never be renamed:
// in container deployments it is commonly a bind mount or Docker volume mount, and
// Linux rejects renaming a mount point with EBUSY.
func replaceReleaseFiles(sourceDir, destinationDir string) error {
	if err := os.MkdirAll(destinationDir, 0755); err != nil {
		return fmt.Errorf("create Factorio destination directory: %w", err)
	}

	stagingDir, err := os.MkdirTemp(destinationDir, ".factorio-release-staging-")
	if err != nil {
		return fmt.Errorf("create staged release directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)
	backupDir, err := os.MkdirTemp(destinationDir, ".factorio-release-backup-")
	if err != nil {
		return fmt.Errorf("create release backup directory: %w", err)
	}
	defer os.RemoveAll(backupDir)

	if err := copyReleaseFiles(sourceDir, stagingDir, nil, releaseDataDirectories); err != nil {
		return fmt.Errorf("stage new Factorio release: %w", err)
	}

	ignoredEntries := map[string]bool{
		filepath.Base(stagingDir): true,
		filepath.Base(backupDir):  true,
	}
	for name := range releaseDataDirectories {
		ignoredEntries[name] = true
	}

	backedUpEntries, err := moveReleaseEntries(destinationDir, backupDir, ignoredEntries)
	if err != nil {
		if rollbackErr := restoreReleaseEntries(backupDir, destinationDir, backedUpEntries); rollbackErr != nil {
			return fmt.Errorf("back up current Factorio release: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("back up current Factorio release: %w (previous release restored)", err)
	}

	activatedEntries, err := moveReleaseEntries(stagingDir, destinationDir, nil)
	if err != nil {
		rollbackErr := rollbackReleaseActivation(destinationDir, stagingDir, backupDir, activatedEntries, backedUpEntries)
		if rollbackErr != nil {
			return fmt.Errorf("activate new Factorio release: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("activate new Factorio release: %w (previous release restored)", err)
	}

	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("remove previous Factorio release backup: %w", err)
	}
	return nil
}

// moveReleaseEntries renames the top-level entries from sourceDir into
// destinationDir. Both directories live below the Factorio mount, so the operation
// remains on one filesystem even when destinationDir is itself a mount point.
func moveReleaseEntries(sourceDir, destinationDir string, ignoredEntries map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, err
	}

	movedEntries := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if ignoredEntries != nil && ignoredEntries[name] {
			continue
		}
		if err := releaseRename(filepath.Join(sourceDir, name), filepath.Join(destinationDir, name)); err != nil {
			return movedEntries, err
		}
		movedEntries = append(movedEntries, name)
	}
	return movedEntries, nil
}

func restoreReleaseEntries(sourceDir, destinationDir string, entries []string) error {
	var restoreErrors []error
	for index := len(entries) - 1; index >= 0; index-- {
		name := entries[index]
		if err := releaseRename(filepath.Join(sourceDir, name), filepath.Join(destinationDir, name)); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", name, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func rollbackReleaseActivation(destinationDir, stagingDir, backupDir string, activatedEntries, backedUpEntries []string) error {
	var rollbackErrors []error
	if err := restoreReleaseEntries(destinationDir, stagingDir, activatedEntries); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("remove partially activated release: %w", err))
	}
	if err := restoreReleaseEntries(backupDir, destinationDir, backedUpEntries); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous release: %w", err))
	}
	return errors.Join(rollbackErrors...)
}

// copyReleaseFiles copies all files when onlyDirectories is nil. Otherwise it copies
// only the named top-level directories. skipDirectories is excluded in either case.
func copyReleaseFiles(sourceDir, destinationDir string, onlyDirectories, skipDirectories map[string]bool) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relativePath == "." {
			return os.MkdirAll(destinationDir, info.Mode())
		}
		firstPathElement := strings.Split(relativePath, string(filepath.Separator))[0]
		if skipDirectories != nil && skipDirectories[firstPathElement] {
			if info.IsDir() && firstPathElement == relativePath {
				return filepath.SkipDir
			}
			return nil
		}
		if onlyDirectories != nil && !onlyDirectories[firstPathElement] {
			if info.IsDir() && firstPathElement == relativePath {
				return filepath.SkipDir
			}
			return nil
		}

		destinationPath := filepath.Join(destinationDir, relativePath)
		if info.IsDir() {
			return os.MkdirAll(destinationPath, info.Mode())
		}
		return copyReleaseFile(path, destinationPath, info.Mode())
	})
}

func copyReleaseFile(sourcePath, destinationPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		return err
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
