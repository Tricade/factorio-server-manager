package factorio

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapSnapshotSettingsPersistAndValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "map-snapshot-settings.json")
	originalPath := mapSnapshotSettingsPath
	mapSnapshotSettingsPath = func() string { return path }
	t.Cleanup(func() { mapSnapshotSettingsPath = originalPath })

	settings, err := LoadMapSnapshotSettings()
	require.NoError(t, err)
	assert.Equal(t, defaultMapSnapshotInterval, settings.IntervalMinutes)

	settings, err = SetMapSnapshotSettings(MapSnapshotSettings{IntervalMinutes: 135})
	require.NoError(t, err)
	assert.Equal(t, 135, settings.IntervalMinutes)

	settings, err = LoadMapSnapshotSettings()
	require.NoError(t, err)
	assert.Equal(t, 135, settings.IntervalMinutes)
	_, err = SetMapSnapshotSettings(MapSnapshotSettings{IntervalMinutes: maximumMapSnapshotInterval + 1})
	assert.ErrorIs(t, err, ErrInvalidMapSnapshotSettings)
}

func TestFindMapSnapshotSourceSaveUsesFreshAutosaveOnlyWhileRunning(t *testing.T) {
	directory := t.TempDir()
	selectedPath := filepath.Join(directory, "world.zip")
	autosavePath := filepath.Join(directory, "_autosave2.zip")
	otherPath := filepath.Join(directory, "other.zip")
	for _, path := range []string{selectedPath, autosavePath, otherPath} {
		require.NoError(t, os.WriteFile(path, []byte("save"), 0644))
	}
	base := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(selectedPath, base, base))
	require.NoError(t, os.Chtimes(autosavePath, base.Add(time.Minute), base.Add(time.Minute)))
	require.NoError(t, os.Chtimes(otherPath, base.Add(2*time.Minute), base.Add(2*time.Minute)))

	stopped, err := findMapSnapshotSourceSave(directory, "world.zip", false)
	require.NoError(t, err)
	assert.Equal(t, "world.zip", stopped.Name)
	running, err := findMapSnapshotSourceSave(directory, "world.zip", true)
	require.NoError(t, err)
	assert.Equal(t, "_autosave2.zip", running.Name)
	latest, err := findMapSnapshotSourceSave(directory, "Load Latest", false)
	require.NoError(t, err)
	assert.Equal(t, "other.zip", latest.Name)
}

func TestMapSnapshotFactorioVersionRequiresChartAPI(t *testing.T) {
	assert.Error(t, validateMapSnapshotFactorioVersion("2.0.60"))
	assert.NoError(t, validateMapSnapshotFactorioVersion("2.0.61"))
	assert.NoError(t, validateMapSnapshotFactorioVersion("2.1.14"))
	assert.Error(t, validateMapSnapshotFactorioVersion("invalid"))
}

func TestPrepareMapSnapshotModsDoesNotChangeActiveModDirectory(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "isolated-mods")
	modList := []byte(`{"mods":[{"name":"base","enabled":true},{"name":"cargo-ships","enabled":true}]}`)
	require.NoError(t, os.WriteFile(filepath.Join(source, "mod-list.json"), modList, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(source, "mod-settings.dat"), []byte("settings"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(source, "cargo-ships_2.1.6.zip"), []byte("archive"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(source, "cargo-ships-extra_1.0.0.zip"), []byte("unrelated"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(source, "disabled-overhaul_9.9.9.zip"), []byte("large disabled archive"), 0644))

	require.NoError(t, prepareMapSnapshotMods(source, destination, "2.1.14"))
	activeList, err := os.ReadFile(filepath.Join(source, "mod-list.json"))
	require.NoError(t, err)
	assert.Equal(t, modList, activeList)
	assert.NoDirExists(t, filepath.Join(source, mapSnapshotExporterName+"_0.1.0"))

	isolatedList, err := os.ReadFile(filepath.Join(destination, "mod-list.json"))
	require.NoError(t, err)
	assert.Contains(t, string(isolatedList), mapSnapshotExporterName)
	assert.DirExists(t, filepath.Join(destination, mapSnapshotExporterName+"_0.1.0"))
	assert.FileExists(t, filepath.Join(destination, mapSnapshotExporterName+"_0.1.0", "control.lua"))
	assert.FileExists(t, filepath.Join(destination, "cargo-ships_2.1.6.zip"))
	assert.NoFileExists(t, filepath.Join(destination, "cargo-ships-extra_1.0.0.zip"))
	assert.NoFileExists(t, filepath.Join(destination, "disabled-overhaul_9.9.9.zip"))
	info, err := os.ReadFile(filepath.Join(destination, mapSnapshotExporterName+"_0.1.0", "info.json"))
	require.NoError(t, err)
	assert.Contains(t, string(info), `"factorio_version": "2.1"`)
}

func TestRenderMapSnapshotSurfaceDecodesRGB565(t *testing.T) {
	directory := t.TempDir()
	pixels := make([]byte, mapSnapshotChunkPixels*mapSnapshotChunkPixels*2)
	for index := 0; index < len(pixels); index += 2 {
		binary.LittleEndian.PutUint16(pixels[index:index+2], 0xf800)
	}
	chunk := mapSnapshotExporterChunk{X: -1, Y: 2, Data: encodeMapSnapshotTestChunk(t, pixels)}
	line, err := json.Marshal(chunk)
	require.NoError(t, err)
	source := filepath.Join(directory, "surface-1.jsonl")
	require.NoError(t, os.WriteFile(source, append(line, '\n'), 0600))
	destination := filepath.Join(directory, "surface-1.png")

	width, height, err := renderMapSnapshotSurface(mapSnapshotExporterSurface{
		Index: 1, Name: "nauvis", ChunkCount: 1,
		MinX: -1, MaxX: -1, MinY: 2, MaxY: 2,
	}, source, destination)
	require.NoError(t, err)
	assert.Equal(t, 32, width)
	assert.Equal(t, 32, height)

	file, err := os.Open(destination)
	require.NoError(t, err)
	defer file.Close()
	image, err := png.Decode(file)
	require.NoError(t, err)
	red, green, blue, alpha := image.At(10, 10).RGBA()
	assert.Equal(t, uint32(0xffff), red)
	assert.Equal(t, uint32(0), green)
	assert.Equal(t, uint32(0), blue)
	assert.Equal(t, uint32(0xffff), alpha)
}

func TestRenderMapSnapshotSurfaceUsesFittedViewBounds(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "surface-2.jsonl")
	lines := make([][]byte, 0, 3)
	for x, colour := range []uint16{0xf800, 0x07e0, 0x001f} {
		pixels := make([]byte, mapSnapshotChunkPixels*mapSnapshotChunkPixels*2)
		for index := 0; index < len(pixels); index += 2 {
			binary.LittleEndian.PutUint16(pixels[index:index+2], colour)
		}
		line, err := json.Marshal(mapSnapshotExporterChunk{X: x, Y: 0, Data: encodeMapSnapshotTestChunk(t, pixels)})
		require.NoError(t, err)
		lines = append(lines, line)
	}
	require.NoError(t, os.WriteFile(source, append(bytes.Join(lines, []byte{'\n'}), '\n'), 0600))
	destination := filepath.Join(directory, "surface-2.png")
	viewMinX, viewMinY, viewMaxX, viewMaxY := 32, 0, 63, 31

	width, height, err := renderMapSnapshotSurface(mapSnapshotExporterSurface{
		Index: 2, Name: "Supply Express", SurfaceName: "platform-1", Kind: "platform", ChunkCount: 3,
		MinX: 0, MaxX: 2, MinY: 0, MaxY: 0,
		ViewMinTileX: &viewMinX, ViewMinTileY: &viewMinY, ViewMaxTileX: &viewMaxX, ViewMaxTileY: &viewMaxY,
	}, source, destination)
	require.NoError(t, err)
	assert.Equal(t, 32, width)
	assert.Equal(t, 32, height)

	file, err := os.Open(destination)
	require.NoError(t, err)
	defer file.Close()
	image, err := png.Decode(file)
	require.NoError(t, err)
	red, green, blue, alpha := image.At(10, 10).RGBA()
	assert.Equal(t, uint32(0), red)
	assert.Equal(t, uint32(0xffff), green)
	assert.Equal(t, uint32(0), blue)
	assert.Equal(t, uint32(0xffff), alpha)
}

func TestMapSnapshotRenderBoundsRejectsIncompleteOrOutOfRangeViews(t *testing.T) {
	viewMinX := 0
	_, _, _, _, err := mapSnapshotRenderTileBounds(mapSnapshotExporterSurface{
		MinX: 0, MinY: 0, MaxX: 2, MaxY: 2, ViewMinTileX: &viewMinX,
	})
	assert.Error(t, err)

	viewMinY, viewMaxX, viewMaxY := 0, 96, 95
	_, _, _, _, err = mapSnapshotRenderTileBounds(mapSnapshotExporterSurface{
		MinX: 0, MinY: 0, MaxX: 2, MaxY: 2,
		ViewMinTileX: &viewMinX, ViewMinTileY: &viewMinY, ViewMaxTileX: &viewMaxX, ViewMaxTileY: &viewMaxY,
	})
	assert.Error(t, err)
}

func TestMapSnapshotManifestAcceptsFactorioEmptyTableEncoding(t *testing.T) {
	var manifest mapSnapshotExporterManifest
	require.NoError(t, json.Unmarshal([]byte(`{"schema_version":1,"game_tick":1,"game_version":"2.1.14","force":"player","surfaces":{}}`), &manifest))
	assert.Empty(t, manifest.Surfaces)
}

func TestMapSnapshotFileReadsAreBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.log")
	require.NoError(t, os.WriteFile(path, []byte("0123456789"), 0600))

	_, err := readMapSnapshotFile(path, 5)
	assert.Error(t, err)
	tail, err := readMapSnapshotFileTail(path, 5)
	require.NoError(t, err)
	assert.Equal(t, "56789", string(tail))
}

func TestPersistedMapSnapshotIsProfileScopedAndReadable(t *testing.T) {
	root := t.TempDir()
	originalRoot := mapSnapshotRootPath
	originalNow := mapSnapshotNow
	mapSnapshotRootPath = func() string { return root }
	generatedAt := time.Date(2026, 8, 22, 13, 30, 0, 0, time.UTC)
	mapSnapshotNow = func() time.Time { return generatedAt }
	t.Cleanup(func() {
		mapSnapshotRootPath = originalRoot
		mapSnapshotNow = originalNow
	})

	exportDirectory := t.TempDir()
	pixels := bytes.Repeat([]byte{0xe0, 0x07}, mapSnapshotChunkPixels*mapSnapshotChunkPixels)
	chunkJSON, err := json.Marshal(mapSnapshotExporterChunk{X: 0, Y: 0, Data: encodeMapSnapshotTestChunk(t, pixels)})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(exportDirectory, "surface-1.jsonl"), append(chunkJSON, '\n'), 0600))
	profile := Profile{ID: "0123456789abcdef"}
	source := Save{Name: "world.zip", LastMod: generatedAt.Add(-time.Minute)}
	manifest := mapSnapshotExporterManifest{
		SchemaVersion: 1, GameTick: 123, GameVersion: "2.1.14", Force: "player",
		Surfaces: []mapSnapshotExporterSurface{{Index: 1, Name: "Supply Express", SurfaceName: "platform-1", Kind: "platform", ChunkCount: 1, MinX: 0, MinY: 0, MaxX: 0, MaxY: 0, File: "surface-1.jsonl"}},
	}
	require.NoError(t, persistRenderedMapSnapshot(profile, source, manifest, exportDirectory))

	snapshot, err := loadMapSnapshot(profile.ID)
	require.NoError(t, err)
	assert.Equal(t, generatedAt, snapshot.GeneratedAt)
	require.Len(t, snapshot.Surfaces, 1)
	assert.Equal(t, "Supply Express", snapshot.Surfaces[0].Name)
	assert.Equal(t, "platform-1", snapshot.Surfaces[0].SurfaceName)
	assert.Equal(t, "platform", snapshot.Surfaces[0].Kind)
	assert.Equal(t, "surface-1.png", snapshot.Surfaces[0].File)
	assert.FileExists(t, filepath.Join(root, profile.ID, "surface-1.png"))
}

func TestMapSnapshotSourceCopyRemainsAValidUnchangedZip(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "world.zip")
	writeMapSnapshotTestSave(t, source)
	before, err := os.ReadFile(source)
	require.NoError(t, err)
	destination := filepath.Join(directory, "copy.zip")
	require.NoError(t, copyCheckpointAtomically(source, destination))
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, before, mustReadMapSnapshotTestFile(t, destination))
}

func encodeMapSnapshotTestChunk(t *testing.T, pixels []byte) string {
	t.Helper()
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err := writer.Write(pixels)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return base64.StdEncoding.EncodeToString(compressed.Bytes())
}

func writeMapSnapshotTestSave(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	require.NoError(t, err)
	writer := zip.NewWriter(file)
	entry, err := writer.Create("world/level.dat")
	require.NoError(t, err)
	_, err = strings.NewReader("level").WriteTo(entry)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
}

func mustReadMapSnapshotTestFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return contents
}
