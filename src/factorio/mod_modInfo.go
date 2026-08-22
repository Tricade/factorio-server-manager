package factorio

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/lockfile"
)

type ModInfoList struct {
	Mods        []ModInfo `json:"mods"`
	Destination string    `json:"-"`
}
type ModInfo struct {
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Title           string   `json:"title"`
	Author          string   `json:"author"`
	FileName        string   `json:"file_name"`
	FactorioVersion Version  `json:"factorio_version"`
	Dependencies    []string `json:"dependencies"`
	Compatibility   bool     `json:"compatibility"`
}

func newModInfoList(destination string) (ModInfoList, error) {
	var err error
	modInfoList := ModInfoList{
		Destination: destination,
	}

	err = modInfoList.listInstalledMods()
	if err != nil {
		log.Printf("ModInfoList ... error listing installed Mods: %s", err)
		return modInfoList, err
	}

	return modInfoList, nil
}

func (modInfoList *ModInfoList) listInstalledMods() error {
	var err error
	modInfoList.Mods = nil

	err = filepath.Walk(modInfoList.Destination, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}
		if !info.IsDir() && filepath.Ext(path) == ".zip" {
			err := FileLock.RLock(path)
			if err != nil && err == lockfile.ErrorAlreadyLocked {
				log.Println(err)
				return nil
			} else if err != nil {
				log.Printf("error locking file: %s", err)
				return err
			}
			zipFile, err := zip.OpenReader(path)
			if err != nil {
				FileLock.RUnlock(path)
				return errors.New("open mod archive " + info.Name() + ": " + err.Error())
			}

			var modInfo ModInfo
			err = modInfo.getModInfo(&zipFile.Reader)
			closeErr := zipFile.Close()
			FileLock.RUnlock(path)
			if err != nil {
				return errors.New("read mod metadata from " + info.Name() + ": " + err.Error())
			}
			if closeErr != nil {
				return closeErr
			}

			modInfo.FileName = info.Name()

			var base Version
			var op string
			for _, dep := range modInfo.Dependencies {
				dep = strings.TrimSpace(dep)
				if dep == "" {
					continue
				}

				// skip optional and incompatible dependencies
				parts := strings.Split(dep, " ")
				if len(parts) > 3 {
					log.Printf("skipping dependency '%s' in '%s': optional dependency or invalid format\n", dep, modInfo.Name)
					continue
				}
				if parts[0] != "base" {
					continue
				}
				if len(parts) == 1 {
					base = modInfo.FactorioVersion
					op = ">="
					continue
				}

				op = parts[1]

				if err := base.UnmarshalText([]byte(parts[2])); err != nil {
					log.Printf("skipping dependency '%s' in '%s': %v\n", dep, modInfo.Name, err)
					continue
				}

				break
			}

			server := GetFactorioServer()

			// check both the factorio-version and the base mod dependency
			modInfo.Compatibility = server.Version.GEC(modInfo.FactorioVersion)
			if modInfo.Compatibility && !base.Equals(NilVersion) {
				modInfo.Compatibility = server.Version.Compatible(base, op)
			}

			modInfoList.Mods = append(modInfoList.Mods, modInfo)
		}

		return nil
	})

	if err != nil {
		log.Printf("error while walking over the given dir: %s", err)
		return err
	}

	return nil
}

func (modInfoList *ModInfoList) deleteMod(modName string) error {
	var err error

	//search for mod, that should be deleted
	for _, mod := range modInfoList.Mods {
		if mod.Name == modName {
			filePath := filepath.Join(modInfoList.Destination, mod.FileName)

			FileLock.LockW(filePath)
			//delete mod
			err = os.Remove(filePath)
			FileLock.Unlock(filePath)
			if err != nil {
				log.Printf("ModInfoList ... error when deleting mod: %s", err)
				return err
			}

			//reload mod-list
			err = modInfoList.listInstalledMods()
			if err != nil {
				log.Printf("ModInfoList ... error while refreshing installedModList: %s", err)
				return err
			}

			return nil
		}
	}

	log.Printf("the mod-file for mod %s doesn't exist!", modName)
	return errors.New("the mod-file for mod " + modName + " doesn't exist!")
}

func (modInfo *ModInfo) getModInfo(reader *zip.Reader) error {
	for _, singleFile := range reader.File {
		if singleFile.FileInfo().Name() == "info.json" {
			//interpret info.json
			rc, err := singleFile.Open()

			if err != nil {
				return err
			}

			byteArray, err := io.ReadAll(io.LimitReader(rc, 1<<20))
			if err != nil {
				rc.Close()
				return err
			}
			err = rc.Close()
			if err != nil {
				log.Printf("Error closing singleFile: %s", err)
				return err
			}

			err = json.Unmarshal(byteArray, modInfo)
			if err != nil {
				return errors.New("invalid info.json: " + err.Error())
			}
			if modInfo.Name == "" || modInfo.Version == "" {
				return errors.New("info.json must contain name and version")
			}

			return nil
		}
	}

	return errors.New("info.json not found in zip-file")
}

func (modInfoList *ModInfoList) createMod(modName string, fileName string, modFile io.Reader) error {
	if err := ValidatePathElement(fileName); err != nil {
		return errors.New("invalid mod filename: " + err.Error())
	}
	if !strings.EqualFold(filepath.Ext(fileName), ".zip") {
		return errors.New("mod filename must end in .zip")
	}

	filePath := filepath.Join(modInfoList.Destination, fileName)
	newFile, err := os.CreateTemp(modInfoList.Destination, ".mod-upload-*")
	if err != nil {
		log.Printf("error creating temporary mod file for %s: %s", fileName, err)
		return err
	}
	temporaryPath := newFile.Name()
	defer os.Remove(temporaryPath)

	_, err = io.Copy(newFile, modFile)
	if err != nil {
		newFile.Close()
		log.Printf("error on copying file to disk: %s", err)
		return err
	}

	err = newFile.Close()
	if err != nil {
		log.Printf("error on closing new created zip-file: %s", err)
		return err
	}
	if err := os.Chmod(temporaryPath, 0644); err != nil {
		return err
	}

	FileLock.LockW(filePath)
	err = os.Rename(temporaryPath, filePath)
	FileLock.Unlock(filePath)
	if err != nil {
		return err
	}

	//reload the list
	err = modInfoList.listInstalledMods()
	if err != nil {
		log.Printf("error on listing mod-infos: %s", err)
		return err
	}

	return nil
}
