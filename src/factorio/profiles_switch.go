package factorio

import (
	"errors"
	"fmt"
	"log"
)

func ActivateProfile(id string) (ProfileState, error) {
	profileDataGate.Lock()
	defer profileDataGate.Unlock()
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if err := ensureProfileServerStopped(); err != nil {
		return ProfileState{}, err
	}
	if err := validateProfileID(id); err != nil {
		return ProfileState{}, ErrProfileNotFound
	}
	manifest, err := loadProfileManifest()
	if err != nil {
		return ProfileState{}, err
	}
	targetIndex := profileIndex(manifest, id)
	if targetIndex < 0 {
		return ProfileState{}, ErrProfileNotFound
	}
	if manifest.ActiveProfileID == id {
		return stateFromManifest(manifest), nil
	}
	currentIndex := profileIndex(manifest, manifest.ActiveProfileID)
	if currentIndex < 0 {
		return ProfileState{}, errors.New("active profile metadata is missing")
	}

	current, err := captureActiveProfile(manifest.Profiles[currentIndex])
	if err != nil {
		return ProfileState{}, fmt.Errorf("capture active profile metadata: %w", err)
	}
	if err := snapshotActiveProfile(current.ID); err != nil {
		return ProfileState{}, fmt.Errorf("snapshot active profile before switch: %w", err)
	}
	manifest.Profiles[currentIndex] = current
	target := manifest.Profiles[targetIndex]
	if err := validateStoredProfileData(target); err != nil {
		return ProfileState{}, fmt.Errorf("validate target profile: %w", err)
	}

	swaps, err := prepareProfileSwaps(target.ID)
	if err != nil {
		return ProfileState{}, err
	}
	defer cleanupProfileSwaps(swaps)

	releaseChanged := !versionsEqual(current.InstalledVersion, target.InstalledVersion)
	if releaseChanged {
		if err := profileInstallRelease(target.InstalledVersion, target.ReleaseTarget, target.EnabledBuiltInMods); err != nil {
			return ProfileState{}, fmt.Errorf("restore Factorio %s for profile %q: %w", target.InstalledVersion, target.Name, err)
		}
	}

	activated := make([]*directoryEntrySwap, 0, len(swaps))
	for _, swap := range swaps {
		if err := swap.activate(); err != nil {
			rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
			if rollbackErr != nil {
				return ProfileState{}, fmt.Errorf("activate profile data: %w (rollback failed: %v)", err, rollbackErr)
			}
			return ProfileState{}, fmt.Errorf("activate profile data: %w (previous profile restored)", err)
		}
		activated = append(activated, swap)
	}

	if err := setBuiltInFeatureMods(profileActiveDirectories()["mods"], target.EnabledBuiltInMods); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("restore profile game mode: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("restore profile game mode: %w (previous profile restored)", err)
	}

	if err := profileReloadServer(target.BindIP, target.Port, target.SelectedSave); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("reload target profile: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("reload target profile: %w (previous profile restored)", err)
	}
	if err := profilePersistRuntimeState(target.ReleaseTarget, target.InstalledVersion); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("persist target runtime: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("persist target runtime: %w (previous profile restored)", err)
	}

	now := profileNow().UTC()
	manifest.ActiveProfileID = target.ID
	manifest.Profiles[currentIndex] = current
	manifest.Profiles[targetIndex].UpdatedAt = now
	if err := saveProfileManifest(manifest); err != nil {
		rollbackErr := rollbackProfileSwitch(activated, current, releaseChanged)
		if rollbackErr != nil {
			return ProfileState{}, fmt.Errorf("commit profile selection: %w (rollback failed: %v)", err, rollbackErr)
		}
		return ProfileState{}, fmt.Errorf("commit profile selection: %w (previous profile restored)", err)
	}

	for _, swap := range swaps {
		if err := swap.commit(); err != nil {
			log.Printf("Profile %s is active but temporary switch data could not be removed: %v", target.ID, err)
		}
	}
	log.Printf("Activated Factorio profile %q (%s); Factorio remains stopped", target.Name, target.ID)
	return stateFromManifest(manifest), nil
}

func rollbackProfileSwitch(activated []*directoryEntrySwap, current Profile, releaseChanged bool) error {
	var rollbackErrors []error
	for index := len(activated) - 1; index >= 0; index-- {
		if err := activated[index].rollback(); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if releaseChanged {
		if err := profileInstallRelease(current.InstalledVersion, current.ReleaseTarget, current.EnabledBuiltInMods); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous Factorio release: %w", err))
		}
	} else if err := profilePersistRuntimeState(current.ReleaseTarget, current.InstalledVersion); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous runtime metadata: %w", err))
	}
	if err := profileReloadServer(current.BindIP, current.Port, current.SelectedSave); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("reload previous profile: %w", err))
	}
	return errors.Join(rollbackErrors...)
}
