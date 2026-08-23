package factorio

import (
	"bytes"
	"encoding/binary"
	"testing"
	"testing/iotest"
)

func TestReadOptimUintHandlesShortReads(t *testing.T) {
	value, err := readOptimUint(iotest.OneByteReader(bytes.NewReader([]byte{0x78, 0x56, 0x34, 0x12})), Version{0, 13, 0, 0}, 32)
	if err != nil {
		t.Fatalf("read optimized integer: %v", err)
	}
	if value != 0x12345678 {
		t.Fatalf("unexpected optimized integer %#x", value)
	}
}

func TestReadStatsUsesDecodedForceID(t *testing.T) {
	data := make([]byte, 0, 17)
	count := make([]byte, 4)
	binary.LittleEndian.PutUint32(count, 1)
	data = append(data, count...)
	data = append(data, byte(7))
	data = append(data, make([]byte, 12)...)

	stats, err := (&SaveHeader{}).readStats(iotest.OneByteReader(bytes.NewReader(data)))
	if err != nil {
		t.Fatalf("read stats: %v", err)
	}
	if len(stats[7]) != 3 {
		t.Fatalf("expected three stat maps for force 7, got %#v", stats)
	}
	if _, wrongID := stats[0]; wrongID {
		t.Fatal("stats were assigned to the stale scratch byte instead of the decoded force ID")
	}
}

func TestReadStringRejectsOversizedLength(t *testing.T) {
	length := make([]byte, 4)
	binary.LittleEndian.PutUint32(length, maximumSaveHeaderStringBytes+1)
	if _, err := readString(bytes.NewReader(length), Version{0, 15, 0, 0}, false); err == nil {
		t.Fatal("expected oversized save-header string to be rejected")
	}
}

// 1.1.14 changed the format of the saves, so new test has to be done
func Test1_1_14(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_1_1_14.zip", "level-init.dat")
	if err != nil {
		t.Fatalf("Error opening level.datmetadata: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:           Version{1, 1, 19, 0},
		Campaign:                  "transport-belt-madness",
		Name:                      "level-01",
		BaseMod:                   "base",
		Difficulty:                1,
		Finished:                  false,
		PlayerWon:                 false,
		NextLevel:                 "",
		CanContinue:               false,
		FinishedButContinuing:     false,
		SavingReplay:              false,
		AllowNonAdminDebugOptions: true,
		LoadedFrom:                Version{1, 1, 19},
		LoadedFromBuild:           57957,
		AllowedCommands:           1,
		Mods: []Mod{
			{
				Version: Version{1, 1, 19},
				Name:    "base",
			},
			{
				Version: Version{3, 0, 0},
				Name:    "belt-balancer",
			},
			{
				Version: Version{3, 0, 0},
				Name:    "train-station-overview",
			},
		},
	}

	header.Equals(testHeader, t)
}

// 1.1 Binary seems equal to 0.18/1.0 binary, just the default values changed
func Test1_1(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_1_1.zip", "level.dat")
	if err != nil {
		t.Fatalf("Error opening level.dat: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:           Version{1, 1, 6, 4},
		Campaign:                  "transport-belt-madness",
		Name:                      "level-01",
		BaseMod:                   "base",
		Difficulty:                1,
		Finished:                  false,
		PlayerWon:                 false,
		NextLevel:                 "",
		CanContinue:               false,
		FinishedButContinuing:     false,
		SavingReplay:              false,
		AllowNonAdminDebugOptions: true,
		LoadedFrom:                Version{1, 1, 6},
		LoadedFromBuild:           57355,
		AllowedCommands:           1,
		Mods: []Mod{
			{
				Version: Version{1, 1, 6},
				Name:    "base",
			},
			{
				Version: Version{3, 0, 0},
				Name:    "belt-balancer",
			},
			{
				Version: Version{3, 0, 0},
				Name:    "train-station-overview",
			},
		},
	}

	header.Equals(testHeader, t)
}

// 0.18 Binary seems equal to 0.17 binary, just the default values changed
// 1.0 == 0.18.48
func Test0_18(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_0_18.zip", "level.dat")
	if err != nil {
		t.Fatalf("Error opening level.dat: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:           Version{0, 18, 2, 2},
		Campaign:                  "transport-belt-madness",
		Name:                      "level-01",
		BaseMod:                   "base",
		Difficulty:                1,
		Finished:                  false,
		PlayerWon:                 false,
		NextLevel:                 "",
		CanContinue:               false,
		FinishedButContinuing:     false,
		SavingReplay:              false,
		AllowNonAdminDebugOptions: true,
		LoadedFrom:                Version{0, 18, 2},
		LoadedFromBuild:           49204,
		AllowedCommands:           1,
		Mods: []Mod{
			{
				Version: Version{0, 18, 2},
				Name:    "base",
			},
			{
				Version: Version{2, 0, 0},
				Name:    "belt-balancer",
			},
			{
				Version: Version{2, 0, 1},
				Name:    "train-station-overview",
			},
		},
	}

	header.Equals(testHeader, t)
}

func Test0_17(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_0_17.zip", "level.dat")
	if err != nil {
		t.Fatalf("Error opening level.dat: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:           Version{0, 17, 1, 1},
		Campaign:                  "transport-belt-madness",
		Name:                      "level-01",
		BaseMod:                   "base",
		Difficulty:                0,
		Finished:                  false,
		PlayerWon:                 false,
		NextLevel:                 "",
		CanContinue:               false,
		FinishedButContinuing:     false,
		SavingReplay:              true,
		AllowNonAdminDebugOptions: true,
		LoadedFrom:                Version{0, 17, 1},
		LoadedFromBuild:           43001,
		AllowedCommands:           1,
		Mods: []Mod{
			{
				Version: Version{0, 2, 0},
				Name:    "Warehousing",
			},
			{
				Version: Version{0, 17, 1},
				Name:    "base",
			},
		},
	}

	header.Equals(testHeader, t)
}

func Test0_16(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_0_16.zip", "level.dat")
	if err != nil {
		t.Fatalf("Error opening level.dat: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:           Version{0, 16, 51, 0},
		Campaign:                  "transport-belt-madness",
		Name:                      "level-01",
		BaseMod:                   "base",
		Difficulty:                0,
		Finished:                  false,
		PlayerWon:                 false,
		NextLevel:                 "",
		CanContinue:               false,
		FinishedButContinuing:     false,
		SavingReplay:              true,
		AllowNonAdminDebugOptions: true,
		LoadedFrom:                Version{0, 16, 51},
		LoadedFromBuild:           36654,
		AllowedCommands:           1,
		Mods: []Mod{
			{
				Version: Version{0, 1, 3},
				Name:    "Warehousing",
			},
			{
				Version: Version{0, 16, 51},
				Name:    "base",
			},
		},
	}

	header.Equals(testHeader, t)
}

func Test0_15(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_0_15.zip", "level.dat")
	if err != nil {
		t.Fatalf("Error opening level.dat: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:       Version{0, 15, 40, 0},
		Campaign:              "transport-belt-madness",
		Name:                  "level-01",
		BaseMod:               "base",
		Difficulty:            0,
		Finished:              false,
		PlayerWon:             false,
		NextLevel:             "",
		CanContinue:           false,
		FinishedButContinuing: false,
		SavingReplay:          true,
		LoadedFrom:            Version{0, 15, 40},
		LoadedFromBuild:       30950,
		AllowedCommands:       1,
		Mods: []Mod{
			{
				Version: Version{0, 0, 13},
				Name:    "Warehousing",
			},
			{
				Version: Version{0, 15, 40},
				Name:    "base",
			},
		},
	}

	header.Equals(testHeader, t)
}

func Test0_14(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_0_14.zip", "level.dat")
	if err != nil {
		t.Fatalf("Error opening level.dat: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:       Version{0, 14, 23, 0},
		Campaign:              "transport-belt-madness",
		Name:                  "level-01",
		BaseMod:               "base",
		Difficulty:            1,
		Finished:              false,
		PlayerWon:             false,
		NextLevel:             "",
		CanContinue:           false,
		FinishedButContinuing: false,
		SavingReplay:          true,
		LoadedFrom:            Version{0, 14, 23},
		LoadedFromBuild:       25374,
		AllowedCommands:       1,
		Mods: []Mod{
			{
				Version: Version{0, 0, 11},
				Name:    "Warehousing",
			},
			{
				Version: Version{0, 14, 23},
				Name:    "base",
			},
		},
	}

	header.Equals(testHeader, t)
}

func Test0_13(t *testing.T) {
	file, err := OpenArchiveFile("../factorio_testfiles/test_0_13.zip", "level.dat")
	if err != nil {
		t.Fatalf("Error opening level.dat: %s", err)
	}
	defer file.Close()

	var header SaveHeader
	err = header.DecodeFrom(file)
	if err != nil {
		t.Fatalf("Error reading header: %s", err)
	}

	testHeader := SaveHeader{
		FactorioVersion:       Version{0, 13, 20, 0},
		Campaign:              "transport-belt-madness",
		Name:                  "level-01",
		BaseMod:               "base",
		Difficulty:            1,
		Finished:              false,
		PlayerWon:             false,
		NextLevel:             "",
		CanContinue:           false,
		FinishedButContinuing: false,
		SavingReplay:          true,
		LoadedFrom:            Version{0, 13, 20},
		LoadedFromBuild:       24011,
		AllowedCommands:       1,
		Mods: []Mod{
			{
				Version: Version{1, 1, 0},
				Name:    "Extra-Virtual-Signals",
			},
			{
				Version: Version{0, 13, 20},
				Name:    "base",
			},
		},
	}

	header.Equals(testHeader, t)
}

func (h *SaveHeader) Equals(other SaveHeader, t *testing.T) {
	if h.FactorioVersion != other.FactorioVersion {
		t.Errorf("FactorioVersion not equal: %s --- %s", h.FactorioVersion, other.FactorioVersion)
	}
	if h.Campaign != other.Campaign {
		t.Errorf("Campaign not equal: %s --- %s", h.Campaign, other.Campaign)
	}
	if h.Name != other.Name {
		t.Errorf("Name not equal: %s --- %s", h.Name, other.Name)
	}
	if h.BaseMod != other.BaseMod {
		t.Errorf("BaseMod not equal: %s --- %s", h.BaseMod, other.BaseMod)
	}
	if h.Difficulty != other.Difficulty {
		t.Errorf("Difficulty not equal: %d --- %d", h.Difficulty, other.Difficulty)
	}
	if h.Finished != other.Finished {
		t.Errorf("Finished not equal: %t --- %t", h.Finished, other.Finished)
	}
	if h.PlayerWon != other.PlayerWon {
		t.Errorf("PlayerWon not equal: %t --- %t", h.PlayerWon, other.PlayerWon)
	}
	if h.NextLevel != other.NextLevel {
		t.Errorf("NextLevel not equal: %s --- %s", h.NextLevel, other.NextLevel)
	}
	if h.CanContinue != other.CanContinue {
		t.Errorf("CanContinue not equal: %t --- %t", h.CanContinue, other.CanContinue)
	}
	if h.FinishedButContinuing != other.FinishedButContinuing {
		t.Errorf("FinishedButContinuing not equal: %t --- %t", h.FinishedButContinuing, other.FinishedButContinuing)
	}
	if h.SavingReplay != other.SavingReplay {
		t.Errorf("SavingReplay not equal: %t --- %t", h.SavingReplay, other.SavingReplay)
	}
	if h.AllowNonAdminDebugOptions != other.AllowNonAdminDebugOptions {
		t.Errorf("AllowNonAdminDebugOptions not equal: %t --- %t", h.AllowNonAdminDebugOptions, other.AllowNonAdminDebugOptions)
	}
	if h.LoadedFrom != other.LoadedFrom {
		t.Errorf("LoadedFrom not equal: %s --- %s", h.LoadedFrom, other.LoadedFrom)
	}
	if h.LoadedFromBuild != other.LoadedFromBuild {
		t.Errorf("LoadedFromBuild not equal: %d --- %d", h.LoadedFromBuild, other.LoadedFromBuild)
	}
	if h.AllowedCommands != other.AllowedCommands {
		t.Errorf("AllowedCommands not equal: %d --- %d", h.AllowedCommands, other.AllowedCommands)
	}
	for k := range h.Mods {
		if h.Mods[k].Name != other.Mods[k].Name {
			t.Errorf("ModNames not equal: %s --- %s", h.Mods[k].Name, other.Mods[k].Name)
		} else if h.Mods[k].Version != other.Mods[k].Version {
			t.Errorf("ModVersions of Mod %s are not equal: %s --- %s", h.Mods[k].Name, h.Mods[k].Version, other.Mods[k].Version)
		}
	}
}
