package factorio

import (
	"fmt"
	"log"
	"os"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type LoginErrorResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}
type LoginSuccessResponse struct {
	UserKey []string `json:""`
}

func DeleteAllMods() error {
	config := bootstrap.GetConfig()
	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	if GetFactorioServer().IsBusy() {
		return ErrServerActive
	}
	return replaceActiveModsWithEmptyDirectory(config.FactorioModsDir)
}

// replaceActiveModsWithEmptyDirectory prepares a valid base-only mods
// directory before renaming the current directory aside. Activation failure
// restores the original directory wholesale instead of leaving a deleted or
// half-recreated active state.
func replaceActiveModsWithEmptyDirectory(destination string) error {
	modsDirInfo, err := os.Stat(destination)
	if err != nil {
		log.Printf("error getting stats of FactorioModsDir: %s", err)
		return err
	}

	modsDirPerm := modsDirInfo.Mode().Perm()
	swap, err := newDirectoryEntrySwap(destination, ".mods-empty-staging-", ".mods-empty-backup-")
	if err != nil {
		return fmt.Errorf("stage empty mods directory: %w", err)
	}
	defer swap.cleanup()
	if err := os.Chmod(swap.staging, modsDirPerm); err != nil {
		return fmt.Errorf("set staged mods permissions: %w", err)
	}
	if _, err := NewMods(swap.staging); err != nil {
		return fmt.Errorf("initialize staged mods directory: %w", err)
	}
	if err := swap.activate(); err != nil {
		return fmt.Errorf("activate empty mods directory: %w", err)
	}
	if err := swap.commit(); err != nil {
		log.Printf("Empty mods directory is active but previous-directory cleanup failed: %v", err)
	}
	return nil
}

func ModStartUp() error {
	return prepareModStartup(bootstrap.GetConfig())
}
