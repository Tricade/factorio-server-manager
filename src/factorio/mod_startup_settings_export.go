package factorio

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	modStartupSettingsExporterName    = "fsm-mod-settings-exporter"
	modStartupSettingsExporterVersion = "0.1.0"
	modStartupSettingsSchemaVersion   = 1
)

//go:embed mod_settings_exporter/info.json mod_settings_exporter/control.lua
var modStartupSettingsExporterFiles embed.FS

var modStartupSettingsRunFactorio = runFactorioWorldCommand
var modStartupSettingsCommandTimeout = 5 * time.Minute

type modStartupSettingsManifest struct {
	SchemaVersion int                              `json:"schema_version"`
	Settings      []modStartupSettingsManifestItem `json:"settings"`
}

type modStartupSettingsManifestItem struct {
	Name          string            `json:"name"`
	Mod           string            `json:"mod"`
	Type          string            `json:"type"`
	Order         string            `json:"order"`
	CurrentValue  json.RawMessage   `json:"current_value"`
	DefaultValue  json.RawMessage   `json:"default_value"`
	MinimumValue  *float64          `json:"minimum_value"`
	MaximumValue  *float64          `json:"maximum_value"`
	AllowedValues []json.RawMessage `json:"allowed_values"`
	AllowBlank    bool              `json:"allow_blank"`
	AutoTrim      bool              `json:"auto_trim"`
	LocaleIndex   int               `json:"locale_index"`
}

func (manifest *modStartupSettingsManifest) UnmarshalJSON(contents []byte) error {
	var raw struct {
		SchemaVersion int             `json:"schema_version"`
		Settings      json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(contents, &raw); err != nil {
		return err
	}
	manifest.SchemaVersion = raw.SchemaVersion
	trimmed := bytes.TrimSpace(raw.Settings)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		manifest.Settings = []modStartupSettingsManifestItem{}
		return nil
	}
	return json.Unmarshal(trimmed, &manifest.Settings)
}

func evaluateModStartupSettings(_ Profile, version string, settingsContents []byte) (modStartupSettingsEvaluation, error) {
	workspace, err := os.MkdirTemp("", "fsm-mod-startup-settings-")
	if err != nil {
		return modStartupSettingsEvaluation{}, fmt.Errorf("create isolated mod settings workspace: %w", err)
	}
	defer os.RemoveAll(workspace)

	modsDirectory := filepath.Join(workspace, "mods")
	if err := prepareModStartupSettingsMods(profileActiveDirectories()["mods"], modsDirectory, version, settingsContents); err != nil {
		return modStartupSettingsEvaluation{}, fmt.Errorf("prepare isolated mod settings: %w", err)
	}
	writeDataDirectory := filepath.Join(workspace, "write-data")
	if err := os.MkdirAll(writeDataDirectory, 0700); err != nil {
		return modStartupSettingsEvaluation{}, err
	}
	configPath := filepath.Join(workspace, "config.ini")
	if err := writeIsolatedFactorioConfig(configPath, bootstrap.GetConfig().FactorioDir, writeDataDirectory); err != nil {
		return modStartupSettingsEvaluation{}, err
	}
	savePath := filepath.Join(workspace, "schema.zip")
	args := []string{
		"--config", configPath,
		"--mod-directory", modsDirectory,
		"--create", savePath,
		"--map-gen-seed", "1",
	}
	if err := modStartupSettingsRunFactorio(modStartupSettingsCommandTimeout, args); err != nil {
		return modStartupSettingsEvaluation{}, fmt.Errorf("run isolated Factorio settings evaluator: %w", err)
	}
	exportDirectory := filepath.Join(writeDataDirectory, "script-output", modStartupSettingsExporterName)
	complete, markerErr := readBoundedRegularFile(filepath.Join(exportDirectory, "complete"), 16, false)
	if markerErr != nil || strings.TrimSpace(string(complete)) != "ok" {
		if _, statErr := os.Stat(savePath); statErr != nil {
			return modStartupSettingsEvaluation{}, errors.New("isolated Factorio settings evaluator did not create a usable save")
		}
		benchmarkArgs := []string{
			"--config", configPath,
			"--mod-directory", modsDirectory,
			"--benchmark", savePath,
			"--benchmark-ticks", "1",
			"--benchmark-runs", "1",
			"--benchmark-sanitize",
		}
		if err := modStartupSettingsRunFactorio(modStartupSettingsCommandTimeout, benchmarkArgs); err != nil {
			return modStartupSettingsEvaluation{}, fmt.Errorf("complete isolated Factorio settings evaluator: %w", err)
		}
		complete, markerErr = readBoundedRegularFile(filepath.Join(exportDirectory, "complete"), 16, false)
	}
	if markerErr != nil || strings.TrimSpace(string(complete)) != "ok" {
		return modStartupSettingsEvaluation{}, errors.New("isolated Factorio settings evaluator did not complete")
	}
	return readModStartupSettingsEvaluation(exportDirectory)
}

func prepareModStartupSettingsMods(source, destination, factorioVersion string, settingsContents []byte) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	modListContents, err := readBoundedRegularFile(filepath.Join(source, "mod-list.json"), maximumModStartupModListBytes, true)
	if err != nil {
		return err
	}
	enabledMods, err := decodeEnabledModNames(modListContents)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(strings.ToLower(name), modStartupSettingsExporterName) {
			continue
		}
		if !mapSnapshotModEntryRequired(name, enabledMods) {
			continue
		}
		sourcePath := filepath.Join(source, name)
		destinationPath := filepath.Join(destination, name)
		info, err := os.Lstat(sourcePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse symbolic link %s", name)
		}
		if info.IsDir() {
			if err := copyProfileDirectory(sourcePath, destinationPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse non-regular mod entry %s", name)
		}
		if strings.EqualFold(filepath.Ext(name), ".zip") {
			if err := os.Link(sourcePath, destinationPath); err == nil {
				continue
			}
		}
		if err := copyProfileFile(sourcePath, destinationPath, info); err != nil {
			return err
		}
	}
	if settingsContents != nil {
		if len(settingsContents) == 0 || len(settingsContents) > maximumModSettingsFileBytes {
			return errors.New("candidate mod-settings.dat has an invalid size")
		}
		if err := os.WriteFile(filepath.Join(destination, "mod-settings.dat"), settingsContents, 0600); err != nil {
			return err
		}
	}
	if err := installModStartupSettingsExporter(destination, factorioVersion); err != nil {
		return err
	}
	return enableModStartupSettingsExporter(filepath.Join(destination, "mod-list.json"))
}

func installModStartupSettingsExporter(modsDirectory, factorioVersion string) error {
	destination := filepath.Join(modsDirectory, modStartupSettingsExporterName+"_"+modStartupSettingsExporterVersion)
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	for _, name := range []string{"info.json", "control.lua"} {
		contents, err := modStartupSettingsExporterFiles.ReadFile("mod_settings_exporter/" + name)
		if err != nil {
			return err
		}
		if name == "info.json" {
			var info map[string]any
			if err := json.Unmarshal(contents, &info); err != nil {
				return err
			}
			compatibilityVersion, err := mapSnapshotCompatibilityVersion(factorioVersion)
			if err != nil {
				return err
			}
			info["factorio_version"] = compatibilityVersion
			info["dependencies"] = []string{"base >= " + compatibilityVersion + ".0"}
			contents, err = json.MarshalIndent(info, "", "  ")
			if err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(destination, name), contents, 0600); err != nil {
			return err
		}
	}
	return nil
}

func enableModStartupSettingsExporter(path string) error {
	document := map[string]json.RawMessage{}
	contents, err := readBoundedRegularFile(path, maximumModStartupModListBytes, true)
	if err != nil {
		return err
	}
	if len(contents) == 0 {
		document["mods"] = json.RawMessage("[]")
	} else if err := json.Unmarshal(contents, &document); err != nil {
		return fmt.Errorf("decode isolated mod list: %w", err)
	}
	var mods []map[string]any
	if raw := document["mods"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &mods); err != nil {
			return fmt.Errorf("decode isolated mods: %w", err)
		}
	}
	if len(mods) > maximumSaveModImportItems*4 {
		return errors.New("isolated mod list contains too many entries")
	}
	baseFound := false
	exporterFound := false
	for _, mod := range mods {
		name, _ := mod["name"].(string)
		switch name {
		case "base":
			mod["enabled"] = true
			baseFound = true
		case modStartupSettingsExporterName:
			mod["enabled"] = true
			exporterFound = true
		}
	}
	if !baseFound {
		mods = append(mods, map[string]any{"name": "base", "enabled": true})
	}
	if !exporterFound {
		mods = append(mods, map[string]any{"name": modStartupSettingsExporterName, "enabled": true})
	}
	rawMods, err := json.Marshal(mods)
	if err != nil {
		return err
	}
	document["mods"] = rawMods
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomically(path, append(encoded, '\n'), 0600)
}

func readModStartupSettingsEvaluation(directory string) (modStartupSettingsEvaluation, error) {
	contents, err := readBoundedRegularFile(filepath.Join(directory, "manifest.json"), maximumModStartupManifestBytes, false)
	if err != nil {
		return modStartupSettingsEvaluation{}, fmt.Errorf("read settings schema: %w", err)
	}
	var manifest modStartupSettingsManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return modStartupSettingsEvaluation{}, fmt.Errorf("decode settings schema: %w", err)
	}
	if manifest.SchemaVersion != modStartupSettingsSchemaVersion {
		return modStartupSettingsEvaluation{}, fmt.Errorf("unsupported settings schema version %d", manifest.SchemaVersion)
	}
	if len(manifest.Settings) > maximumModStartupSettings {
		return modStartupSettingsEvaluation{}, fmt.Errorf("settings schema exceeds %d entries", maximumModStartupSettings)
	}
	seenNames := make(map[string]struct{}, len(manifest.Settings))
	seenLocales := make(map[int]struct{}, len(manifest.Settings))
	evaluation := modStartupSettingsEvaluation{Settings: make([]modStartupEvaluatedSetting, 0, len(manifest.Settings))}
	for _, item := range manifest.Settings {
		if err := validateModStartupManifestIdentity(item, seenNames, seenLocales); err != nil {
			return modStartupSettingsEvaluation{}, err
		}
		displayName, err := readModStartupLocale(directory, "name", item.LocaleIndex)
		if err != nil {
			return modStartupSettingsEvaluation{}, err
		}
		description, err := readModStartupLocale(directory, "description", item.LocaleIndex)
		if err != nil {
			return modStartupSettingsEvaluation{}, err
		}
		if displayName == "" || strings.HasPrefix(displayName, "Unknown key:") {
			displayName = item.Name
		}
		definition := ModStartupSetting{
			Name:         item.Name,
			DisplayName:  displayName,
			Description:  description,
			Type:         item.Type,
			MinimumValue: item.MinimumValue,
			MaximumValue: item.MaximumValue,
			AllowBlank:   item.AllowBlank,
			AutoTrim:     item.AutoTrim,
			Order:        item.Order,
		}
		if definition.MinimumValue != nil && (math.IsNaN(*definition.MinimumValue) || math.IsInf(*definition.MinimumValue, 0)) {
			return modStartupSettingsEvaluation{}, errors.New("settings schema contains a non-finite minimum")
		}
		if definition.MaximumValue != nil && (math.IsNaN(*definition.MaximumValue) || math.IsInf(*definition.MaximumValue, 0)) {
			return modStartupSettingsEvaluation{}, errors.New("settings schema contains a non-finite maximum")
		}
		if definition.MinimumValue != nil && definition.MaximumValue != nil && *definition.MinimumValue > *definition.MaximumValue {
			return modStartupSettingsEvaluation{}, errors.New("settings schema contains an inverted range")
		}
		for _, rawAllowed := range item.AllowedValues {
			if len(rawAllowed) > maximumModStartupValueBytes {
				return modStartupSettingsEvaluation{}, errors.New("settings schema contains an oversized allowed value")
			}
			allowed, err := decodeModStartupValue(item.Type, rawAllowed)
			if err != nil {
				return modStartupSettingsEvaluation{}, errors.New("settings schema contains an invalid allowed value")
			}
			definition.AllowedValues = append(definition.AllowedValues, allowed)
		}
		defaultValue, _, err := validateAndEncodeModStartupValue(definition, item.DefaultValue)
		if err != nil {
			return modStartupSettingsEvaluation{}, errors.New("settings schema contains an invalid default value")
		}
		currentValue, _, err := validateAndEncodeModStartupValue(definition, item.CurrentValue)
		if err != nil {
			return modStartupSettingsEvaluation{}, errors.New("settings schema contains an invalid current value")
		}
		definition.DefaultValue = defaultValue
		definition.Value = currentValue
		evaluation.Settings = append(evaluation.Settings, modStartupEvaluatedSetting{Mod: item.Mod, Setting: definition})
	}
	return evaluation, nil
}

func validateModStartupManifestIdentity(item modStartupSettingsManifestItem, seenNames map[string]struct{}, seenLocales map[int]struct{}) error {
	for label, value := range map[string]string{"setting name": item.Name, "mod name": item.Mod, "order": item.Order} {
		limit := 255
		if label == "order" {
			limit = 1024
		}
		if (label != "order" && value == "") || len(value) > limit || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return fmt.Errorf("settings schema contains an invalid %s", label)
		}
	}
	switch item.Type {
	case "bool-setting", "int-setting", "double-setting", "string-setting", "color-setting":
	default:
		return fmt.Errorf("settings schema contains unsupported type %q", item.Type)
	}
	if _, duplicate := seenNames[item.Name]; duplicate {
		return errors.New("settings schema contains a duplicate setting")
	}
	seenNames[item.Name] = struct{}{}
	if item.LocaleIndex <= 0 || item.LocaleIndex > maximumModStartupSettings {
		return errors.New("settings schema contains an invalid locale index")
	}
	if _, duplicate := seenLocales[item.LocaleIndex]; duplicate {
		return errors.New("settings schema contains a duplicate locale index")
	}
	seenLocales[item.LocaleIndex] = struct{}{}
	if len(item.CurrentValue) == 0 || len(item.DefaultValue) == 0 || len(item.CurrentValue) > maximumModStartupValueBytes || len(item.DefaultValue) > maximumModStartupValueBytes {
		return errors.New("settings schema contains a missing or oversized value")
	}
	return nil
}

func readModStartupLocale(directory, kind string, index int) (string, error) {
	path := filepath.Join(directory, "locale", kind+"-"+strconv.Itoa(index)+".txt")
	contents, err := readBoundedRegularFile(path, maximumModStartupLocaleBytes, false)
	if err != nil {
		return "", fmt.Errorf("read localized setting %s: %w", kind, err)
	}
	if !utf8.Valid(contents) || bytes.IndexByte(contents, 0) >= 0 {
		return "", fmt.Errorf("localized setting %s is invalid UTF-8", kind)
	}
	return strings.TrimSpace(string(contents)), nil
}

func sortedModStartupSettingNames(evaluation modStartupSettingsEvaluation) []string {
	names := make([]string, 0, len(evaluation.Settings))
	for _, setting := range evaluation.Settings {
		names = append(names, setting.Setting.Name)
	}
	sort.Strings(names)
	return names
}
