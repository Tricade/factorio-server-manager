package factorio

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
)

type Save struct {
	Name    string    `json:"name"`
	LastMod time.Time `json:"last_mod"`
	Size    int64     `json:"size"`
}

func (s *Save) String() string {
	return s.Name
}

// Lists save files in factorio/saves
func ListSaves() (saves []Save, err error) {
	config := bootstrap.GetConfig()
	saves = []Save{}
	entries, err := os.ReadDir(config.FactorioSavesDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		if !isUsableSave(info) {
			continue
		}
		saves = append(saves, Save{
			info.Name(),
			info.ModTime(),
			info.Size(),
		})
	}
	return saves, nil
}

func isUsableSave(info os.FileInfo) bool {
	if info == nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	name := strings.ToLower(info.Name())
	return strings.HasSuffix(name, ".zip") && !strings.HasSuffix(name, ".tmp.zip")
}

func FindSave(name string) (*Save, error) {
	if err := ValidatePathElement(name); err != nil {
		return nil, fmt.Errorf("invalid save name: %w", err)
	}
	saves, err := ListSaves()
	if err != nil {
		return nil, fmt.Errorf("error listing saves: %v", err)
	}

	for _, save := range saves {
		if save.Name == name {
			return &save, nil
		}
	}

	return nil, errors.New("save not found")
}

func (s *Save) Remove() error {
	if err := ValidatePathElement(s.Name); err != nil {
		return fmt.Errorf("invalid save name: %w", err)
	}
	config := bootstrap.GetConfig()
	return os.Remove(filepath.Join(config.FactorioSavesDir, s.Name))
}

// Create savefiles for Factorio
func CreateSave(filePath string) (string, error) {
	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		log.Printf("Error in creating Factorio save: %s", err)
		return "", err
	}

	args := []string{"--create", filePath}
	config := bootstrap.GetConfig()
	cmdOutput, err := exec.Command(config.FactorioBinary, args...).Output()
	if err != nil {
		log.Printf("Error in creating Factorio save: %s", err)
		log.Println(string(cmdOutput))
		return "", err
	}

	result := string(cmdOutput)

	return result, nil
}

func GetLatestSave() (save Save, err error) {
	saves, err := ListSaves()
	if err != nil {
		return Save{}, err
	}
	if len(saves) == 0 {
		return Save{}, errors.New("no usable save file found")
	}

	latest := saves[0]
	for _, candidate := range saves[1:] {
		if latest.LastMod.Before(candidate.LastMod) {
			latest = candidate
		}
	}
	return latest, nil
}
