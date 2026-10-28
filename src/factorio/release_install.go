package factorio

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

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
	downloadURL, err := officialReleaseDownloadURL(normalizedTarget)
	if err != nil {
		return err
	}
	// Share the same engine-operation reservation as startup and world creation.
	// Checking under lifecycle ownership closes the earlier middleware check/use
	// gap, while releasing that mutex keeps start requests responsive during a
	// download: they receive a conflict instead of waiting for network I/O.
	serverLifecycleMutex.Lock()
	if GetFactorioServer().IsBusy() {
		serverLifecycleMutex.Unlock()
		return ErrServerActive
	}
	if !worldGenerationLock.TryLock() {
		serverLifecycleMutex.Unlock()
		return ErrWorldGenerationBusy
	}
	serverLifecycleMutex.Unlock()
	defer worldGenerationLock.Unlock()

	archive, err := os.CreateTemp("", "factorio-release-*.tar.xz")
	if err != nil {
		return fmt.Errorf("create temporary download: %w", err)
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)

	request := &http.Request{Method: http.MethodGet, URL: downloadURL, Header: make(http.Header)}
	response, err := newFactorioReleaseHTTPClient().Do(request)
	if err != nil {
		archive.Close()
		return fmt.Errorf("download Factorio %s release: %w", normalizedTarget, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		archive.Close()
		return fmt.Errorf("download Factorio %s release: unexpected status %s", normalizedTarget, response.Status)
	}
	if response.ContentLength > maximumFactorioReleaseArchiveBytes {
		archive.Close()
		return fmt.Errorf("download Factorio %s release: archive exceeds %d bytes", normalizedTarget, maximumFactorioReleaseArchiveBytes)
	}
	if err := copyWithHardLimit(archive, response.Body, maximumFactorioReleaseArchiveBytes); err != nil {
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
		return fmt.Errorf("the Factorio %s release archive does not contain a factorio directory", normalizedTarget)
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

	runtimeStateSnapshot, err := snapshotRuntimeStateFiles()
	if err != nil {
		return err
	}
	persistAttempted := false
	persistSucceeded := false
	installErr := replaceReleaseFilesValidated(sourceDir, bootstrap.GetConfig().FactorioDir, func() error {
		if err := RefreshFactorioVersion(); err != nil {
			return fmt.Errorf("reload installed Factorio version: %w", err)
		}
		persistAttempted = true
		if err := persistRuntimeState(normalizedPersistedTarget, installedVersion); err != nil {
			return err
		}
		persistSucceeded = true
		return nil
	})
	if installErr == nil || persistSucceeded {
		return installErr
	}

	var recoveryErrors []error
	if persistAttempted {
		if err := restoreRuntimeStateFiles(runtimeStateSnapshot); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("restore previous runtime metadata: %w", err))
		}
	}
	if err := RefreshFactorioVersion(); err != nil {
		recoveryErrors = append(recoveryErrors, fmt.Errorf("reload restored Factorio version: %w", err))
	}
	if recoveryErr := errors.Join(recoveryErrors...); recoveryErr != nil {
		return fmt.Errorf("%w (recovery failed: %v)", installErr, recoveryErr)
	}
	return installErr
}
