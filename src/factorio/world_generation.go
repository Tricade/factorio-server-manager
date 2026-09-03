package factorio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	defaultMapPreviewSize = 768
	maxMapPreviewSize     = 1536
	maxGeneratedPreview   = 24 * 1024 * 1024
	maxWorldDimension     = 2_000_000
)

var (
	ErrWorldGenerationBusy   = errors.New("another world preview or creation is already running")
	ErrFactorioMustBeStopped = errors.New("stop Factorio before generating a world")
	ErrSaveAlreadyExists     = errors.New("a save with this name already exists")

	worldGenerationLock sync.Mutex
	worldIdentifier     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

type WorldPreset struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type WorldPlanet struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type WorldControlDefinition struct {
	Name             string `json:"name"`
	Label            string `json:"label"`
	Planet           string `json:"planet"`
	Category         string `json:"category"`
	SupportsRichness bool   `json:"supports_richness"`
	CanDisable       bool   `json:"can_disable"`
}

type WorldGenerationOptions struct {
	FactorioVersion string                   `json:"factorio_version"`
	GameMode        GameMode                 `json:"game_mode"`
	Planets         []WorldPlanet            `json:"planets"`
	Presets         []WorldPreset            `json:"presets"`
	Controls        []WorldControlDefinition `json:"controls"`
	PreviewSizes    []int                    `json:"preview_sizes"`
}

type WorldControlSettings struct {
	Frequency *float64 `json:"frequency,omitempty"`
	Size      *float64 `json:"size,omitempty"`
	Richness  *float64 `json:"richness,omitempty"`
}

type WorldGenerationRequest struct {
	Name          string                          `json:"name,omitempty"`
	Preset        string                          `json:"preset"`
	Seed          uint64                          `json:"seed"`
	Planet        string                          `json:"planet,omitempty"`
	PreviewSize   int                             `json:"preview_size,omitempty"`
	Width         *uint32                         `json:"width,omitempty"`
	Height        *uint32                         `json:"height,omitempty"`
	StartingArea  *float64                        `json:"starting_area,omitempty"`
	PeacefulMode  *bool                           `json:"peaceful_mode,omitempty"`
	NoEnemiesMode *bool                           `json:"no_enemies_mode,omitempty"`
	Controls      map[string]WorldControlSettings `json:"controls,omitempty"`
}

var worldPresets = []WorldPreset{
	{Name: "default", Label: "Default", Description: "Factorio's balanced default world."},
	{Name: "rich-resources", Label: "Rich resources", Description: "Default layout with much richer deposits."},
	{Name: "marathon", Label: "Marathon", Description: "More expensive technology and a longer progression."},
	{Name: "death-world", Label: "Death world", Description: "Denser enemies and faster evolution."},
	{Name: "death-world-marathon", Label: "Death world marathon", Description: "Death world pressure with marathon costs."},
	{Name: "rail-world", Label: "Rail world", Description: "Large, widely spaced deposits built for trains."},
	{Name: "ribbon-world", Label: "Ribbon world", Description: "A narrow map with resources adjusted for the limited space."},
	{Name: "lakes", Label: "Lakes", Description: "A wetter landscape with larger connected lakes."},
	{Name: "island", Label: "Island", Description: "Start on a bounded island surrounded by water."},
}

var baseWorldPlanets = []WorldPlanet{
	{Name: "nauvis", Label: "Nauvis", Description: "The starting planet and primary factory world."},
}

var spaceAgeWorldPlanets = []WorldPlanet{
	{Name: "vulcanus", Label: "Vulcanus", Description: "Volcanic terrain, lava and heavy industry resources."},
	{Name: "gleba", Label: "Gleba", Description: "Wet organic terrain, agriculture and pentapods."},
	{Name: "fulgora", Label: "Fulgora", Description: "Scrap islands separated by oil oceans."},
	{Name: "aquilo", Label: "Aquilo", Description: "Frozen islands with scarce geothermal resources."},
}

var baseWorldControls = []WorldControlDefinition{
	{Name: "iron-ore", Label: "Iron ore", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "copper-ore", Label: "Copper ore", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "coal", Label: "Coal", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "stone", Label: "Stone", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "uranium-ore", Label: "Uranium ore", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "crude-oil", Label: "Crude oil", Planet: "nauvis", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "water", Label: "Water", Planet: "nauvis", Category: "terrain", CanDisable: true},
	{Name: "trees", Label: "Trees", Planet: "nauvis", Category: "terrain", CanDisable: true},
	{Name: "enemy-base", Label: "Enemy bases", Planet: "nauvis", Category: "enemy", CanDisable: true},
	{Name: "nauvis_cliff", Label: "Cliffs", Planet: "nauvis", Category: "terrain", CanDisable: true},
}

var spaceAgeWorldControls = []WorldControlDefinition{
	{Name: "vulcanus_coal", Label: "Coal", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "tungsten_ore", Label: "Tungsten ore", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "calcite", Label: "Calcite", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "sulfuric_acid_geyser", Label: "Sulfuric acid geysers", Planet: "vulcanus", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "vulcanus_volcanism", Label: "Volcanism", Planet: "vulcanus", Category: "terrain", CanDisable: false},
	{Name: "gleba_stone", Label: "Stone", Planet: "gleba", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "gleba_plants", Label: "Plants", Planet: "gleba", Category: "terrain", CanDisable: false},
	{Name: "gleba_water", Label: "Water", Planet: "gleba", Category: "terrain", CanDisable: false},
	{Name: "gleba_enemy_base", Label: "Pentapod nests", Planet: "gleba", Category: "enemy", CanDisable: false},
	{Name: "gleba_cliff", Label: "Cliffs", Planet: "gleba", Category: "terrain", CanDisable: true},
	{Name: "scrap", Label: "Scrap", Planet: "fulgora", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "fulgora_islands", Label: "Island size", Planet: "fulgora", Category: "terrain", CanDisable: false},
	{Name: "fulgora_cliff", Label: "Cliffs", Planet: "fulgora", Category: "terrain", CanDisable: true},
	{Name: "aquilo_crude_oil", Label: "Crude oil", Planet: "aquilo", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "lithium_brine", Label: "Lithium brine", Planet: "aquilo", Category: "resource", SupportsRichness: true, CanDisable: true},
	{Name: "fluorine_vent", Label: "Fluorine vents", Planet: "aquilo", Category: "resource", SupportsRichness: true, CanDisable: true},
}

func GetWorldGenerationOptions() (WorldGenerationOptions, error) {
	snapshot := GetFactorioServer().Snapshot()
	modeStatus, err := GetGameModeStatus()
	if err != nil {
		return WorldGenerationOptions{}, fmt.Errorf("inspect game mode: %w", err)
	}

	planets := append([]WorldPlanet{}, baseWorldPlanets...)
	controls := append([]WorldControlDefinition{}, baseWorldControls...)
	spaceAgeEnabled := false
	for _, feature := range modeStatus.Features {
		if feature.Name == "space-age" && feature.Enabled {
			spaceAgeEnabled = true
			break
		}
	}
	if spaceAgeEnabled {
		planets = append(planets, spaceAgeWorldPlanets...)
		controls = append(controls, spaceAgeWorldControls...)
	}

	return WorldGenerationOptions{
		FactorioVersion: snapshot.Version.String(),
		GameMode:        modeStatus.Mode,
		Planets:         planets,
		Presets:         append([]WorldPreset{}, worldPresets...),
		Controls:        controls,
		PreviewSizes:    []int{512, defaultMapPreviewSize, 1024, maxMapPreviewSize},
	}, nil
}

func GenerateWorldPreview(request WorldGenerationRequest) ([]byte, error) {
	normalized, _, err := normalizeWorldGenerationRequest(request, true)
	if err != nil {
		return nil, err
	}
	if !worldGenerationLock.TryLock() {
		return nil, ErrWorldGenerationBusy
	}
	defer worldGenerationLock.Unlock()
	if server := GetFactorioServer(); server.GetRunning() || server.IsStopping() {
		return nil, ErrFactorioMustBeStopped
	}

	temporaryDir, err := os.MkdirTemp("", "factorio-map-preview-*")
	if err != nil {
		return nil, fmt.Errorf("create preview workspace: %w", err)
	}
	defer os.RemoveAll(temporaryDir)

	settingsPath, err := writeMapGenSettings(temporaryDir, normalized)
	if err != nil {
		return nil, err
	}
	previewPath := filepath.Join(temporaryDir, "preview.png")
	args := worldGenerationArgs(normalized, settingsPath)
	args = append(args,
		"--generate-map-preview", previewPath,
		"--map-preview-size", strconv.Itoa(normalized.PreviewSize),
		"--map-preview-planet", normalized.Planet,
	)
	if err := runFactorioWorldCommand(2*time.Minute, args); err != nil {
		return nil, fmt.Errorf("generate %s preview: %w", normalized.Planet, err)
	}

	info, err := os.Stat(previewPath)
	if err != nil {
		return nil, fmt.Errorf("read generated preview metadata: %w", err)
	}
	if info.Size() <= 8 || info.Size() > maxGeneratedPreview {
		return nil, fmt.Errorf("generated preview has an invalid size of %d bytes", info.Size())
	}
	contents, err := os.ReadFile(previewPath)
	if err != nil {
		return nil, fmt.Errorf("read generated preview: %w", err)
	}
	if !strings.HasPrefix(string(contents[:8]), "\x89PNG\r\n\x1a\n") {
		return nil, errors.New("Factorio did not produce a PNG preview")
	}
	return contents, nil
}

func CreateWorld(request WorldGenerationRequest) (Save, error) {
	normalized, _, err := normalizeWorldGenerationRequest(request, false)
	if err != nil {
		return Save{}, err
	}
	if !worldGenerationLock.TryLock() {
		return Save{}, ErrWorldGenerationBusy
	}
	defer worldGenerationLock.Unlock()
	if server := GetFactorioServer(); server.GetRunning() || server.IsStopping() {
		return Save{}, ErrFactorioMustBeStopped
	}

	config := bootstrap.GetConfig()
	if err := os.MkdirAll(config.FactorioSavesDir, 0755); err != nil {
		return Save{}, fmt.Errorf("create saves directory: %w", err)
	}
	if !filepath.IsLocal(normalized.Name) {
		return Save{}, errors.New("save name must be a local filename")
	}
	finalPath := filepath.Join(config.FactorioSavesDir, normalized.Name)
	if _, err := os.Stat(finalPath); err == nil {
		return Save{}, ErrSaveAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return Save{}, fmt.Errorf("inspect save destination: %w", err)
	}

	temporaryDir, err := os.MkdirTemp(config.FactorioSavesDir, ".world-create-*")
	if err != nil {
		return Save{}, fmt.Errorf("create world workspace: %w", err)
	}
	defer os.RemoveAll(temporaryDir)

	settingsPath, err := writeMapGenSettings(temporaryDir, normalized)
	if err != nil {
		return Save{}, err
	}
	temporarySave := filepath.Join(temporaryDir, "world.zip")
	args := worldGenerationArgs(normalized, settingsPath)
	args = append(args, "--create", temporarySave)
	if err := runFactorioWorldCommand(5*time.Minute, args); err != nil {
		return Save{}, fmt.Errorf("create Factorio world: %w", err)
	}

	info, err := os.Stat(temporarySave)
	if err != nil {
		return Save{}, fmt.Errorf("inspect generated save: %w", err)
	}
	if !isUsableSave(info) {
		return Save{}, errors.New("Factorio produced an incomplete save archive")
	}
	if err := os.Rename(temporarySave, finalPath); err != nil {
		return Save{}, fmt.Errorf("activate generated save: %w", err)
	}
	info, err = os.Stat(finalPath)
	if err != nil {
		return Save{}, fmt.Errorf("inspect activated save: %w", err)
	}
	return Save{Name: info.Name(), LastMod: info.ModTime(), Size: info.Size()}, nil
}

func WorldGenerationBusy() bool {
	if !worldGenerationLock.TryLock() {
		return true
	}
	worldGenerationLock.Unlock()
	return false
}

func ValidateWorldGenerationRequest(request WorldGenerationRequest, preview bool) error {
	_, _, err := normalizeWorldGenerationRequest(request, preview)
	return err
}

func normalizeWorldGenerationRequest(request WorldGenerationRequest, preview bool) (WorldGenerationRequest, WorldGenerationOptions, error) {
	options, err := GetWorldGenerationOptions()
	if err != nil {
		return request, options, err
	}
	return normalizeWorldGenerationRequestWithOptions(request, preview, options)
}

func normalizeWorldGenerationRequestWithOptions(request WorldGenerationRequest, preview bool, options WorldGenerationOptions) (WorldGenerationRequest, WorldGenerationOptions, error) {
	request.Preset = strings.TrimSpace(strings.ToLower(request.Preset))
	if request.Preset == "" {
		request.Preset = "default"
	}
	validPreset := false
	for _, preset := range options.Presets {
		if preset.Name == request.Preset {
			validPreset = true
			break
		}
	}
	if !validPreset {
		return request, options, fmt.Errorf("unsupported map preset %q", request.Preset)
	}
	if request.Seed > uint64(^uint32(0)) {
		return request, options, errors.New("map seed must be between 0 and 4294967295")
	}
	if (request.Width != nil && *request.Width > maxWorldDimension) || (request.Height != nil && *request.Height > maxWorldDimension) {
		return request, options, fmt.Errorf("map dimensions must not exceed %d tiles", maxWorldDimension)
	}
	if request.StartingArea != nil && (*request.StartingArea < 0.1 || *request.StartingArea > 10) {
		return request, options, errors.New("starting area must be between 0.1 and 10")
	}

	request.Planet = strings.TrimSpace(strings.ToLower(request.Planet))
	if request.Planet == "" {
		request.Planet = "nauvis"
	}
	validPlanet := false
	for _, planet := range options.Planets {
		if planet.Name == request.Planet {
			validPlanet = true
			break
		}
	}
	if !validPlanet {
		return request, options, fmt.Errorf("planet %q is not available in the active game mode", request.Planet)
	}
	if preview {
		if request.PreviewSize == 0 {
			request.PreviewSize = defaultMapPreviewSize
		}
		if request.PreviewSize < 256 || request.PreviewSize > maxMapPreviewSize {
			return request, options, fmt.Errorf("preview size must be between 256 and %d pixels", maxMapPreviewSize)
		}
	}

	if !preview {
		request.Name = strings.TrimSpace(request.Name)
		if filepath.Ext(request.Name) == "" {
			request.Name += ".zip"
		}
		if err := ValidatePathElement(request.Name); err != nil || !strings.EqualFold(filepath.Ext(request.Name), ".zip") {
			return request, options, errors.New("save name must be a valid .zip filename")
		}
		request.Name = filepath.Base(request.Name)
	}

	definitions := make(map[string]WorldControlDefinition, len(options.Controls))
	for _, definition := range options.Controls {
		definitions[definition.Name] = definition
	}
	for name, settings := range request.Controls {
		definition, ok := definitions[name]
		if !ok || !worldIdentifier.MatchString(name) {
			return request, options, fmt.Errorf("map control %q is not available in the active game mode", name)
		}
		for field, value := range map[string]*float64{
			"frequency": settings.Frequency,
			"size":      settings.Size,
			"richness":  settings.Richness,
		} {
			if value == nil {
				continue
			}
			if *value < 0 || *value > 6 {
				return request, options, fmt.Errorf("%s for %s must be between 0 and 6", field, name)
			}
			if *value == 0 && !definition.CanDisable {
				return request, options, fmt.Errorf("%s cannot be disabled", definition.Label)
			}
			if field == "richness" && !definition.SupportsRichness {
				return request, options, fmt.Errorf("%s does not support richness", definition.Label)
			}
		}
	}
	return request, options, nil
}

func buildMapGenSettings(request WorldGenerationRequest, options WorldGenerationOptions) map[string]interface{} {
	settings := map[string]interface{}{
		"seed": request.Seed,
	}
	if request.Width != nil {
		settings["width"] = *request.Width
	}
	if request.Height != nil {
		settings["height"] = *request.Height
	}
	if request.StartingArea != nil {
		settings["starting_area"] = *request.StartingArea
	}
	if request.PeacefulMode != nil {
		settings["peaceful_mode"] = *request.PeacefulMode
	}
	if request.NoEnemiesMode != nil {
		settings["no_enemies_mode"] = *request.NoEnemiesMode
	}

	definitions := make(map[string]WorldControlDefinition, len(options.Controls))
	for _, definition := range options.Controls {
		definitions[definition.Name] = definition
	}
	autoplace := make(map[string]map[string]float64, len(request.Controls))
	for name, control := range request.Controls {
		definition := definitions[name]
		values := make(map[string]float64, 3)
		if control.Frequency != nil {
			values["frequency"] = *control.Frequency
		}
		if control.Size != nil {
			values["size"] = *control.Size
		}
		if definition.SupportsRichness && control.Richness != nil {
			values["richness"] = *control.Richness
		}
		if len(values) > 0 {
			autoplace[name] = values
		}
	}
	if len(autoplace) > 0 {
		settings["autoplace_controls"] = autoplace
	}
	return settings
}

func writeMapGenSettings(directory string, request WorldGenerationRequest) (string, error) {
	options, err := GetWorldGenerationOptions()
	if err != nil {
		return "", err
	}
	contents, err := json.MarshalIndent(buildMapGenSettings(request, options), "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode map generator settings: %w", err)
	}
	path := filepath.Join(directory, "map-gen-settings.json")
	if err := os.WriteFile(path, append(contents, '\n'), 0600); err != nil {
		return "", fmt.Errorf("write map generator settings: %w", err)
	}
	return path, nil
}

func worldGenerationArgs(request WorldGenerationRequest, settingsPath string) []string {
	config := bootstrap.GetConfig()
	return []string{
		"--mod-directory", config.FactorioModsDir,
		"--preset", request.Preset,
		"--map-gen-settings", settingsPath,
		"--map-gen-seed", strconv.FormatUint(request.Seed, 10),
	}
}

func runFactorioWorldCommand(timeout time.Duration, args []string) error {
	config := bootstrap.GetConfig()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var command *exec.Cmd
	if config.GlibcCustom == "true" {
		loaderArgs := append([]string{"--library-path", config.GlibcLibLoc, config.FactorioBinary, "--executable-path", config.FactorioBinary}, args...)
		command = exec.CommandContext(ctx, config.GlibcLocation, loaderArgs...)
	} else {
		command = exec.CommandContext(ctx, config.FactorioBinary, args...)
	}
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("Factorio timed out after %s", timeout)
	}
	if err == nil {
		return nil
	}
	message := redactWorldCommandOutput(output)
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}

func redactWorldCommandOutput(output []byte) string {
	const maxOutput = 24 * 1024
	if len(output) > maxOutput {
		output = output[len(output)-maxOutput:]
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for index := range lines {
		lines[index] = redactLogLine(lines[index])
	}
	return strings.Join(lines, "\n")
}
