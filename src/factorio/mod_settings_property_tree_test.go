package factorio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModSettingsPropertyTreeRoundTripPreservesEverySupportedType(t *testing.T) {
	text := "hello"
	longText := strings.Repeat("x", 255)
	document := newModSettingsDocument(Version{2, 1, 17, 0})
	document.Root.Children = append(document.Root.Children,
		propertyTreeNamedEntry("unknown", propertyTreeNode{Type: propertyTreeList, AnyType: true, Children: []propertyTreeEntry{
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeNone}},
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeBool, Bool: true}},
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeNumber, Number: 12.5}},
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeString, String: &text}},
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeString, String: &longText}},
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeString, String: nil}},
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeSignedInteger, Signed: -42}},
			{Name: nil, Value: propertyTreeNode{Type: propertyTreeUnsignedInteger, Unsigned: 42}},
		}}),
	)

	encoded, err := document.encode()
	require.NoError(t, err)
	decoded, err := decodeModSettingsDocument(encoded)
	require.NoError(t, err)
	reencoded, err := decoded.encode()
	require.NoError(t, err)
	assert.Equal(t, encoded, reencoded)
	assert.Equal(t, Version{2, 1, 17, 0}, decoded.Version)
}

func TestSetStartupValuePreservesUnknownAndRuntimeBranches(t *testing.T) {
	document := newModSettingsDocument(Version{2, 0, 77, 0})
	runtimeGlobal, err := dictionaryChild(&document.Root, "runtime-global", false)
	require.NoError(t, err)
	runtimeGlobal.Children = append(runtimeGlobal.Children, propertyTreeNamedEntry("secret-setting", propertyTreeNode{Type: propertyTreeString, String: stringPointer("keep-me")}))
	document.Root.Children = append(document.Root.Children, propertyTreeNamedEntry("future-section", propertyTreeNode{Type: propertyTreeUnsignedInteger, Unsigned: 99}))

	require.NoError(t, document.setStartupValue("example-enabled", propertyTreeNode{Type: propertyTreeBool, Bool: true}))
	require.NoError(t, document.setStartupValue("example-enabled", propertyTreeNode{Type: propertyTreeBool, Bool: false}))

	encoded, err := document.encode()
	require.NoError(t, err)
	decoded, err := decodeModSettingsDocument(encoded)
	require.NoError(t, err)
	startup, err := dictionaryChild(&decoded.Root, "startup", false)
	require.NoError(t, err)
	require.Len(t, startup.Children, 1)
	require.Len(t, startup.Children[0].Value.Children, 1)
	assert.False(t, startup.Children[0].Value.Children[0].Value.Bool)
	runtimeGlobal, err = dictionaryChild(&decoded.Root, "runtime-global", false)
	require.NoError(t, err)
	assert.Equal(t, "keep-me", *runtimeGlobal.Children[0].Value.String)
	assert.Equal(t, uint64(99), decoded.Root.Children[len(decoded.Root.Children)-1].Value.Unsigned)
}

func TestDecodeModSettingsRejectsMalformedAndUnsupportedDocuments(t *testing.T) {
	valid, err := newModSettingsDocument(Version{2, 0, 77, 0}).encode()
	require.NoError(t, err)

	for name, contents := range map[string][]byte{
		"truncated":        valid[:10],
		"trailing data":    append(append([]byte{}, valid...), 0xff),
		"header flag":      append(append([]byte{}, valid[:8]...), append([]byte{1}, valid[9:]...)...),
		"unsupported type": append(append([]byte{}, valid[:9]...), 8, 0),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeModSettingsDocument(contents)
			assert.Error(t, err)
		})
	}
}

func TestDecodeModSettingsRejectsNonFiniteNumbers(t *testing.T) {
	document := newModSettingsDocument(Version{2, 0, 77, 0})
	document.Root.Children = append(document.Root.Children, propertyTreeNamedEntry("bad", propertyTreeNode{Type: propertyTreeNumber, Number: math.Inf(1)}))
	_, err := document.encode()
	assert.Error(t, err)

	var raw bytes.Buffer
	var version [8]byte
	binary.LittleEndian.PutUint16(version[0:2], 2)
	raw.Write(version[:])
	raw.WriteByte(0)
	raw.Write([]byte{byte(propertyTreeNumber), 0})
	var number [8]byte
	binary.LittleEndian.PutUint64(number[:], math.Float64bits(math.NaN()))
	raw.Write(number[:])
	_, err = decodeModSettingsDocument(raw.Bytes())
	assert.Error(t, err)
}

func TestDictionaryChildReportsMissingOptionalBranch(t *testing.T) {
	document := newModSettingsDocument(Version{2, 0, 77, 0})
	_, err := dictionaryChild(&document.Root, "missing", false)
	var missing osErrNotExist
	assert.True(t, errors.As(err, &missing))
}

func TestModSettingsPropertyTreeReadsOfficialFixtureWhenProvided(t *testing.T) {
	path := os.Getenv("FSM_TEST_MOD_SETTINGS_FILE")
	if path == "" {
		t.Skip("FSM_TEST_MOD_SETTINGS_FILE is not set")
	}
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	document, err := decodeModSettingsDocument(contents)
	require.NoError(t, err)
	encoded, err := document.encode()
	require.NoError(t, err)
	assert.Equal(t, contents, encoded)
	startup, err := dictionaryChild(&document.Root, "startup", false)
	require.NoError(t, err)
	assert.NotEmpty(t, startup.Children)
	if testing.Verbose() {
		for _, entry := range startup.Children {
			if entry.Name == nil {
				continue
			}
			for _, field := range entry.Value.Children {
				if field.Name != nil && *field.Name == "value" {
					t.Logf("%s type=%d any=%t bool=%t number=%v children=%d", *entry.Name, field.Value.Type, field.Value.AnyType, field.Value.Bool, field.Value.Number, len(field.Value.Children))
				}
			}
		}
	}
}

func TestModSettingsPropertyTreeMutatesOfficialFixtureWhenProvided(t *testing.T) {
	path := os.Getenv("FSM_TEST_MOD_SETTINGS_FILE")
	if path == "" {
		t.Skip("FSM_TEST_MOD_SETTINGS_FILE is not set")
	}
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	document, err := decodeModSettingsDocument(contents)
	require.NoError(t, err)
	require.NoError(t, document.setStartupValue("fsm-fixture-bool", propertyTreeNode{Type: propertyTreeBool, Bool: false}))
	require.NoError(t, document.setStartupValue("fsm-fixture-int", propertyTreeNode{Type: propertyTreeNumber, Number: -3}))
	require.NoError(t, document.setStartupValue("fsm-fixture-double", propertyTreeNode{Type: propertyTreeNumber, Number: 3.5}))
	require.NoError(t, document.setStartupValue("fsm-fixture-choice", propertyTreeNode{Type: propertyTreeString, String: stringPointer("beta")}))
	require.NoError(t, document.setStartupValue("fsm-fixture-color", propertyTreeNode{Type: propertyTreeDictionary, Children: []propertyTreeEntry{
		propertyTreeNamedEntry("r", propertyTreeNode{Type: propertyTreeNumber, Number: 1}),
		propertyTreeNamedEntry("g", propertyTreeNode{Type: propertyTreeNumber, Number: 0}),
		propertyTreeNamedEntry("b", propertyTreeNode{Type: propertyTreeNumber, Number: 0.5}),
		propertyTreeNamedEntry("a", propertyTreeNode{Type: propertyTreeNumber, Number: 0.75}),
	}}))
	encoded, err := document.encode()
	require.NoError(t, err)
	decoded, err := decodeModSettingsDocument(encoded)
	require.NoError(t, err)
	assert.Equal(t, document.Root, decoded.Root)
	if output := os.Getenv("FSM_TEST_MOD_SETTINGS_OUTPUT"); output != "" {
		require.NoError(t, os.WriteFile(output, encoded, 0600))
	}
}

func stringPointer(value string) *string { return &value }
