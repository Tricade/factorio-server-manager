package factorio

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

func writeMapSnapshotEntityFile(sourcePath, destinationPath string, expectedCount int) error {
	if expectedCount < 0 || expectedCount > maximumMapSnapshotEntityCount {
		return errors.New("entity count exceeds the limit")
	}
	source, info, err := openRegularMapSnapshotFile(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	if info.Size() > maximumMapSnapshotEntityBytes {
		return errors.New("entity dataset exceeds the size limit")
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	processErr := processMapSnapshotEntities(io.LimitReader(source, maximumMapSnapshotEntityBytes+1), expectedCount, destination)
	if processErr == nil {
		processErr = destination.Sync()
	}
	closeErr := destination.Close()
	if processErr != nil {
		_ = os.Remove(destinationPath)
		return processErr
	}
	if closeErr != nil {
		_ = os.Remove(destinationPath)
		return closeErr
	}
	return nil
}

func processMapSnapshotEntities(reader io.Reader, expectedCount int, destination io.Writer) error {
	if expectedCount < 0 || expectedCount > maximumMapSnapshotEntityCount {
		return errors.New("entity count exceeds the limit")
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maximumMapSnapshotEntityLine)
	count := 0
	var written int64
	for scanner.Scan() {
		if count >= maximumMapSnapshotEntityCount {
			return errors.New("entity dataset contains too many records")
		}
		entity, err := decodeMapSnapshotEntity(scanner.Bytes())
		if err != nil {
			return fmt.Errorf("decode entity line %d: %w", count+1, err)
		}
		count++
		if destination == nil {
			continue
		}
		line, err := json.Marshal(entity)
		if err != nil {
			return err
		}
		line = append(line, '\n')
		if len(line) > maximumMapSnapshotEntityLine || written+int64(len(line)) > maximumMapSnapshotEntityBytes {
			return errors.New("canonical entity dataset exceeds the size limit")
		}
		amount, err := destination.Write(line)
		if err != nil {
			return err
		}
		if amount != len(line) {
			return io.ErrShortWrite
		}
		written += int64(amount)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if count != expectedCount {
		return fmt.Errorf("expected %d entities, read %d", expectedCount, count)
	}
	return nil
}

func decodeMapSnapshotEntity(line []byte) (MapSnapshotEntity, error) {
	var entity MapSnapshotEntity
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entity); err != nil {
		return MapSnapshotEntity{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("entity line contains more than one JSON value")
		}
		return MapSnapshotEntity{}, err
	}
	if !validMapSnapshotEntityLabel(entity.Name) || !validMapSnapshotEntityLabel(entity.Type) {
		return MapSnapshotEntity{}, errors.New("entity name or type is invalid")
	}
	if entity.Direction > 15 {
		return MapSnapshotEntity{}, errors.New("entity direction is invalid")
	}
	coordinates := []float64{
		entity.BoundingBox.LeftTop.X,
		entity.BoundingBox.LeftTop.Y,
		entity.BoundingBox.RightBottom.X,
		entity.BoundingBox.RightBottom.Y,
	}
	for _, coordinate := range coordinates {
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) || math.Abs(coordinate) > maximumMapSnapshotCoordinate {
			return MapSnapshotEntity{}, errors.New("entity bounding box coordinate is invalid")
		}
	}
	if entity.BoundingBox.LeftTop.X > entity.BoundingBox.RightBottom.X || entity.BoundingBox.LeftTop.Y > entity.BoundingBox.RightBottom.Y {
		return MapSnapshotEntity{}, errors.New("entity bounding box is inverted")
	}
	return entity, nil
}

func validMapSnapshotEntityLabel(value string) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 200 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func renderMapSnapshotSurface(surface mapSnapshotExporterSurface, sourcePath, destinationPath string) (int, int, error) {
	geometry, err := calculateMapSnapshotRenderGeometry(surface)
	if err != nil {
		return 0, 0, err
	}
	renderMinX, renderMinY, renderMaxX, renderMaxY := geometry.MinTileX, geometry.MinTileY, geometry.MaxTileX, geometry.MaxTileY
	scale := geometry.PixelsPerTile
	width, height := geometry.Width, geometry.Height
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 7, G: 12, B: 17, A: 255}}, image.Point{}, draw.Src)

	file, err := os.Open(sourcePath)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, maximumMapSnapshotExportBytes+1))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	count := 0
	renderedCount := 0
	seen := make(map[[2]int]bool, surface.ChunkCount)
	for scanner.Scan() {
		var chunk mapSnapshotExporterChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			return 0, 0, fmt.Errorf("decode chunk line: %w", err)
		}
		key := [2]int{chunk.X, chunk.Y}
		if seen[key] || chunk.X < surface.MinX || chunk.X > surface.MaxX || chunk.Y < surface.MinY || chunk.Y > surface.MaxY {
			return 0, 0, fmt.Errorf("invalid or duplicate chunk %d,%d", chunk.X, chunk.Y)
		}
		seen[key] = true
		count++
		chunkMinX := chunk.X * mapSnapshotChunkPixels
		chunkMinY := chunk.Y * mapSnapshotChunkPixels
		chunkMaxX := chunkMinX + mapSnapshotChunkPixels - 1
		chunkMaxY := chunkMinY + mapSnapshotChunkPixels - 1
		if chunkMaxX < renderMinX || chunkMinX > renderMaxX || chunkMaxY < renderMinY || chunkMinY > renderMaxY {
			continue
		}
		pixels, err := decodeMapSnapshotChunk(chunk.Data)
		if err != nil {
			return 0, 0, fmt.Errorf("decode chunk %d,%d: %w", chunk.X, chunk.Y, err)
		}
		paintMapSnapshotChunk(canvas, pixels, chunk.X, chunk.Y, renderMinX, renderMinY, renderMaxX, renderMaxY, scale)
		renderedCount++
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if count != surface.ChunkCount {
		return 0, 0, fmt.Errorf("expected %d chunks, read %d", surface.ChunkCount, count)
	}
	if renderedCount == 0 {
		return 0, 0, errors.New("surface view contains no charted chunks")
	}

	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, 0, err
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	encodeErr := encoder.Encode(destination, canvas)
	closeErr := destination.Close()
	if encodeErr != nil {
		return 0, 0, encodeErr
	}
	if closeErr != nil {
		return 0, 0, closeErr
	}
	return width, height, nil
}

type mapSnapshotRenderGeometry struct {
	MinTileX      int
	MinTileY      int
	MaxTileX      int
	MaxTileY      int
	Width         int
	Height        int
	PixelsPerTile float64
}

func calculateMapSnapshotRenderGeometry(surface mapSnapshotExporterSurface) (mapSnapshotRenderGeometry, error) {
	renderMinX, renderMinY, renderMaxX, renderMaxY, err := mapSnapshotRenderTileBounds(surface)
	if err != nil {
		return mapSnapshotRenderGeometry{}, err
	}
	sourceWidth := renderMaxX - renderMinX + 1
	sourceHeight := renderMaxY - renderMinY + 1
	if sourceWidth < 1 || sourceHeight < 1 {
		return mapSnapshotRenderGeometry{}, errors.New("surface bounds are empty")
	}
	scale := 1.0
	if sourceWidth > maximumMapSnapshotDimension || sourceHeight > maximumMapSnapshotDimension {
		widthScale := float64(maximumMapSnapshotDimension) / float64(sourceWidth)
		heightScale := float64(maximumMapSnapshotDimension) / float64(sourceHeight)
		if widthScale < heightScale {
			scale = widthScale
		} else {
			scale = heightScale
		}
	}
	return mapSnapshotRenderGeometry{
		MinTileX: renderMinX, MinTileY: renderMinY, MaxTileX: renderMaxX, MaxTileY: renderMaxY,
		Width: max(1, int(float64(sourceWidth)*scale)), Height: max(1, int(float64(sourceHeight)*scale)), PixelsPerTile: scale,
	}, nil
}

func mapSnapshotRenderTileBounds(surface mapSnapshotExporterSurface) (int, int, int, int, error) {
	if surface.MinX > surface.MaxX || surface.MinY > surface.MaxY {
		return 0, 0, 0, 0, errors.New("surface chunk bounds are inverted")
	}
	minimumChunk := -((maximumMapSnapshotCoordinate + mapSnapshotChunkPixels - 1) / mapSnapshotChunkPixels)
	maximumChunk := (maximumMapSnapshotCoordinate+1)/mapSnapshotChunkPixels - 1
	if surface.MinX < minimumChunk || surface.MinY < minimumChunk || surface.MaxX > maximumChunk || surface.MaxY > maximumChunk {
		return 0, 0, 0, 0, errors.New("surface chunk bounds exceed the supported coordinate limit")
	}
	chartMinX := surface.MinX * mapSnapshotChunkPixels
	chartMinY := surface.MinY * mapSnapshotChunkPixels
	chartMaxX := (surface.MaxX+1)*mapSnapshotChunkPixels - 1
	chartMaxY := (surface.MaxY+1)*mapSnapshotChunkPixels - 1
	viewBounds := []*int{surface.ViewMinTileX, surface.ViewMinTileY, surface.ViewMaxTileX, surface.ViewMaxTileY}
	present := 0
	for _, value := range viewBounds {
		if value != nil {
			present++
		}
	}
	if present == 0 {
		return chartMinX, chartMinY, chartMaxX, chartMaxY, nil
	}
	if present != len(viewBounds) {
		return 0, 0, 0, 0, errors.New("surface view bounds are incomplete")
	}
	if *surface.ViewMinTileX < chartMinX || *surface.ViewMinTileY < chartMinY || *surface.ViewMaxTileX > chartMaxX || *surface.ViewMaxTileY > chartMaxY || *surface.ViewMinTileX > *surface.ViewMaxTileX || *surface.ViewMinTileY > *surface.ViewMaxTileY {
		return 0, 0, 0, 0, errors.New("surface view bounds exceed charted bounds")
	}
	return *surface.ViewMinTileX, *surface.ViewMinTileY, *surface.ViewMaxTileX, *surface.ViewMaxTileY, nil
}

func decodeMapSnapshotChunk(encoded string) ([]byte, error) {
	compressed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	pixels, readErr := io.ReadAll(io.LimitReader(reader, mapSnapshotChunkPixels*mapSnapshotChunkPixels*2+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(pixels) != mapSnapshotChunkPixels*mapSnapshotChunkPixels*2 {
		return nil, fmt.Errorf("expected 2048 RGB565 bytes, got %d", len(pixels))
	}
	return pixels, nil
}

func paintMapSnapshotChunk(canvas *image.RGBA, pixels []byte, chunkX, chunkY, minX, minY, maxX, maxY int, scale float64) {
	baseX := chunkX * mapSnapshotChunkPixels
	baseY := chunkY * mapSnapshotChunkPixels
	for y := 0; y < mapSnapshotChunkPixels; y++ {
		for x := 0; x < mapSnapshotChunkPixels; x++ {
			tileX := baseX + x
			tileY := baseY + y
			if tileX < minX || tileX > maxX || tileY < minY || tileY > maxY {
				continue
			}
			offset := (y*mapSnapshotChunkPixels + x) * 2
			value := binary.LittleEndian.Uint16(pixels[offset : offset+2])
			red := uint8(((value >> 11) & 0x1f) * 255 / 31)
			green := uint8(((value >> 5) & 0x3f) * 255 / 63)
			blue := uint8((value & 0x1f) * 255 / 31)
			destinationX := int(float64(tileX-minX) * scale)
			destinationY := int(float64(tileY-minY) * scale)
			if image.Pt(destinationX, destinationY).In(canvas.Bounds()) {
				canvas.SetRGBA(destinationX, destinationY, color.RGBA{R: red, G: green, B: blue, A: 255})
			}
		}
	}
}
