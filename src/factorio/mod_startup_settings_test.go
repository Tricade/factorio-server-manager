package factorio

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateModStartupSettingsCommitsValidatedProfileFile(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	originalContents := testModStartupSettingsContents(t, true, "keep-runtime-value")
	settingsPath := filepath.Join(environment.active["mods"], "mod-settings.dat")
	require.NoError(t, os.WriteFile(settingsPath, originalContents, 0644))
	installModStartupSettingsEvaluatorStub(t, nil)

	view, err := GetModStartupSettings()
	require.NoError(t, err)
	require.Len(t, view.Groups, 1)
	assert.Equal(t, true, view.Groups[0].Settings[0].Value)

	updated, err := UpdateModStartupSettings(ModStartupSettingsUpdate{
		Revision: view.Revision,
		Changes:  []ModStartupSettingChange{{Name: "fixture-enabled", Value: json.RawMessage("false")}},
	})
	require.NoError(t, err)
	assert.NotEqual(t, view.Revision, updated.Revision)
	assert.Equal(t, false, updated.Groups[0].Settings[0].Value)

	committed, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	document, err := decodeModSettingsDocument(committed)
	require.NoError(t, err)
	assert.False(t, testModStartupSettingsBool(t, document))
	runtimeGlobal, err := dictionaryChild(&document.Root, "runtime-global", false)
	require.NoError(t, err)
	require.Len(t, runtimeGlobal.Children, 1)
	assert.Equal(t, "keep-runtime-value", *runtimeGlobal.Children[0].Value.String)
	testAssertNoAtomicModSettingsFiles(t, environment.active["mods"])
}

func TestUpdateModStartupSettingsRejectsStaleProfileState(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	settingsPath := filepath.Join(environment.active["mods"], "mod-settings.dat")
	originalContents := testModStartupSettingsContents(t, true, "keep-runtime-value")
	require.NoError(t, os.WriteFile(settingsPath, originalContents, 0644))
	installModStartupSettingsEvaluatorStub(t, nil)

	view, err := GetModStartupSettings()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(environment.active["mods"], "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true},{"name":"quality","enabled":true}]}`), 0644))

	_, err = UpdateModStartupSettings(ModStartupSettingsUpdate{
		Revision: view.Revision,
		Changes:  []ModStartupSettingChange{{Name: "fixture-enabled", Value: json.RawMessage("false")}},
	})
	assert.ErrorIs(t, err, ErrModStartupSettingsStale)
	committed, readErr := os.ReadFile(settingsPath)
	require.NoError(t, readErr)
	assert.Equal(t, originalContents, committed)
}

func TestUpdateModStartupSettingsKeepsPreviousFileWhenFactorioRejectsCandidate(t *testing.T) {
	environment := setupProfileTest(t)
	require.NoError(t, InitializeProfiles())
	settingsPath := filepath.Join(environment.active["mods"], "mod-settings.dat")
	originalContents := testModStartupSettingsContents(t, true, "keep-runtime-value")
	require.NoError(t, os.WriteFile(settingsPath, originalContents, 0644))
	installModStartupSettingsEvaluatorStub(t, func(contents []byte) error {
		if !bytes.Equal(contents, originalContents) {
			return errors.New("simulated Factorio validation failure")
		}
		return nil
	})

	view, err := GetModStartupSettings()
	require.NoError(t, err)
	_, err = UpdateModStartupSettings(ModStartupSettingsUpdate{
		Revision: view.Revision,
		Changes:  []ModStartupSettingChange{{Name: "fixture-enabled", Value: json.RawMessage("false")}},
	})
	assert.ErrorIs(t, err, ErrInvalidModStartupSettings)
	committed, readErr := os.ReadFile(settingsPath)
	require.NoError(t, readErr)
	assert.Equal(t, originalContents, committed)
	testAssertNoAtomicModSettingsFiles(t, environment.active["mods"])
}

func TestApplyModStartupSettingsChangesSupportsEveryControlTypeAndPreservesRuntime(t *testing.T) {
	document := newModSettingsDocument(Version{2, 0, 77, 0})
	runtimeGlobal, err := dictionaryChild(&document.Root, "runtime-global", false)
	require.NoError(t, err)
	runtimeGlobal.Children = append(runtimeGlobal.Children, propertyTreeNamedEntry("private-runtime-value", propertyTreeNode{
		Type: propertyTreeString, String: stringPointer("unchanged"),
	}))
	minimum := -10.0
	maximum := 10.0
	evaluation := modStartupSettingsEvaluation{Settings: []modStartupEvaluatedSetting{
		{Mod: "fixture", Setting: ModStartupSetting{Name: "bool", Type: "bool-setting", Value: true, DefaultValue: true}},
		{Mod: "fixture", Setting: ModStartupSetting{Name: "int", Type: "int-setting", Value: int64(1), DefaultValue: int64(1), MinimumValue: &minimum, MaximumValue: &maximum}},
		{Mod: "fixture", Setting: ModStartupSetting{Name: "double", Type: "double-setting", Value: 1.0, DefaultValue: 1.0, MinimumValue: &minimum, MaximumValue: &maximum}},
		{Mod: "fixture", Setting: ModStartupSetting{Name: "string", Type: "string-setting", Value: "alpha", DefaultValue: "alpha", AllowedValues: []any{"alpha", "beta"}}},
		{Mod: "fixture", Setting: ModStartupSetting{Name: "color", Type: "color-setting", Value: map[string]float64{"r": 0.0, "g": 0.0, "b": 0.0, "a": 1.0}, DefaultValue: map[string]float64{"r": 0.0, "g": 0.0, "b": 0.0, "a": 1.0}}},
	}}
	changes := []ModStartupSettingChange{
		{Name: "bool", Value: json.RawMessage("false")},
		{Name: "int", Value: json.RawMessage("-3")},
		{Name: "double", Value: json.RawMessage("3.5")},
		{Name: "string", Value: json.RawMessage(`"beta"`)},
		{Name: "color", Value: json.RawMessage(`{"r":1,"g":0,"b":0.5,"a":0.75}`)},
	}

	candidate, expected, err := applyModStartupSettingsChanges(document, evaluation, changes)
	require.NoError(t, err)
	assert.Equal(t, false, expected["bool"])
	assert.Equal(t, int64(-3), expected["int"])
	encoded, err := candidate.encode()
	require.NoError(t, err)
	decoded, err := decodeModSettingsDocument(encoded)
	require.NoError(t, err)
	runtimeGlobal, err = dictionaryChild(&decoded.Root, "runtime-global", false)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", *runtimeGlobal.Children[0].Value.String)
	startup, err := dictionaryChild(&decoded.Root, "startup", false)
	require.NoError(t, err)
	for _, entry := range startup.Children {
		if entry.Name != nil && *entry.Name == "int" {
			assert.Equal(t, propertyTreeSignedInteger, entry.Value.Children[0].Value.Type)
			assert.Equal(t, int64(-3), entry.Value.Children[0].Value.Signed)
		}
	}
}

func TestApplyModStartupSettingsChangesRejectsInvalidAndDuplicateValues(t *testing.T) {
	minimum := 1.0
	maximum := 5.0
	evaluation := modStartupSettingsEvaluation{Settings: []modStartupEvaluatedSetting{{Mod: "fixture", Setting: ModStartupSetting{
		Name: "count", Type: "int-setting", Value: int64(2), DefaultValue: int64(2), MinimumValue: &minimum, MaximumValue: &maximum,
	}}}}
	document := newModSettingsDocument(Version{2, 0, 77, 0})
	for name, changes := range map[string][]ModStartupSettingChange{
		"unknown":    {{Name: "unknown", Value: json.RawMessage("2")}},
		"wrong type": {{Name: "count", Value: json.RawMessage(`"2"`)}},
		"below min":  {{Name: "count", Value: json.RawMessage("0")}},
		"duplicate":  {{Name: "count", Value: json.RawMessage("2")}, {Name: "count", Value: json.RawMessage("3")}},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := applyModStartupSettingsChanges(document, evaluation, changes)
			assert.ErrorIs(t, err, ErrInvalidModStartupSettings)
		})
	}
}

func TestReadModStartupSettingsEvaluationLocalizesGroupsAndValues(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "locale"), 0700))
	manifest := `{"schema_version":1,"settings":[` +
		`{"name":"fixture-enabled","mod":"fixture-mod","type":"bool-setting","order":"a","current_value":false,"default_value":true,"locale_index":1},` +
		`{"name":"fixture-mode","mod":"fixture-mod","type":"string-setting","order":"b","current_value":"beta","default_value":"alpha","allowed_values":["alpha","beta"],"allow_blank":false,"auto_trim":true,"locale_index":2}` +
		`]}`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "manifest.json"), []byte(manifest), 0600))
	for path, contents := range map[string]string{
		"name-1.txt":        "Enable fixture\n",
		"description-1.txt": "A localized description.\n",
		"name-2.txt":        "Fixture mode\n",
		"description-2.txt": "Choose the fixture mode.\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(directory, "locale", path), []byte(contents), 0600))
	}

	evaluation, err := readModStartupSettingsEvaluation(directory)
	require.NoError(t, err)
	require.Len(t, evaluation.Settings, 2)
	assert.Equal(t, "Enable fixture", evaluation.Settings[0].Setting.DisplayName)
	assert.Equal(t, false, evaluation.Settings[0].Setting.Value)
	assert.Equal(t, []any{"alpha", "beta"}, evaluation.Settings[1].Setting.AllowedValues)
	assert.True(t, evaluation.Settings[1].Setting.AutoTrim)
	view := buildModStartupSettingsView(Profile{ID: "0123456789abcdef", Name: "Test"}, "2.0.77", strings.Repeat("a", 64), evaluation)
	require.Len(t, view.Groups, 1)
	assert.Equal(t, "Fixture Mod", view.Groups[0].Title)
	assert.Equal(t, "fixture-mod", view.Groups[0].Mod)
}

func TestReadModStartupSettingsEvaluationRejectsMalformedData(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "manifest.json"), []byte(`{"schema_version":1,"settings":[{"name":"bad","mod":"fixture","type":"unsupported","current_value":true,"default_value":true,"locale_index":1}]}`), 0600))
	_, err := readModStartupSettingsEvaluation(directory)
	assert.Error(t, err)
}

func TestModStartupSettingsFileErrorsDoNotBlockNormalModManagement(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "mod-list.json"), []byte(`{"mods":[{"name":"base","enabled":true}]}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "mod-settings.dat"), []byte("malformed"), 0600))

	_, _, err := loadModStartupSettingsDocument(filepath.Join(directory, "mod-settings.dat"), Version{2, 1, 17, 0})
	assert.ErrorIs(t, err, ErrInvalidModSettingsFile)
	mods, err := NewMods(directory)
	require.NoError(t, err)
	assert.Empty(t, mods.ListInstalledMods().ModsResult)
}

func TestMissingModStartupSettingsFileUsesAnEmptyVersionedDocument(t *testing.T) {
	document, contents, err := loadModStartupSettingsDocument(filepath.Join(t.TempDir(), "mod-settings.dat"), Version{1, 1, 110, 0})
	require.NoError(t, err)
	assert.Nil(t, contents)
	assert.Equal(t, Version{1, 1, 110, 0}, document.Version)
	assert.Equal(t, propertyTreeDictionary, document.Root.Type)
}

func TestModStartupSettingsUpdateEnvelopeIsBounded(t *testing.T) {
	valid := ModStartupSettingsUpdate{Revision: strings.Repeat("a", 64), Changes: []ModStartupSettingChange{{Name: "one", Value: json.RawMessage("true")}}}
	assert.NoError(t, validateModStartupSettingsUpdateEnvelope(valid))
	invalidRevision := valid
	invalidRevision.Revision = strings.Repeat("z", 64)
	assert.ErrorIs(t, validateModStartupSettingsUpdateEnvelope(invalidRevision), ErrInvalidModStartupSettings)
	empty := valid
	empty.Changes = nil
	assert.ErrorIs(t, validateModStartupSettingsUpdateEnvelope(empty), ErrInvalidModStartupSettings)
}

func TestModStartupSettingsOperationRejectsConcurrentEvaluation(t *testing.T) {
	modStartupSettingsOperationMutex.Lock()
	t.Cleanup(modStartupSettingsOperationMutex.Unlock)
	unlock, err := lockModStartupSettingsOperation()
	assert.Nil(t, unlock)
	assert.ErrorIs(t, err, ErrModStartupSettingsBusy)
}

func TestVerifyModStartupSettingsValuesRejectsEngineCoercion(t *testing.T) {
	evaluation := modStartupSettingsEvaluation{Settings: []modStartupEvaluatedSetting{{Mod: "fixture", Setting: ModStartupSetting{Name: "enabled", Value: true}}}}
	assert.NoError(t, verifyModStartupSettingsValues(evaluation, map[string]any{"enabled": true}))
	assert.ErrorIs(t, verifyModStartupSettingsValues(evaluation, map[string]any{"enabled": false}), ErrInvalidModStartupSettings)
}

func testModStartupSettingsContents(t *testing.T, enabled bool, runtimeValue string) []byte {
	t.Helper()
	document := newModSettingsDocument(Version{2, 1, 14, 0})
	require.NoError(t, document.setStartupValue("fixture-enabled", propertyTreeNode{Type: propertyTreeBool, Bool: enabled}))
	runtimeGlobal, err := dictionaryChild(&document.Root, "runtime-global", false)
	require.NoError(t, err)
	runtimeGlobal.Children = append(runtimeGlobal.Children, propertyTreeNamedEntry("private-runtime-value", propertyTreeNode{
		Type: propertyTreeString, String: stringPointer(runtimeValue),
	}))
	contents, err := document.encode()
	require.NoError(t, err)
	return contents
}

func installModStartupSettingsEvaluatorStub(t *testing.T, reject func([]byte) error) {
	t.Helper()
	originalEvaluator := evaluateActiveModStartupSettings
	evaluateActiveModStartupSettings = func(_ Profile, _ string, contents []byte) (modStartupSettingsEvaluation, error) {
		if reject != nil {
			if err := reject(contents); err != nil {
				return modStartupSettingsEvaluation{}, err
			}
		}
		document, err := decodeModSettingsDocument(contents)
		if err != nil {
			return modStartupSettingsEvaluation{}, err
		}
		return modStartupSettingsEvaluation{Settings: []modStartupEvaluatedSetting{{
			Mod: "fixture-mod",
			Setting: ModStartupSetting{
				Name: "fixture-enabled", DisplayName: "Enable fixture", Description: "Fixture description.",
				Type: "bool-setting", Value: testModStartupSettingsBool(t, document), DefaultValue: true,
			},
		}}}, nil
	}
	modStartupSettingsCacheMutex.Lock()
	modStartupSettingsCache = make(map[string]modStartupSettingsEvaluation)
	modStartupSettingsCacheMutex.Unlock()
	t.Cleanup(func() {
		evaluateActiveModStartupSettings = originalEvaluator
		modStartupSettingsCacheMutex.Lock()
		modStartupSettingsCache = make(map[string]modStartupSettingsEvaluation)
		modStartupSettingsCacheMutex.Unlock()
	})
}

func testModStartupSettingsBool(t *testing.T, document modSettingsDocument) bool {
	t.Helper()
	startup, err := dictionaryChild(&document.Root, "startup", false)
	require.NoError(t, err)
	setting, err := dictionaryChild(startup, "fixture-enabled", false)
	require.NoError(t, err)
	for _, entry := range setting.Children {
		if entry.Name != nil && *entry.Name == "value" {
			require.Equal(t, propertyTreeBool, entry.Value.Type)
			return entry.Value.Bool
		}
	}
	t.Fatal("fixture-enabled value not found")
	return false
}

func testAssertNoAtomicModSettingsFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".fsm-write-") || strings.HasPrefix(entry.Name(), ".fsm-backup-") {
			t.Fatalf("startup-settings transaction file was left behind: %s", entry.Name())
		}
	}
}
