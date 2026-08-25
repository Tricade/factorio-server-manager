package factorio

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type ModPackMap map[string]*ModPack
type ModPack struct {
	Mods Mods
}

type ModPackResult struct {
	Name string         `json:"name"`
	Mods ModsResultList `json:"mods"`
}

func NewModPackMap() (ModPackMap, error) {
	var err error
	modPackMap := make(ModPackMap)

	err = modPackMap.reload()
	if err != nil {
		log.Printf("error on loading the modpacks: %s", err)
		return modPackMap, err
	}

	return modPackMap, nil
}

func newModPack(modPackFolder string) (*ModPack, error) {
	var err error
	var modPack ModPack

	modPack.Mods, err = NewMods(modPackFolder)
	if err != nil {
		log.Printf("error on loading mods in mod_pack_dir: %s", err)
		return &modPack, err
	}

	return &modPack, err
}

func (modPackMap *ModPackMap) reload() error {
	var err error
	newModPackMap := make(ModPackMap)
	config := bootstrap.GetConfig()

	err = filepath.Walk(config.FactorioModPackDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}
		if path == config.FactorioModPackDir || !info.IsDir() {
			return nil
		}

		modPackName := filepath.Base(path)

		newModPackMap[modPackName], err = newModPack(path)
		if err != nil {
			log.Printf("error on creating newModPack: %s", err)
			return err
		}

		return nil
	})
	if err != nil {
		log.Printf("error on walking over the ModDir: %s", err)
		return err
	}

	*modPackMap = newModPackMap

	return nil
}

func (modPackMap *ModPackMap) ListInstalledModPacks() []ModPackResult {
	list := make([]ModPackResult, 0)

	for modPackName, modPack := range *modPackMap {
		var modPackResult ModPackResult
		modPackResult.Name = modPackName
		modPackResult.Mods = modPack.Mods.ListInstalledMods()

		list = append(list, modPackResult)
	}

	return list
}

func (modPackMap *ModPackMap) CreateModPack(modPackName string) error {
	if err := ValidatePathElement(modPackName); err != nil {
		return errors.New("invalid mod pack name: " + err.Error())
	}
	config := bootstrap.GetConfig()
	modPackFolder := filepath.Join(config.FactorioModPackDir, modPackName)

	if modPackMap.CheckModPackExists(modPackName) == true {
		log.Printf("ModPack %s already existis", modPackName)
		return errors.New("ModPack " + modPackName + " already exists, please choose a different name")
	}

	if err := os.MkdirAll(config.FactorioModPackDir, 0755); err != nil {
		return fmt.Errorf("create mod pack root: %w", err)
	}
	staging, err := os.MkdirTemp(config.FactorioModPackDir, ".modpack-create-")
	if err != nil {
		return fmt.Errorf("stage mod pack: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := copyProfileDirectory(config.FactorioModsDir, staging); err != nil {
		return fmt.Errorf("copy active mods into staged mod pack: %w", err)
	}
	if _, err := NewMods(staging); err != nil {
		return fmt.Errorf("validate staged mod pack: %w", err)
	}
	if err := profileRename(staging, modPackFolder); err != nil {
		return fmt.Errorf("activate staged mod pack: %w", err)
	}

	//reload the ModPackList
	err = modPackMap.reload()
	if err != nil {
		log.Printf("error reloading ModPack: %s", err)
		return err
	}

	return nil
}

func (modPackMap *ModPackMap) CreateEmptyModPack(packName string) error {
	if err := ValidatePathElement(packName); err != nil {
		return errors.New("invalid mod pack name: " + err.Error())
	}
	config := bootstrap.GetConfig()
	modPackFolder := filepath.Join(config.FactorioModPackDir, packName)

	if modPackMap.CheckModPackExists(packName) == true {
		log.Printf("ModPack %s already existis", packName)
		return errors.New("ModPack " + packName + " already exists, please choose a different name")
	}

	if err := os.MkdirAll(config.FactorioModPackDir, 0755); err != nil {
		return fmt.Errorf("create mod pack root: %w", err)
	}
	staging, err := os.MkdirTemp(config.FactorioModPackDir, ".modpack-empty-")
	if err != nil {
		return fmt.Errorf("stage empty mod pack: %w", err)
	}
	defer os.RemoveAll(staging)
	if _, err := NewMods(staging); err != nil {
		return fmt.Errorf("initialize staged empty mod pack: %w", err)
	}
	if err := profileRename(staging, modPackFolder); err != nil {
		return fmt.Errorf("activate staged empty mod pack: %w", err)
	}

	err = modPackMap.reload()
	if err != nil {
		log.Printf("error reloading ModPack: %s", err)
		return err
	}
	return nil
}

func (modPackMap *ModPackMap) CheckModPackExists(modPackName string) bool {
	for modPackId := range *modPackMap {
		if modPackId == modPackName {
			return true
		}
	}

	return false
}

func (modPackMap *ModPackMap) DeleteModPack(modPackName string) error {
	var err error
	if err := ValidatePathElement(modPackName); err != nil {
		return errors.New("invalid mod pack name: " + err.Error())
	}
	config := bootstrap.GetConfig()
	modPackDir := filepath.Join(config.FactorioModPackDir, modPackName)

	err = os.RemoveAll(modPackDir)
	if err != nil {
		log.Printf("error on removing the ModPack: %s", err)
		return err
	}

	err = modPackMap.reload()
	if err != nil {
		log.Printf("error on reloading the ModPackList: %s", err)
		return err
	}

	return nil
}

func (modPack *ModPack) LoadModPack() error {
	// Replacing the active mods directory must exclude every other profile-data
	// reader and writer. In particular, a concurrent download/list request must
	// never observe the short rename window between the old and new directory.
	profileDataGate.Lock()
	defer profileDataGate.Unlock()
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	if GetFactorioServer().IsBusy() {
		return ErrServerActive
	}

	source := modPack.Mods.ModInfoList.Destination
	destination := profileActiveDirectories()["mods"]
	if source == "" || destination == "" {
		return errors.New("mod pack source or active mods directory is not configured")
	}
	swap, err := prepareModPackDirectorySwap(source, destination)
	if err != nil {
		return fmt.Errorf("stage mod pack: %w", err)
	}
	defer swap.cleanup()
	if err := swap.activate(); err != nil {
		return fmt.Errorf("activate staged mod pack: %w", err)
	}
	if _, err := NewMods(destination); err != nil {
		rollbackErr := swap.rollback()
		if rollbackErr != nil {
			return fmt.Errorf("validate active mod pack: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("validate active mod pack: %w (previous mods restored)", err)
	}
	if err := swap.commit(); err != nil {
		log.Printf("Mod pack was activated but transactional backup cleanup failed: %v", err)
	}
	return nil
}

// Mod-pack activation uses the mount-safe entry transaction shared with
// profile activation. The active mods directory itself can be a Docker mount
// point, so it must never be renamed.
type modPackDirectorySwap = directoryEntrySwap

func prepareModPackDirectorySwap(source, destination string) (*modPackDirectorySwap, error) {
	if source == "" || destination == "" || filepath.Dir(destination) == "" {
		return nil, errors.New("mod pack source or destination is empty")
	}
	swap, err := newDirectoryEntrySwap(destination, ".mods-modpack-staging-", ".mods-modpack-backup-")
	if err != nil {
		return nil, fmt.Errorf("create staged mods directory: %w", err)
	}
	if err := copyProfileDirectory(source, swap.staging); err != nil {
		_ = swap.cleanup()
		return nil, fmt.Errorf("copy mod pack into staging: %w", err)
	}
	if _, err := NewMods(swap.staging); err != nil {
		_ = swap.cleanup()
		return nil, fmt.Errorf("validate staged mod pack: %w", err)
	}
	return swap, nil
}
