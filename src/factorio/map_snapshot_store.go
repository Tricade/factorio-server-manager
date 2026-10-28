package factorio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func persistRenderedMapSnapshot(profile Profile, source Save, manifest mapSnapshotExporterManifest, exportDirectory string) error {
	root := mapSnapshotRootPath()
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, ".map-snapshot-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	surfaces := make([]MapSnapshotSurface, 0, len(manifest.Surfaces))
	for _, exportedSurface := range manifest.Surfaces {
		filename := "surface-" + strconv.FormatUint(uint64(exportedSurface.Index), 10) + ".png"
		geometry, err := calculateMapSnapshotRenderGeometry(exportedSurface)
		if err != nil {
			return fmt.Errorf("calculate surface %q geometry: %w", exportedSurface.Name, err)
		}
		width, height, err := renderMapSnapshotSurface(exportedSurface, filepath.Join(exportDirectory, exportedSurface.File), filepath.Join(stage, filename))
		if err != nil {
			return fmt.Errorf("render surface %q: %w", exportedSurface.Name, err)
		}
		entityFilename := "surface-" + strconv.FormatUint(uint64(exportedSurface.Index), 10) + "-entities.jsonl"
		if err := writeMapSnapshotEntityFile(filepath.Join(exportDirectory, exportedSurface.EntityFile), filepath.Join(stage, entityFilename), exportedSurface.EntityCount); err != nil {
			return fmt.Errorf("persist entity details for surface %q: %w", exportedSurface.Name, err)
		}
		surfaces = append(surfaces, MapSnapshotSurface{
			ID:                  strconv.FormatUint(uint64(exportedSurface.Index), 10),
			Index:               exportedSurface.Index,
			Name:                exportedSurface.Name,
			SurfaceName:         exportedSurface.SurfaceName,
			Kind:                exportedSurface.Kind,
			ChunkCount:          exportedSurface.ChunkCount,
			Width:               width,
			Height:              height,
			ViewBoundsAvailable: true,
			ViewMinTileX:        geometry.MinTileX,
			ViewMinTileY:        geometry.MinTileY,
			ViewMaxTileX:        geometry.MaxTileX,
			ViewMaxTileY:        geometry.MaxTileY,
			PixelsPerTile:       geometry.PixelsPerTile,
			EntitiesAvailable:   true,
			EntityCount:         exportedSurface.EntityCount,
			EntityTotalCount:    exportedSurface.EntityTotalCount,
			EntityTruncated:     exportedSurface.EntityTruncated,
			File:                filename,
			EntityFile:          entityFilename,
		})
	}
	sort.SliceStable(surfaces, func(left, right int) bool { return surfaces[left].Index < surfaces[right].Index })
	players := make([]MapSnapshotPlayer, 0, len(manifest.Players))
	for _, exportedPlayer := range manifest.Players {
		players = append(players, MapSnapshotPlayer{
			Name:              exportedPlayer.Name,
			OnlineTimeTicks:   exportedPlayer.OnlineTime,
			OnlineTimeSeconds: exportedPlayer.OnlineTime / 60,
		})
	}
	sort.SliceStable(players, func(left, right int) bool {
		if players[left].OnlineTimeTicks != players[right].OnlineTimeTicks {
			return players[left].OnlineTimeTicks > players[right].OnlineTimeTicks
		}
		return strings.ToLower(players[left].Name) < strings.ToLower(players[right].Name)
	})
	for index := range players {
		players[index].Rank = index + 1
		if index > 0 && players[index].OnlineTimeTicks == players[index-1].OnlineTimeTicks {
			players[index].Rank = players[index-1].Rank
		}
	}
	snapshot := MapSnapshot{
		SchemaVersion:   mapSnapshotSchemaVersion,
		ProfileID:       profile.ID,
		SaveName:        source.Name,
		SaveModifiedAt:  source.LastMod.UTC(),
		GeneratedAt:     mapSnapshotNow().UTC(),
		FactorioVersion: manifest.GameVersion,
		GameTick:        manifest.GameTick,
		Force:           manifest.Force,
		Players:         players,
		Surfaces:        surfaces,
	}
	if err := writeMapSnapshotJSONAtomically(filepath.Join(stage, mapSnapshotMetadataFileName), snapshot, 0600); err != nil {
		return err
	}
	return activateMapSnapshotDirectory(profile.ID, stage)
}

func activateMapSnapshotDirectory(profileID, stage string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	mapSnapshotStoreGate.Lock()
	defer mapSnapshotStoreGate.Unlock()
	destination := filepath.Join(mapSnapshotRootPath(), profileID)
	backup := destination + ".previous"
	destinationInfo, destinationErr := os.Lstat(destination)
	backupInfo, backupErr := os.Lstat(backup)
	if destinationErr != nil && !errors.Is(destinationErr, os.ErrNotExist) {
		return fmt.Errorf("inspect current map snapshot: %w", destinationErr)
	}
	if backupErr != nil && !errors.Is(backupErr, os.ErrNotExist) {
		return fmt.Errorf("inspect previous map snapshot: %w", backupErr)
	}
	if backupErr == nil && (backupInfo.Mode()&os.ModeSymlink != 0 || !backupInfo.IsDir()) {
		return errors.New("previous map snapshot recovery entry is not a directory")
	}
	if destinationErr == nil && (destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir()) {
		return errors.New("current map snapshot entry is not a directory")
	}
	if errors.Is(destinationErr, os.ErrNotExist) && backupErr == nil {
		if err := mapSnapshotRename(backup, destination); err != nil {
			return fmt.Errorf("recover previous map snapshot: %w", err)
		}
		destinationInfo, destinationErr = os.Lstat(destination)
		backupErr = os.ErrNotExist
	}
	if destinationErr == nil && backupErr == nil {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove stale previous map snapshot: %w", err)
		}
	}
	hasPrevious := false
	if destinationErr == nil && destinationInfo.IsDir() {
		if err := mapSnapshotRename(destination, backup); err != nil {
			return fmt.Errorf("back up previous map snapshot: %w", err)
		}
		hasPrevious = true
	}
	if err := mapSnapshotRename(stage, destination); err != nil {
		if hasPrevious {
			if restoreErr := mapSnapshotRename(backup, destination); restoreErr != nil {
				return fmt.Errorf("activate map snapshot: %w (restore failed: %v; previous snapshot remains at %s)", err, restoreErr, backup)
			}
		}
		return fmt.Errorf("activate map snapshot: %w", err)
	}
	if hasPrevious {
		if err := os.RemoveAll(backup); err != nil {
			log.Printf("Map snapshot for profile %s is active but previous snapshot cleanup failed: %v", profileID, err)
		}
	}
	return nil
}

func loadMapSnapshot(profileID string) (*MapSnapshot, error) {
	if err := validateProfileID(profileID); err != nil {
		return nil, err
	}
	mapSnapshotStoreGate.RLock()
	defer mapSnapshotStoreGate.RUnlock()
	contents, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profileID, mapSnapshotMetadataFileName), maximumMapSnapshotManifest)
	if err != nil {
		return nil, err
	}
	var snapshot MapSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil {
		return nil, fmt.Errorf("decode map snapshot: %w", err)
	}
	if snapshot.SchemaVersion != mapSnapshotSchemaVersion || snapshot.ProfileID != profileID {
		return nil, errors.New("map snapshot metadata is invalid")
	}
	for index := range snapshot.Surfaces {
		surface := &snapshot.Surfaces[index]
		if err := populateMapSnapshotSurfaceFiles(surface); err != nil {
			return nil, err
		}
	}
	return &snapshot, nil
}

func populateMapSnapshotSurfaceFiles(surface *MapSnapshotSurface) error {
	if surface.ID != strconv.FormatUint(uint64(surface.Index), 10) || surface.ChunkCount < 1 || surface.ChunkCount > maximumMapSnapshotChunks || surface.Width < 1 || surface.Width > maximumMapSnapshotDimension || surface.Height < 1 || surface.Height > maximumMapSnapshotDimension {
		return errors.New("map snapshot surface metadata is invalid")
	}
	surface.File = "surface-" + surface.ID + ".png"
	if !surface.ViewBoundsAvailable {
		if surface.ViewMinTileX != 0 || surface.ViewMinTileY != 0 || surface.ViewMaxTileX != 0 || surface.ViewMaxTileY != 0 || surface.PixelsPerTile != 0 || surface.EntitiesAvailable {
			return errors.New("map snapshot view metadata is invalid")
		}
	} else if err := validateStoredMapSnapshotGeometry(*surface); err != nil {
		return err
	}
	if !surface.EntitiesAvailable {
		if surface.EntityCount != 0 || surface.EntityTotalCount != 0 || surface.EntityTruncated {
			return errors.New("map snapshot entity metadata is invalid")
		}
		surface.EntityFile = ""
		return nil
	}
	if surface.EntityCount < 0 || surface.EntityCount > maximumMapSnapshotEntityCount || surface.EntityTotalCount < surface.EntityCount || surface.EntityTotalCount > maximumMapSnapshotEntityTotal || surface.EntityTruncated != (surface.EntityTotalCount > surface.EntityCount) {
		return errors.New("map snapshot entity metadata is invalid")
	}
	surface.EntityFile = "surface-" + surface.ID + "-entities.jsonl"
	return nil
}

func validateStoredMapSnapshotGeometry(surface MapSnapshotSurface) error {
	coordinates := []int{surface.ViewMinTileX, surface.ViewMinTileY, surface.ViewMaxTileX, surface.ViewMaxTileY}
	for _, coordinate := range coordinates {
		if coordinate < -maximumMapSnapshotCoordinate || coordinate > maximumMapSnapshotCoordinate {
			return errors.New("map snapshot view metadata is invalid")
		}
	}
	if surface.ViewMinTileX > surface.ViewMaxTileX || surface.ViewMinTileY > surface.ViewMaxTileY {
		return errors.New("map snapshot view metadata is invalid")
	}
	sourceWidth := surface.ViewMaxTileX - surface.ViewMinTileX + 1
	sourceHeight := surface.ViewMaxTileY - surface.ViewMinTileY + 1
	expectedScale := 1.0
	if sourceWidth > maximumMapSnapshotDimension || sourceHeight > maximumMapSnapshotDimension {
		expectedScale = math.Min(float64(maximumMapSnapshotDimension)/float64(sourceWidth), float64(maximumMapSnapshotDimension)/float64(sourceHeight))
	}
	expectedWidth := max(1, int(float64(sourceWidth)*expectedScale))
	expectedHeight := max(1, int(float64(sourceHeight)*expectedScale))
	if surface.Width != expectedWidth || surface.Height != expectedHeight || math.IsNaN(surface.PixelsPerTile) || math.IsInf(surface.PixelsPerTile, 0) || math.Abs(surface.PixelsPerTile-expectedScale) > 1e-12 {
		return errors.New("map snapshot view metadata is invalid")
	}
	return nil
}

func ReadMapSnapshotImage(surfaceID string) ([]byte, time.Time, error) {
	if _, err := strconv.ParseUint(surfaceID, 10, 32); err != nil {
		return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
	}
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return nil, time.Time{}, err
	}
	mapSnapshotStoreGate.RLock()
	defer mapSnapshotStoreGate.RUnlock()
	contents, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, mapSnapshotMetadataFileName), maximumMapSnapshotManifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	var snapshot MapSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil || snapshot.SchemaVersion != mapSnapshotSchemaVersion || snapshot.ProfileID != profile.ID {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	for index := range snapshot.Surfaces {
		surface := &snapshot.Surfaces[index]
		if surface.ID != surfaceID {
			continue
		}
		if err := populateMapSnapshotSurfaceFiles(surface); err != nil {
			return nil, time.Time{}, ErrMapSnapshotNotFound
		}
		imageBytes, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, surface.File), maximumMapSnapshotImageBytes)
		if err != nil {
			return nil, time.Time{}, err
		}
		return imageBytes, snapshot.GeneratedAt, nil
	}
	return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
}

// ReadMapSnapshotEntities returns a bounded, validated NDJSON dataset for one
// surface. Each line is a MapSnapshotEntity from the isolated save exporter.
func ReadMapSnapshotEntities(surfaceID string) ([]byte, time.Time, error) {
	if _, err := strconv.ParseUint(surfaceID, 10, 32); err != nil {
		return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
	}
	profile, err := activeMapSnapshotProfile()
	if err != nil {
		return nil, time.Time{}, err
	}
	mapSnapshotStoreGate.RLock()
	defer mapSnapshotStoreGate.RUnlock()
	contents, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, mapSnapshotMetadataFileName), maximumMapSnapshotManifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	var snapshot MapSnapshot
	if err := json.Unmarshal(contents, &snapshot); err != nil || snapshot.SchemaVersion != mapSnapshotSchemaVersion || snapshot.ProfileID != profile.ID {
		return nil, time.Time{}, ErrMapSnapshotNotFound
	}
	for index := range snapshot.Surfaces {
		surface := &snapshot.Surfaces[index]
		if surface.ID != surfaceID {
			continue
		}
		if err := populateMapSnapshotSurfaceFiles(surface); err != nil {
			return nil, time.Time{}, ErrMapSnapshotNotFound
		}
		if !surface.EntitiesAvailable {
			return nil, time.Time{}, ErrMapSnapshotDetailsNotFound
		}
		entityBytes, err := readMapSnapshotFile(filepath.Join(mapSnapshotRootPath(), profile.ID, surface.EntityFile), maximumMapSnapshotEntityBytes)
		if err != nil {
			return nil, time.Time{}, err
		}
		if err := processMapSnapshotEntities(bytes.NewReader(entityBytes), surface.EntityCount, nil); err != nil {
			return nil, time.Time{}, fmt.Errorf("validate stored map snapshot entities: %w", err)
		}
		return entityBytes, snapshot.GeneratedAt, nil
	}
	return nil, time.Time{}, ErrMapSnapshotSurfaceNotFound
}

func deleteProfileMapSnapshot(profileID string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	mapSnapshotStoreGate.Lock()
	defer mapSnapshotStoreGate.Unlock()
	return os.RemoveAll(filepath.Join(mapSnapshotRootPath(), profileID))
}

func writeMapSnapshotJSONAtomically(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".map-snapshot-json-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}
	backup := path + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
