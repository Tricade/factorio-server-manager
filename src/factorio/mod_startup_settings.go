package factorio

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
)

const (
	maximumModStartupSettings       = 5000
	maximumModStartupChanges        = 1000
	maximumModStartupValueBytes     = 64 * 1024
	maximumModStartupManifestBytes  = 8 * 1024 * 1024
	maximumModStartupLocaleBytes    = 64 * 1024
	maximumModStartupModListBytes   = 4 * 1024 * 1024
	maximumModStartupFingerprintSet = 20000
)

var (
	ErrModStartupSettingsBusy        = errors.New("mod startup settings are busy")
	ErrModStartupSettingsStale       = errors.New("mod startup settings form is stale")
	ErrInvalidModStartupSettings     = errors.New("invalid mod startup settings")
	ErrModStartupSettingsUnsupported = errors.New("mod startup settings are unsupported for this Factorio version")
)

type ModStartupSettingsView struct {
	ProfileID       string                    `json:"profile_id"`
	ProfileName     string                    `json:"profile_name"`
	FactorioVersion string                    `json:"factorio_version"`
	Revision        string                    `json:"revision"`
	Groups          []ModStartupSettingsGroup `json:"groups"`
}

type ModStartupSettingsGroup struct {
	Mod      string              `json:"mod"`
	Title    string              `json:"title"`
	Settings []ModStartupSetting `json:"settings"`
}

type ModStartupSetting struct {
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description,omitempty"`
	Type          string   `json:"type"`
	Value         any      `json:"value"`
	DefaultValue  any      `json:"default_value"`
	MinimumValue  *float64 `json:"minimum_value,omitempty"`
	MaximumValue  *float64 `json:"maximum_value,omitempty"`
	AllowedValues []any    `json:"allowed_values,omitempty"`
	AllowBlank    bool     `json:"allow_blank,omitempty"`
	AutoTrim      bool     `json:"auto_trim,omitempty"`
	Order         string   `json:"-"`
}

type ModStartupSettingChange struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type ModStartupSettingsUpdate struct {
	Revision string                    `json:"revision"`
	Changes  []ModStartupSettingChange `json:"changes"`
}

type modStartupSettingsEvaluation struct {
	Settings []modStartupEvaluatedSetting
}

type modStartupEvaluatedSetting struct {
	Mod     string
	Setting ModStartupSetting
}

var modStartupSettingsOperationMutex sync.Mutex

var modStartupSettingsCacheMutex sync.Mutex

var modStartupSettingsCache = make(map[string]modStartupSettingsEvaluation)

var evaluateActiveModStartupSettings = evaluateModStartupSettings

// GetModStartupSettings evaluates the startup-setting schema with the active
// profile's exact Factorio binary and enabled mod set. The lifecycle locks are
// owned here because direct callers and tests do not necessarily use the HTTP
// middleware.
func GetModStartupSettings() (ModStartupSettingsView, error) {
	unlock, err := lockModStartupSettingsOperation()
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	defer unlock()

	profile, version, document, existing, revision, err := activeModStartupSettingsState()
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	_ = document // Decoding here validates the existing PropertyTree before Factorio is run.

	evaluation, ok := cachedModStartupSettings(revision)
	if !ok {
		evaluation, err = evaluateActiveModStartupSettings(profile, version, existing)
		if err != nil {
			return ModStartupSettingsView{}, err
		}
		cacheModStartupSettings(revision, evaluation)
	}
	return buildModStartupSettingsView(profile, version, revision, evaluation), nil
}

// UpdateModStartupSettings validates requested values against an engine-
// evaluated schema, validates the complete candidate with Factorio, and only
// then atomically replaces mod-settings.dat. Unknown settings and both runtime
// sections remain untouched in the decoded PropertyTree.
func UpdateModStartupSettings(request ModStartupSettingsUpdate) (ModStartupSettingsView, error) {
	if err := validateModStartupSettingsUpdateEnvelope(request); err != nil {
		return ModStartupSettingsView{}, err
	}
	unlock, err := lockModStartupSettingsOperation()
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	defer unlock()

	profile, version, document, existing, revision, err := activeModStartupSettingsState()
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	if request.Revision != revision {
		return ModStartupSettingsView{}, ErrModStartupSettingsStale
	}

	evaluation, ok := cachedModStartupSettings(revision)
	if !ok {
		evaluation, err = evaluateActiveModStartupSettings(profile, version, existing)
		if err != nil {
			return ModStartupSettingsView{}, err
		}
	}
	candidate, expected, err := applyModStartupSettingsChanges(document, evaluation, request.Changes)
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	candidateContents, err := candidate.encode()
	if err != nil {
		return ModStartupSettingsView{}, fmt.Errorf("%w: encode candidate: %v", ErrInvalidModStartupSettings, err)
	}

	validated, err := evaluateActiveModStartupSettings(profile, version, candidateContents)
	if err != nil {
		return ModStartupSettingsView{}, fmt.Errorf("%w: Factorio rejected the candidate configuration", ErrInvalidModStartupSettings)
	}
	if err := verifyModStartupSettingsValues(validated, expected); err != nil {
		return ModStartupSettingsView{}, err
	}

	// The operation locks serialize manager activity. Recalculate immediately
	// before committing as a stale-form guard against out-of-band volume edits.
	_, freshSettings, err := loadModStartupSettingsDocument(filepath.Join(profileActiveDirectories()["mods"], "mod-settings.dat"), document.Version)
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	currentRevision, err := modStartupSettingsRevision(profile, version, freshSettings)
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	if currentRevision != revision || GetFactorioServer().IsBusy() {
		return ModStartupSettingsView{}, ErrModStartupSettingsStale
	}

	settingsPath := filepath.Join(profileActiveDirectories()["mods"], "mod-settings.dat")
	if err := FileLock.LockW(settingsPath); err != nil {
		return ModStartupSettingsView{}, fmt.Errorf("lock mod startup settings: %w", err)
	}
	err = writeFileAtomically(settingsPath, candidateContents, 0600)
	unlockErr := FileLock.Unlock(settingsPath)
	if err != nil {
		return ModStartupSettingsView{}, fmt.Errorf("save mod startup settings: %w", err)
	}
	if unlockErr != nil {
		return ModStartupSettingsView{}, fmt.Errorf("unlock mod startup settings: %w", unlockErr)
	}
	newRevision, err := modStartupSettingsRevision(profile, version, candidateContents)
	if err != nil {
		return ModStartupSettingsView{}, err
	}
	cacheModStartupSettings(newRevision, validated)
	return buildModStartupSettingsView(profile, version, newRevision, validated), nil
}

func lockModStartupSettingsOperation() (func(), error) {
	if !modStartupSettingsOperationMutex.TryLock() {
		return nil, ErrModStartupSettingsBusy
	}
	profileDataGate.RLock()
	serverLifecycleMutex.Lock()
	if GetFactorioServer().IsBusy() {
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		modStartupSettingsOperationMutex.Unlock()
		return nil, ErrServerActive
	}
	if !worldGenerationLock.TryLock() {
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		modStartupSettingsOperationMutex.Unlock()
		return nil, ErrModStartupSettingsBusy
	}
	if !mapSnapshotOperationMutex.TryLock() {
		worldGenerationLock.Unlock()
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		modStartupSettingsOperationMutex.Unlock()
		return nil, ErrModStartupSettingsBusy
	}
	factorioProgramFilesGate.RLock()
	return func() {
		factorioProgramFilesGate.RUnlock()
		mapSnapshotOperationMutex.Unlock()
		worldGenerationLock.Unlock()
		serverLifecycleMutex.Unlock()
		profileDataGate.RUnlock()
		modStartupSettingsOperationMutex.Unlock()
	}, nil
}

func buildModStartupSettingsView(profile Profile, version, revision string, evaluation modStartupSettingsEvaluation) ModStartupSettingsView {
	groupsByMod := make(map[string][]ModStartupSetting)
	for _, evaluated := range evaluation.Settings {
		groupsByMod[evaluated.Mod] = append(groupsByMod[evaluated.Mod], evaluated.Setting)
	}
	mods := make([]string, 0, len(groupsByMod))
	for mod := range groupsByMod {
		mods = append(mods, mod)
	}
	sort.SliceStable(mods, func(left, right int) bool {
		return strings.ToLower(modStartupModTitle(mods[left])) < strings.ToLower(modStartupModTitle(mods[right]))
	})
	groups := make([]ModStartupSettingsGroup, 0, len(mods))
	for _, mod := range mods {
		settings := groupsByMod[mod]
		sort.SliceStable(settings, func(left, right int) bool {
			if settings[left].Order != settings[right].Order {
				return settings[left].Order < settings[right].Order
			}
			return settings[left].Name < settings[right].Name
		})
		groups = append(groups, ModStartupSettingsGroup{Mod: mod, Title: modStartupModTitle(mod), Settings: settings})
	}
	return ModStartupSettingsView{
		ProfileID:       profile.ID,
		ProfileName:     profile.Name,
		FactorioVersion: version,
		Revision:        revision,
		Groups:          groups,
	}
}

func modStartupModTitle(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for index := range parts {
		runes := []rune(parts[index])
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			parts[index] = string(runes)
		}
	}
	if len(parts) == 0 {
		return name
	}
	return strings.Join(parts, " ")
}

func cachedModStartupSettings(revision string) (modStartupSettingsEvaluation, bool) {
	modStartupSettingsCacheMutex.Lock()
	defer modStartupSettingsCacheMutex.Unlock()
	evaluation, ok := modStartupSettingsCache[revision]
	return evaluation, ok
}

func cacheModStartupSettings(revision string, evaluation modStartupSettingsEvaluation) {
	modStartupSettingsCacheMutex.Lock()
	defer modStartupSettingsCacheMutex.Unlock()
	if len(modStartupSettingsCache) >= 8 {
		modStartupSettingsCache = make(map[string]modStartupSettingsEvaluation)
	}
	modStartupSettingsCache[revision] = evaluation
}
