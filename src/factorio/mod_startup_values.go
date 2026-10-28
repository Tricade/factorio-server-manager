package factorio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
