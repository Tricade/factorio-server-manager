package factorio

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	mapSnapshotSchemaVersion      = 1
	mapSnapshotSettingsFileName   = "map-snapshot-settings.json"
	mapSnapshotMetadataFileName   = "snapshot.json"
	mapSnapshotExporterName       = "fsm-map-exporter"
	mapSnapshotExporterVersion    = "0.2.0"
	defaultMapSnapshotInterval    = 60
	maximumMapSnapshotInterval    = 7 * 24 * 60
	maximumMapSnapshotDimension   = 4096
	maximumMapSnapshotChunks      = 250000
	maximumMapSnapshotExportBytes = 512 * 1024 * 1024
	maximumMapSnapshotManifest    = 4 * 1024 * 1024
	maximumMapSnapshotErrorLog    = 64 * 1024
	maximumMapSnapshotImageBytes  = 128 * 1024 * 1024
	maximumMapSnapshotEntityCount = 100000
	maximumMapSnapshotEntityTotal = 1_000_000_000
	maximumMapSnapshotEntityBytes = 32 * 1024 * 1024
	maximumMapSnapshotEntityLine  = 16 * 1024
	maximumMapSnapshotCoordinate  = 10_000_000
	mapSnapshotChunkPixels        = 32
)

var (
	ErrMapSnapshotBusy            = errors.New("a map snapshot is already being generated")
	ErrMapSnapshotsDisabled       = errors.New("factory map snapshots are disabled")
	ErrMapSnapshotNotFound        = errors.New("map snapshot not found")
	ErrMapSnapshotSurfaceNotFound = errors.New("map snapshot surface not found")
	ErrMapSnapshotDetailsNotFound = errors.New("map snapshot entity details not found")
	ErrInvalidMapSnapshotSettings = errors.New("invalid map snapshot settings")
)

type MapSnapshotSettings struct {
	Enabled                    bool `json:"enabled"`
	IntervalMinutes            int  `json:"interval_minutes"`
	AutomaticOnlyWhenNoPlayers bool `json:"automatic_only_when_no_players"`
	IncludeSpacePlatforms      bool `json:"include_space_platforms"`
}

type MapSnapshotSurface struct {
	ID                  string  `json:"id"`
	Index               uint32  `json:"index"`
	Name                string  `json:"name"`
	SurfaceName         string  `json:"surface_name,omitempty"`
	Kind                string  `json:"kind,omitempty"`
	ChunkCount          int     `json:"chunk_count"`
	Width               int     `json:"width"`
	Height              int     `json:"height"`
	ViewBoundsAvailable bool    `json:"view_bounds_available"`
	ViewMinTileX        int     `json:"view_min_tile_x"`
	ViewMinTileY        int     `json:"view_min_tile_y"`
	ViewMaxTileX        int     `json:"view_max_tile_x"`
	ViewMaxTileY        int     `json:"view_max_tile_y"`
	PixelsPerTile       float64 `json:"pixels_per_tile"`
	EntitiesAvailable   bool    `json:"entities_available"`
	EntityCount         int     `json:"entity_count"`
	EntityTotalCount    int     `json:"entity_total_count"`
	EntityTruncated     bool    `json:"entity_truncated"`
	File                string  `json:"-"`
	EntityFile          string  `json:"-"`
}

// MapSnapshotEntity describes the footprint of a player-force building in the
// isolated snapshot save. It deliberately contains geometry and prototype
// identifiers only; chart images do not contain client sprites.
type MapSnapshotEntity struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Direction   uint8                  `json:"direction"`
	BoundingBox MapSnapshotBoundingBox `json:"bounding_box"`
}

type MapSnapshotBoundingBox struct {
	LeftTop     MapSnapshotPosition `json:"left_top"`
	RightBottom MapSnapshotPosition `json:"right_bottom"`
}

type MapSnapshotPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// MapSnapshotPlayer is read only from the isolated snapshot save. OnlineTime
// is Factorio's persisted LuaPlayer.online_time value in game ticks; no Lua is
// executed against the live server to collect it.
type MapSnapshotPlayer struct {
	Name              string `json:"name"`
	OnlineTimeTicks   uint64 `json:"online_time_ticks"`
	OnlineTimeSeconds uint64 `json:"online_time_seconds"`
	Rank              int    `json:"rank"`
}

type MapSnapshot struct {
	SchemaVersion   int                  `json:"schema_version"`
	ProfileID       string               `json:"profile_id"`
	SaveName        string               `json:"save_name"`
	SaveModifiedAt  time.Time            `json:"save_modified_at"`
	GeneratedAt     time.Time            `json:"generated_at"`
	FactorioVersion string               `json:"factorio_version"`
	GameTick        uint64               `json:"game_tick"`
	Force           string               `json:"force"`
	Players         []MapSnapshotPlayer  `json:"players"`
	Surfaces        []MapSnapshotSurface `json:"surfaces"`
}

type MapSnapshotState struct {
	Settings       MapSnapshotSettings `json:"settings"`
	Running        bool                `json:"running"`
	RunningProfile string              `json:"running_profile,omitempty"`
	LastAttemptAt  *time.Time          `json:"last_attempt_at,omitempty"`
	LastError      string              `json:"last_error,omitempty"`
	Snapshot       *MapSnapshot        `json:"snapshot,omitempty"`
}

type mapSnapshotExporterManifest struct {
	SchemaVersion int                          `json:"schema_version"`
	GameTick      uint64                       `json:"game_tick"`
	GameVersion   string                       `json:"game_version"`
	Force         string                       `json:"force"`
	Players       []mapSnapshotExporterPlayer  `json:"players"`
	Surfaces      []mapSnapshotExporterSurface `json:"surfaces"`
}

func (manifest *mapSnapshotExporterManifest) UnmarshalJSON(contents []byte) error {
	var raw struct {
		SchemaVersion int             `json:"schema_version"`
		GameTick      uint64          `json:"game_tick"`
		GameVersion   string          `json:"game_version"`
		Force         string          `json:"force"`
		Players       json.RawMessage `json:"players"`
		Surfaces      json.RawMessage `json:"surfaces"`
	}
	if err := json.Unmarshal(contents, &raw); err != nil {
		return err
	}
	manifest.SchemaVersion = raw.SchemaVersion
	manifest.GameTick = raw.GameTick
	manifest.GameVersion = raw.GameVersion
	manifest.Force = raw.Force
	trimmedPlayers := bytes.TrimSpace(raw.Players)
	if bytes.Equal(trimmedPlayers, []byte("{}")) || bytes.Equal(trimmedPlayers, []byte("null")) || len(trimmedPlayers) == 0 {
		manifest.Players = []mapSnapshotExporterPlayer{}
	} else if err := json.Unmarshal(trimmedPlayers, &manifest.Players); err != nil {
		return err
	}
	trimmedSurfaces := bytes.TrimSpace(raw.Surfaces)
	if bytes.Equal(trimmedSurfaces, []byte("{}")) || bytes.Equal(trimmedSurfaces, []byte("null")) || len(trimmedSurfaces) == 0 {
		manifest.Surfaces = []mapSnapshotExporterSurface{}
		return nil
	}
	return json.Unmarshal(trimmedSurfaces, &manifest.Surfaces)
}

type mapSnapshotExporterPlayer struct {
	Name       string `json:"name"`
	OnlineTime uint64 `json:"online_time"`
}

type mapSnapshotExporterSurface struct {
	Index            uint32 `json:"index"`
	Name             string `json:"name"`
	SurfaceName      string `json:"surface_name,omitempty"`
	Kind             string `json:"kind,omitempty"`
	ChunkCount       int    `json:"chunk_count"`
	MinX             int    `json:"min_x"`
	MinY             int    `json:"min_y"`
	MaxX             int    `json:"max_x"`
	MaxY             int    `json:"max_y"`
	ViewMinTileX     *int   `json:"view_min_tile_x,omitempty"`
	ViewMinTileY     *int   `json:"view_min_tile_y,omitempty"`
	ViewMaxTileX     *int   `json:"view_max_tile_x,omitempty"`
	ViewMaxTileY     *int   `json:"view_max_tile_y,omitempty"`
	File             string `json:"file"`
	EntityFile       string `json:"entity_file"`
	EntityCount      int    `json:"entity_count"`
	EntityTotalCount int    `json:"entity_total_count"`
	EntityTruncated  bool   `json:"entity_truncated"`
}

type mapSnapshotExporterChunk struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Data string `json:"data"`
}

type mapSnapshotRuntimeState struct {
	Running            bool
	RunningProfile     string
	LastAttemptProfile string
	LastAttemptAt      *time.Time
	LastError          string
}

//go:embed map_snapshot_exporter/info.json map_snapshot_exporter/control.lua
var mapSnapshotExporterFiles embed.FS

var mapSnapshotOperationMutex sync.Mutex
var mapSnapshotSettingsMutex sync.Mutex
var mapSnapshotRuntimeMutex sync.RWMutex
var mapSnapshotStoreGate sync.RWMutex
var mapSnapshotSchedulerOnce sync.Once
var mapSnapshotSchedulerWake = make(chan struct{}, 1)
var mapSnapshotNow = time.Now
var mapSnapshotCommandTimeout = 10 * time.Minute
var mapSnapshotRootPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), "map-snapshots")
}
var mapSnapshotSettingsPath = func() string {
	return filepath.Join(filepath.Dir(bootstrap.GetConfig().ConfFile), mapSnapshotSettingsFileName)
}
var mapSnapshotRunFactorio = func(timeout time.Duration, args []string) error {
	return runFactorioWorldCommand(timeout, args)
}
var mapSnapshotRename = os.Rename
var mapSnapshotServerRunning = func() bool {
	return GetFactorioServer().GetRunning()
}
var mapSnapshotServerBusy = func() bool {
	return GetFactorioServer().IsBusy()
}
var mapSnapshotConnectedPlayerCount = connectedPlayerCount
