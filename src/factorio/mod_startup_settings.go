package factorio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
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
	FileLock.LockW(settingsPath)
	err = writeFileAtomically(settingsPath, candidateContents, 0600)
	FileLock.Unlock(settingsPath)
	if err != nil {
		return ModStartupSettingsView{}, fmt.Errorf("save mod startup settings: %w", err)
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

func validateModStartupSettingsUpdateEnvelope(request ModStartupSettingsUpdate) error {
	if len(request.Revision) != sha256.Size*2 {
		return fmt.Errorf("%w: revision is invalid", ErrInvalidModStartupSettings)
	}
	if _, err := hex.DecodeString(request.Revision); err != nil {
		return fmt.Errorf("%w: revision is invalid", ErrInvalidModStartupSettings)
	}
	if len(request.Changes) == 0 || len(request.Changes) > maximumModStartupChanges {
		return fmt.Errorf("%w: changes must contain between 1 and %d items", ErrInvalidModStartupSettings, maximumModStartupChanges)
	}
	return nil
}

func applyModStartupSettingsChanges(document modSettingsDocument, evaluation modStartupSettingsEvaluation, changes []ModStartupSettingChange) (modSettingsDocument, map[string]any, error) {
	definitions := make(map[string]ModStartupSetting, len(evaluation.Settings))
	for _, evaluated := range evaluation.Settings {
		definitions[evaluated.Setting.Name] = evaluated.Setting
	}
	seen := make(map[string]struct{}, len(changes))
	expected := make(map[string]any, len(changes))
	for _, change := range changes {
		if len(change.Name) == 0 || len(change.Name) > 255 || strings.IndexFunc(change.Name, unicode.IsControl) >= 0 {
			return modSettingsDocument{}, nil, fmt.Errorf("%w: setting name is invalid", ErrInvalidModStartupSettings)
		}
		if _, duplicate := seen[change.Name]; duplicate {
			return modSettingsDocument{}, nil, fmt.Errorf("%w: duplicate setting name", ErrInvalidModStartupSettings)
		}
		seen[change.Name] = struct{}{}
		definition, ok := definitions[change.Name]
		if !ok {
			return modSettingsDocument{}, nil, fmt.Errorf("%w: setting is not available", ErrInvalidModStartupSettings)
		}
		value, node, err := validateAndEncodeModStartupValue(definition, change.Value)
		if err != nil {
			return modSettingsDocument{}, nil, err
		}
		if definition.Type == "int-setting" && !document.Version.Less(Version{2, 0, 0, 0}) {
			node = propertyTreeNode{Type: propertyTreeSignedInteger, Signed: value.(int64)}
		}
		if err := document.setStartupValue(change.Name, node); err != nil {
			return modSettingsDocument{}, nil, err
		}
		expected[change.Name] = value
	}
	return document, expected, nil
}

func validateAndEncodeModStartupValue(definition ModStartupSetting, raw json.RawMessage) (any, propertyTreeNode, error) {
	if len(raw) == 0 || len(raw) > maximumModStartupValueBytes {
		return nil, propertyTreeNode{}, fmt.Errorf("%w: value for %s has an invalid size", ErrInvalidModStartupSettings, definition.Name)
	}
	value, err := decodeModStartupValue(definition.Type, raw)
	if err != nil {
		return nil, propertyTreeNode{}, fmt.Errorf("%w: value for %s has the wrong type", ErrInvalidModStartupSettings, definition.Name)
	}
	if text, ok := value.(string); ok {
		if definition.AutoTrim {
			text = strings.TrimSpace(text)
			value = text
		}
		if !definition.AllowBlank && text == "" {
			return nil, propertyTreeNode{}, fmt.Errorf("%w: %s does not allow a blank value", ErrInvalidModStartupSettings, definition.Name)
		}
		if !utf8.ValidString(text) || len(text) > maximumModStartupValueBytes || strings.IndexByte(text, 0) >= 0 {
			return nil, propertyTreeNode{}, fmt.Errorf("%w: value for %s is invalid", ErrInvalidModStartupSettings, definition.Name)
		}
	}
	if number, ok := modStartupNumericValue(value); ok {
		if definition.MinimumValue != nil && number < *definition.MinimumValue {
			return nil, propertyTreeNode{}, fmt.Errorf("%w: value for %s is below its minimum", ErrInvalidModStartupSettings, definition.Name)
		}
		if definition.MaximumValue != nil && number > *definition.MaximumValue {
			return nil, propertyTreeNode{}, fmt.Errorf("%w: value for %s is above its maximum", ErrInvalidModStartupSettings, definition.Name)
		}
	}
	if len(definition.AllowedValues) > 0 {
		allowed := false
		for _, candidate := range definition.AllowedValues {
			if reflect.DeepEqual(candidate, value) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, propertyTreeNode{}, fmt.Errorf("%w: value for %s is not allowed", ErrInvalidModStartupSettings, definition.Name)
		}
	}
	node, err := modStartupValuePropertyNode(definition.Type, value)
	if err != nil {
		return nil, propertyTreeNode{}, err
	}
	return value, node, nil
}

func decodeModStartupValue(settingType string, raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("value contains trailing JSON")
	}
	switch settingType {
	case "bool-setting":
		value, ok := decoded.(bool)
		if !ok {
			return nil, errors.New("expected boolean")
		}
		return value, nil
	case "int-setting":
		number, ok := decoded.(json.Number)
		if !ok {
			return nil, errors.New("expected integer")
		}
		value, err := strconv.ParseInt(number.String(), 10, 32)
		if err != nil {
			floating, floatingErr := strconv.ParseFloat(number.String(), 64)
			if floatingErr != nil || math.Trunc(floating) != floating || floating < math.MinInt32 || floating > math.MaxInt32 {
				return nil, errors.New("expected int32")
			}
			value = int64(floating)
		}
		return value, nil
	case "double-setting":
		number, ok := decoded.(json.Number)
		if !ok {
			return nil, errors.New("expected number")
		}
		value, err := strconv.ParseFloat(number.String(), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("expected finite number")
		}
		return value, nil
	case "string-setting":
		value, ok := decoded.(string)
		if !ok {
			return nil, errors.New("expected string")
		}
		return value, nil
	case "color-setting":
		object, ok := decoded.(map[string]any)
		if !ok {
			return nil, errors.New("expected color object")
		}
		if len(object) < 3 || len(object) > 4 {
			return nil, errors.New("expected r, g, b and optional a")
		}
		color := make(map[string]float64, 4)
		for _, component := range []string{"r", "g", "b", "a"} {
			rawComponent, exists := object[component]
			if !exists && component == "a" {
				color[component] = 1
				continue
			}
			number, ok := rawComponent.(json.Number)
			if !ok {
				return nil, errors.New("color component is not numeric")
			}
			value, err := strconv.ParseFloat(number.String(), 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				return nil, errors.New("color component is outside 0..1")
			}
			color[component] = value
		}
		for key := range object {
			if key != "r" && key != "g" && key != "b" && key != "a" {
				return nil, errors.New("unknown color component")
			}
		}
		return color, nil
	default:
		return nil, fmt.Errorf("unsupported mod setting type %q", settingType)
	}
}

func modStartupValuePropertyNode(settingType string, value any) (propertyTreeNode, error) {
	switch settingType {
	case "bool-setting":
		return propertyTreeNode{Type: propertyTreeBool, Bool: value.(bool)}, nil
	case "int-setting":
		return propertyTreeNode{Type: propertyTreeNumber, Number: float64(value.(int64))}, nil
	case "double-setting":
		return propertyTreeNode{Type: propertyTreeNumber, Number: value.(float64)}, nil
	case "string-setting":
		text := value.(string)
		return propertyTreeNode{Type: propertyTreeString, String: &text}, nil
	case "color-setting":
		color := value.(map[string]float64)
		children := make([]propertyTreeEntry, 0, 4)
		for _, component := range []string{"r", "g", "b", "a"} {
			children = append(children, propertyTreeNamedEntry(component, propertyTreeNode{Type: propertyTreeNumber, Number: color[component]}))
		}
		return propertyTreeNode{Type: propertyTreeDictionary, Children: children}, nil
	default:
		return propertyTreeNode{}, fmt.Errorf("%w: unsupported setting type", ErrInvalidModStartupSettings)
	}
}

func modStartupNumericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func verifyModStartupSettingsValues(evaluation modStartupSettingsEvaluation, expected map[string]any) error {
	actual := make(map[string]any, len(evaluation.Settings))
	for _, evaluated := range evaluation.Settings {
		actual[evaluated.Setting.Name] = evaluated.Setting.Value
	}
	for name, value := range expected {
		if !reflect.DeepEqual(actual[name], value) {
			return fmt.Errorf("%w: Factorio did not accept one or more requested values", ErrInvalidModStartupSettings)
		}
	}
	return nil
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
