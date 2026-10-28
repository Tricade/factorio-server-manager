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
		return nil, errors.New("the Factorio did not produce a PNG preview")
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
		return Save{}, errors.New("the Factorio produced an incomplete save archive")
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
		return fmt.Errorf("the Factorio timed out after %s", timeout)
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
