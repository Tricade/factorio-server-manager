package factorio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

func captureActiveProfile(profile Profile) (Profile, error) {
	runtimeState, err := profileLoadRuntimeState()
	if err != nil {
		return Profile{}, err
	}
	snapshot := GetFactorioServer().Snapshot()
	installedVersion := runtimeState.InstalledVersion
	if installedVersion == "" {
		installedVersion = snapshot.BaseModVersion
	}
	if installedVersion == "" {
		installedVersion = snapshot.Version.ReleaseString()
	}
	installedVersion, err = NormalizeExactReleaseVersion(installedVersion)
	if err != nil {
		return Profile{}, err
	}
	releaseTarget := runtimeState.ReleaseTarget
	if releaseTarget == "" {
		releaseTarget = installedVersion
	}
	releaseTarget, err = NormalizeReleaseTarget(releaseTarget)
	if err != nil {
		return Profile{}, err
	}

	mode, err := profileGetGameModeStatus()
	if err != nil {
		return Profile{}, err
	}
	enabledBuiltIns := make([]string, 0, len(mode.Features))
	for _, feature := range mode.Features {
		if feature.Enabled {
			enabledBuiltIns = append(enabledBuiltIns, feature.Name)
		}
	}
	sort.Strings(enabledBuiltIns)

	directories := profileActiveDirectories()
	saveCount, err := countUsableSaves(directories["saves"])
	if err != nil {
		return Profile{}, err
	}
	modCount, err := countInstalledModArchives(directories["mods"])
	if err != nil {
		return Profile{}, err
	}
	selectedSave := snapshot.Savefile
	if saveCount == 0 {
		selectedSave = ""
	} else if selectedSave == "" {
		if latest, latestErr := GetLatestSave(); latestErr == nil {
			selectedSave = latest.Name
		}
	}
	bindIP := snapshot.BindIP
	if bindIP == "" {
		bindIP = bootstrap.GetConfig().FactorioIP
	}
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	port := snapshot.Port
	if port == 0 {
		port = 34197
	}

	profile.Active = true
	profile.ReleaseTarget = releaseTarget
	profile.InstalledVersion = installedVersion
	profile.GameMode = mode.Mode
	profile.EnabledBuiltInMods = enabledBuiltIns
	profile.SelectedSave = selectedSave
	profile.BindIP = bindIP
	profile.Port = port
	profile.SaveCount = saveCount
	profile.ModCount = modCount
	profile.UpdatedAt = profileNow().UTC()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = profile.UpdatedAt
	}
	return profile, nil
}

func ensureActiveProfileDirectories() error {
	for name, directory := range profileActiveDirectories() {
		if directory == "" {
			return fmt.Errorf("active %s directory is not configured", name)
		}
		if err := os.MkdirAll(directory, 0755); err != nil {
			return fmt.Errorf("create active %s directory: %w", name, err)
		}
	}
	return nil
}

func countUsableSaves(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		if isUsableSave(info) {
			count++
		}
	}
	return count, nil
}

func countInstalledModArchives(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
			continue
		}
		count++
	}
	return count, nil
}
