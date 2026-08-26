package factorio

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

const (
	maximumSaveHeaderStringBytes = 1 * 1024 * 1024
	maximumSaveHeaderMods        = 100000
	maximumSaveHeaderStatForces  = 1024
	maximumSaveHeaderStatEntries = 1_000_000
)

type archiveFile struct {
	io.ReadCloser
	archive io.Closer
}

func (af *archiveFile) Close() error {
	if af.ReadCloser != nil {
		if err := af.ReadCloser.Close(); err != nil {
			return err
		}
	}
	if af.archive != nil {
		if err := af.archive.Close(); err != nil {
			return err
		}
	}
	return nil
}

// openNames contains a list of all. Will stop searching at the first occurance
func OpenArchiveFile(path string, openNames ...string) (r io.ReadCloser, err error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}

	f := &archiveFile{archive: archive}

	for _, file := range archive.File {
		name := file.FileInfo().Name()
		for _, openName := range openNames {
			if name == openName {
				f.ReadCloser, err = file.Open()
				if err != nil {
					archive.Close()
					return nil, err
				}
				return f, nil
			}
		}
	}

	archive.Close()
	return nil, os.ErrNotExist
}

// ReadSaveHeader resolves a profile-scoped save by its portable filename,
// rejects non-regular or structurally invalid archives and decodes the first
// Factorio level header it contains.
func ReadSaveHeader(name string) (SaveHeader, error) {
	if _, err := FindSave(name); err != nil {
		return SaveHeader{}, err
	}
	path := filepath.Join(bootstrap.GetConfig().FactorioSavesDir, name)
	if err := ValidateSaveArchive(path); err != nil {
		return SaveHeader{}, fmt.Errorf("invalid Factorio save archive: %w", err)
	}
	file, err := OpenArchiveFile(path, "level.dat", "level-init.dat")
	if err != nil {
		return SaveHeader{}, fmt.Errorf("open save level file: %w", err)
	}
	defer file.Close()

	var header SaveHeader
	if err := header.DecodeFrom(file); err != nil {
		return SaveHeader{}, fmt.Errorf("read save header: %w", err)
	}
	return header, nil
}

type SaveHeader struct {
	FactorioVersion           Version                      `json:"factorio_version"`
	Campaign                  string                       `json:"campaign"`
	Name                      string                       `json:"name"`
	BaseMod                   string                       `json:"base_mod"`
	Difficulty                uint8                        `json:"difficulty"`
	Finished                  bool                         `json:"finished"`
	PlayerWon                 bool                         `json:"player_won"`
	NextLevel                 string                       `json:"next_level"`
	CanContinue               bool                         `json:"can_continue"`
	FinishedButContinuing     bool                         `json:"finished_but_continuing"`
	SavingReplay              bool                         `json:"saving_replay"`
	AllowNonAdminDebugOptions bool                         `json:"allow_non_admin_debug_options"`
	LoadedFrom                Version                      `json:"loaded_from"`
	LoadedFromBuild           uint16                       `json:"loaded_from_build"`
	AllowedCommands           uint8                        `json:"allowed_commands"`
	Stats                     map[byte][]map[uint16]uint32 `json:"stats,omitempty"`
	Mods                      []Mod                        `json:"mods"`
}

type Mod struct {
	Name    string  `json:"name"`
	Version Version `json:"version"`
	CRC     uint32  `json:"crc"`
}

// Mostly based on https://forums.factorio.com/viewtopic.php?f=5&t=8568&p=277892&hilit=level.dat+python#p277892
func (h *SaveHeader) DecodeFrom(r io.Reader) (err error) {
	var scratch [8]byte

	var fv version64
	_, err = io.ReadFull(r, scratch[:8])
	if err != nil {
		return err
	}
	if err := fv.UnmarshalBinary(scratch[:8]); err != nil {
		return fmt.Errorf("read FactorioVersion: %v", err)
	}
	h.FactorioVersion = Version(fv)

	atLeast016 := !h.FactorioVersion.Less(Version{0, 16, 0, 0})

	if !h.FactorioVersion.Less(Version{0, 17, 0, 0}) {
		//FIXME correct naming
		_, err = io.ReadFull(r, scratch[:1])
		if err != nil {
			return fmt.Errorf("read first random 0.17 byte: %v", err)
		}
	}

	// campaign, example: "transport-belt-madness"
	h.Campaign, err = readString(r, Version(h.FactorioVersion), false)
	if err != nil {
		return fmt.Errorf("read Campaign: %v", err)
	}

	// level name, example: "level-01"
	h.Name, err = readString(r, Version(h.FactorioVersion), false)
	if err != nil {
		return fmt.Errorf("read Name: %v", err)
	}

	// name of the base mod, example: "base"
	h.BaseMod, err = readString(r, Version(h.FactorioVersion), false)
	if err != nil {
		return fmt.Errorf("read BaseMod: %v", err)
	}

	// Read Difficulty
	_, err = io.ReadFull(r, scratch[:1])
	if err != nil {
		return fmt.Errorf("read Difficulty: %v", err)
	}
	h.Difficulty = scratch[0]

	// Read finished as bool
	_, err = io.ReadFull(r, scratch[:1])
	if err != nil {
		return fmt.Errorf("read Finished: %v", err)
	}
	h.Finished = scratch[0] != 0

	// Read playerWon as bool
	_, err = io.ReadFull(r, scratch[:1])
	if err != nil {
		return fmt.Errorf("read PlayerWon: %v", err)
	}
	h.PlayerWon = scratch[0] != 0

	// Read NextLevel string (normally empty)
	h.NextLevel, err = readString(r, Version(h.FactorioVersion), false)
	if err != nil {
		return fmt.Errorf("read NextLevel: %v", err)
	}

	if !h.FactorioVersion.Less(Version{0, 12, 0, 0}) {
		_, err = io.ReadFull(r, scratch[:1])
		if err != nil {
			return fmt.Errorf("read CanContinue: %v", err)
		}
		h.CanContinue = scratch[0] != 0

		_, err = io.ReadFull(r, scratch[:1])
		if err != nil {
			return fmt.Errorf("read FinishedButContinuing: %v", err)
		}
		h.FinishedButContinuing = scratch[0] != 0
	}

	_, err = io.ReadFull(r, scratch[:1])
	if err != nil {
		return fmt.Errorf("read SavingReplay: %v", err)
	}
	h.SavingReplay = scratch[0] != 0

	if atLeast016 {
		_, err = io.ReadFull(r, scratch[:1])
		if err != nil {
			return fmt.Errorf("read AllowNonAdminDebugOptions: %v", err)
		}
		h.AllowNonAdminDebugOptions = scratch[0] != 0
	}

	var loadedFrom version48
	err = loadedFrom.decodeFrom(r, Version(h.FactorioVersion))
	if err != nil {
		return fmt.Errorf("read LoadedFrom: %v", err)
	}
	h.LoadedFrom = Version(loadedFrom)

	_, err = io.ReadFull(r, scratch[:2])
	if err != nil {
		return fmt.Errorf("read LoadedFromBuild: %v", err)
	}
	h.LoadedFromBuild = binary.LittleEndian.Uint16(scratch[:2])

	_, err = io.ReadFull(r, scratch[:1])
	if err != nil {
		return fmt.Errorf("read AllowedCommands: %v", err)
	}
	h.AllowedCommands = scratch[0]
	if h.FactorioVersion.Less(Version{0, 13, 0, 87}) {
		if h.AllowedCommands == 0 {
			h.AllowedCommands = 2
		} else {
			h.AllowedCommands = 1
		}
	}

	if h.FactorioVersion.Less(Version{0, 13, 0, 42}) {
		h.Stats, err = h.readStats(r)
		if err != nil {
			return fmt.Errorf("read Stats: %v", err)
		}
	}

	var n uint32
	if atLeast016 {
		n, err = readOptimUint(r, Version(h.FactorioVersion), 32)
		if err != nil {
			return fmt.Errorf("read num mods: %v", err)
		}
	} else {
		_, err = io.ReadFull(r, scratch[:4])
		if err != nil {
			return fmt.Errorf("read num mods: %v", err)
		}
		n = binary.LittleEndian.Uint32(scratch[:4])
	}
	if n > maximumSaveHeaderMods {
		return fmt.Errorf("read num mods: %d exceeds limit %d", n, maximumSaveHeaderMods)
	}

	for i := uint32(0); i < n; i++ {
		var m Mod
		if err = (&m).decodeFrom(r, Version(h.FactorioVersion)); err != nil {
			return fmt.Errorf("read mod: %v", err)
		}
		h.Mods = append(h.Mods, m)
	}

	return nil
}

func readOptimUint(r io.Reader, v Version, bitSize int) (uint32, error) {
	var b [4]byte
	if !v.Less(Version{0, 14, 14, 0}) {
		_, err := io.ReadFull(r, b[:1])
		if err != nil {
			return 0, err
		}
		if b[0] != 0xFF {
			return uint32(b[0]), nil
		}
	}

	if bitSize < 0 || bitSize > 64 || (bitSize%8 != 0) {
		panic("invalid bit size")
	}

	_, err := io.ReadFull(r, b[:bitSize/8])
	if err != nil {
		return 0, err
	}

	switch bitSize {
	case 16:
		return uint32(binary.LittleEndian.Uint16(b[:2])), nil
	case 32:
		return binary.LittleEndian.Uint32(b[:4]), nil
	default:
		panic("invalid bit size")
	}
}

func readString(r io.Reader, game Version, forceOptimized bool) (s string, err error) {
	var n uint32

	// since 0.16 read optimized uint
	if !game.Less(Version{0, 16, 0, 0}) || forceOptimized {
		n, err = readOptimUint(r, game, 32)
		if err != nil {
			return "", err
		}
	} else {
		var b [4]byte
		_, err := io.ReadFull(r, b[:])
		if err != nil {
			return "", fmt.Errorf("failed to read string length: %v", err)
		}
		n = uint32(binary.LittleEndian.Uint32(b[:]))
	}

	if n > maximumSaveHeaderStringBytes {
		return "", fmt.Errorf("string length %d exceeds limit %d", n, maximumSaveHeaderStringBytes)
	}
	d := make([]byte, n)
	_, err = io.ReadFull(r, d)
	if err != nil {
		return "", fmt.Errorf("failed to read string: %v", err)
	}

	return string(d), nil
}

func (h *SaveHeader) readStats(r io.Reader) (stats map[byte][]map[uint16]uint32, err error) {
	var scratch [4]byte
	stats = make(map[byte][]map[uint16]uint32)

	_, err = io.ReadFull(r, scratch[:4])
	if err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(scratch[:4])
	if n > maximumSaveHeaderStatForces {
		return nil, fmt.Errorf("stat force count %d exceeds limit %d", n, maximumSaveHeaderStatForces)
	}
	for i := uint32(0); i < n; i++ {
		_, err := io.ReadFull(r, scratch[:1])
		if err != nil {
			return nil, fmt.Errorf("read stat %d force id: %v", i, err)
		}
		id := scratch[0]
		for j := 0; j < 3; j++ {
			st := make(map[uint16]uint32)
			_, err = io.ReadFull(r, scratch[:4])
			if err != nil {
				return nil, fmt.Errorf("read stat %d (id %d) length: %v", i, id, err)
			}
			length := binary.LittleEndian.Uint32(scratch[:4])
			if length > maximumSaveHeaderStatEntries {
				return nil, fmt.Errorf("stat %d (id %d) length %d exceeds limit %d", i, id, length, maximumSaveHeaderStatEntries)
			}
			for k := uint32(0); k < length; k++ {
				_, err = io.ReadFull(r, scratch[:2])
				if err != nil {
					return nil, fmt.Errorf("read stat %d (id %d; index %d) key: %v", i, id, k, err)
				}
				key := binary.LittleEndian.Uint16(scratch[:2])
				_, err = io.ReadFull(r, scratch[:4])
				if err != nil {
					return nil, fmt.Errorf("read stat %d (id %d; index %d) val: %v", i, id, k, err)
				}
				val := binary.LittleEndian.Uint32(scratch[:4])
				st[key] = val
			}
			stats[id] = append(stats[id], st)
		}
	}

	return stats, nil
}

func (m *Mod) decodeFrom(r io.Reader, game Version) (err error) {
	m.Name, err = readString(r, game, true)
	if err != nil {
		return fmt.Errorf("read Name: %v", err)
	}

	var version version48
	err = version.decodeFrom(r, game)
	if err != nil {
		return err
	}
	m.Version = Version(version)

	var scratch [4]byte
	if game.Greater(Version{0, 15, 0, 91}) {
		_, err = io.ReadFull(r, scratch[:4])
		if err != nil {
			return err
		}
		m.CRC = binary.LittleEndian.Uint32(scratch[:4])
	}

	return nil
}
