package factorio

import (
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type ModPortalPreferences struct {
	PreselectOptional bool `json:"preselect_optional"`
}

var modPortalPreferencesMutex sync.Mutex
var modPortalPreferencesPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), "mod-portal-preferences.json")
}

func LoadModPortalPreferences() (ModPortalPreferences, error) {
	modPortalPreferencesMutex.Lock()
	defer modPortalPreferencesMutex.Unlock()
	var preferences ModPortalPreferences
	contents, err := readBoundedRegularFile(modPortalPreferencesPath(), 4096, true)
	if err == nil && contents == nil {
		return preferences, nil
	}
	if err != nil {
		return preferences, err
	}
	err = json.Unmarshal(contents, &preferences)
	return preferences, err
}

func SetModPortalPreferences(preferences ModPortalPreferences) (ModPortalPreferences, error) {
	modPortalPreferencesMutex.Lock()
	defer modPortalPreferencesMutex.Unlock()
	contents, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return ModPortalPreferences{}, err
	}
	if err := writeFileAtomically(modPortalPreferencesPath(), append(contents, '\n'), 0600); err != nil {
		return ModPortalPreferences{}, err
	}
	return preferences, nil
}
