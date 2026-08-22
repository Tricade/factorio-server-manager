package factorio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func floatPointer(value float64) *float64 { return &value }
func boolPointer(value bool) *bool        { return &value }
func uint32Pointer(value uint32) *uint32  { return &value }

func testWorldOptions() WorldGenerationOptions {
	return WorldGenerationOptions{
		Planets:  append(append([]WorldPlanet{}, baseWorldPlanets...), spaceAgeWorldPlanets...),
		Presets:  append([]WorldPreset{}, worldPresets...),
		Controls: append(append([]WorldControlDefinition{}, baseWorldControls...), spaceAgeWorldControls...),
	}
}

func TestNormalizeWorldGenerationRequestPreservesPresetDefaults(t *testing.T) {
	request, _, err := normalizeWorldGenerationRequestWithOptions(WorldGenerationRequest{
		Preset: "",
		Seed:   42,
	}, true, testWorldOptions())

	require.NoError(t, err)
	assert.Equal(t, "default", request.Preset)
	assert.Equal(t, "nauvis", request.Planet)
	assert.Equal(t, defaultMapPreviewSize, request.PreviewSize)
	assert.Nil(t, request.Width)
	assert.Nil(t, request.Height)
	assert.Nil(t, request.StartingArea)
	assert.Nil(t, request.PeacefulMode)
	assert.Nil(t, request.NoEnemiesMode)
}

func TestNormalizeWorldGenerationRequestAddsZipAndAcceptsOverrides(t *testing.T) {
	request, _, err := normalizeWorldGenerationRequestWithOptions(WorldGenerationRequest{
		Name:          "space-factory",
		Preset:        "rail-world",
		Seed:          4294967295,
		Width:         uint32Pointer(0),
		Height:        uint32Pointer(256),
		StartingArea:  floatPointer(2),
		PeacefulMode:  boolPointer(false),
		NoEnemiesMode: boolPointer(true),
		Controls: map[string]WorldControlSettings{
			"scrap": {Frequency: floatPointer(2), Size: floatPointer(4), Richness: floatPointer(6)},
		},
	}, false, testWorldOptions())

	require.NoError(t, err)
	assert.Equal(t, "space-factory.zip", request.Name)
}

func TestNormalizeWorldGenerationRequestRejectsUnavailablePlanet(t *testing.T) {
	options := testWorldOptions()
	options.Planets = append([]WorldPlanet{}, baseWorldPlanets...)

	_, _, err := normalizeWorldGenerationRequestWithOptions(WorldGenerationRequest{
		Preset: "default",
		Planet: "vulcanus",
	}, true, options)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestNormalizeWorldGenerationRequestRejectsDisabledRequiredTerrain(t *testing.T) {
	_, _, err := normalizeWorldGenerationRequestWithOptions(WorldGenerationRequest{
		Preset: "default",
		Controls: map[string]WorldControlSettings{
			"vulcanus_volcanism": {Frequency: floatPointer(0)},
		},
	}, true, testWorldOptions())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be disabled")
}

func TestBuildMapGenSettingsLeavesUnchangedPresetValuesOut(t *testing.T) {
	settings := buildMapGenSettings(WorldGenerationRequest{Seed: 123}, testWorldOptions())

	assert.Equal(t, uint64(123), settings["seed"])
	assert.NotContains(t, settings, "width")
	assert.NotContains(t, settings, "starting_area")
	assert.NotContains(t, settings, "peaceful_mode")
	assert.NotContains(t, settings, "no_enemies_mode")
	assert.NotContains(t, settings, "autoplace_controls")
}

func TestBuildMapGenSettingsIncludesOnlyExplicitControlValues(t *testing.T) {
	settings := buildMapGenSettings(WorldGenerationRequest{
		Seed:          123,
		Width:         uint32Pointer(0),
		PeacefulMode:  boolPointer(false),
		NoEnemiesMode: boolPointer(true),
		Controls: map[string]WorldControlSettings{
			"water": {Frequency: floatPointer(0.5), Size: floatPointer(2)},
		},
	}, testWorldOptions())

	assert.Equal(t, uint32(0), settings["width"])
	assert.Equal(t, false, settings["peaceful_mode"])
	assert.Equal(t, true, settings["no_enemies_mode"])
	autoplace := settings["autoplace_controls"].(map[string]map[string]float64)
	assert.Equal(t, map[string]float64{"frequency": 0.5, "size": 2}, autoplace["water"])
}

func TestRedactWorldCommandOutput(t *testing.T) {
	redacted := redactWorldCommandOutput([]byte("token=secret-value\n--rcon-password hunter2"))

	assert.NotContains(t, redacted, "secret-value")
	assert.NotContains(t, redacted, "hunter2")
	assert.Contains(t, redacted, "[REDACTED]")
}

func TestServerRunRejectsConcurrentWorldGeneration(t *testing.T) {
	worldGenerationLock.Lock()
	defer worldGenerationLock.Unlock()

	err := (&Server{}).Run()

	require.ErrorIs(t, err, ErrWorldGenerationBusy)
}

func TestCloseRconWithoutConnection(t *testing.T) {
	server := &Server{}
	require.NotPanics(t, server.closeRcon)
}
