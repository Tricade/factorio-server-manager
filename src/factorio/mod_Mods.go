package factorio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenFactorioServerManager/factorio-server-manager/lockfile"
)

type Mods struct {
	ModSimpleList ModSimpleList `json:"mod_simple_list"`
	ModInfoList   ModInfoList   `json:"mod_info_list"`
}
type ModsResult struct {
	ModInfo
	Enabled bool `json:"enabled"`
}
type ModsResultList struct {
	ModsResult []ModsResult `json:"mods"`
}

var FileLock = lockfile.NewLock()

const maximumModPortalArchiveBytes int64 = 512 * 1024 * 1024

func NewMods(destination string) (Mods, error) {
	var err error
	var mods Mods

	mods.ModSimpleList, err = newModSimpleList(destination)
	if err != nil {
		log.Printf("error on creating newModSimpleList: %s", err)
		return mods, err
	}

	mods.ModInfoList, err = newModInfoList(destination)
	if err != nil {
		log.Printf("error on creating newModInfoList: %s", err)
		return mods, err
	}

	return mods, nil
}

func (mods *Mods) ListInstalledMods() ModsResultList {
	result := ModsResultList{make([]ModsResult, 0)}

	for _, modInfo := range mods.ModInfoList.Mods {
		var modsResult ModsResult
		modsResult.Name = modInfo.Name
		modsResult.FileName = modInfo.FileName
		modsResult.Author = modInfo.Author
		modsResult.Title = modInfo.Title
		modsResult.Version = modInfo.Version
		modsResult.FactorioVersion = modInfo.FactorioVersion
		modsResult.Compatibility = modInfo.Compatibility

		for _, simpleMod := range mods.ModSimpleList.Mods {
			if simpleMod.Name == modsResult.Name {
				modsResult.Enabled = simpleMod.Enabled
				break
			}
		}

		result.ModsResult = append(result.ModsResult, modsResult)
	}

	return result
}

func (mods *Mods) DeleteMod(modName string) error {
	unlockMutation, err := lockActiveModMutation(mods.ModInfoList.Destination)
	if err != nil {
		return err
	}
	defer unlockMutation()

	err = mods.ModInfoList.deleteMod(modName)
	if err != nil {
		log.Printf("error when deleting mod in ModInfoList: %s", err)
		return err
	}

	err = mods.ModSimpleList.deleteMod(modName)
	if err != nil {
		log.Printf("error when deleting mod in ModSimpleList: %s", err)
		return err
	}

	return nil
}

func (mods *Mods) createMod(modName string, fileName string, fileRc io.Reader) error {
	return mods.createModWithLimit(modName, fileName, fileRc, 0)
}

func (mods *Mods) createModWithLimit(modName string, fileName string, fileRc io.Reader, maximumBytes int64) error {
	if !filepath.IsLocal(modName) {
		return errors.New("invalid mod name")
	}
	if !filepath.IsLocal(fileName) {
		return errors.New("invalid mod filename")
	}
	if err := ValidatePathElement(modName); err != nil {
		return fmt.Errorf("invalid mod name: %w", err)
	}
	if err := ValidatePathElement(fileName); err != nil {
		return fmt.Errorf("invalid mod filename: %w", err)
	}
	modName = filepath.Base(modName)
	fileName = filepath.Base(fileName)

	existingFiles := make([]string, 0)
	for _, mod := range mods.ModInfoList.Mods {
		if mod.Name == modName {
			existingFiles = append(existingFiles, mod.FileName)
		}
	}

	// Stage, validate and atomically activate the new archive before removing
	// any previous version. A failed download can therefore never erase the
	// working copy it was meant to replace.
	err := mods.ModInfoList.createModWithLimit(modName, fileName, fileRc, maximumBytes)
	if err != nil {
		log.Printf("error on creating mod-file: %s", err)
		return err
	}

	// also add to ModSimpleList if not there yet
	if !mods.ModSimpleList.CheckModExists(modName) {
		err = mods.ModSimpleList.createMod(modName)
		if err != nil {
			log.Printf("error creating mod in modSimpleList: %s", err)
			return err
		}
	}

	for _, previousFile := range existingFiles {
		if previousFile == fileName {
			continue
		}
		previousPath := filepath.Join(mods.ModInfoList.Destination, previousFile)
		FileLock.LockW(previousPath)
		removeErr := os.Remove(previousPath)
		FileLock.Unlock(previousPath)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("remove previous mod archive %s: %w", previousFile, removeErr)
		}
	}
	if err := mods.ModInfoList.listInstalledMods(); err != nil {
		return fmt.Errorf("refresh installed mods: %w", err)
	}

	return nil
}

func (mods *Mods) DownloadMod(downloadPath string, filename string, modId string) error {
	if !filepath.IsLocal(filename) {
		return errors.New("invalid mod filename")
	}
	if !filepath.IsLocal(modId) {
		return errors.New("invalid mod name")
	}
	if err := ValidatePathElement(filename); err != nil {
		return fmt.Errorf("invalid mod filename: %w", err)
	}
	if err := ValidatePathElement(modId); err != nil {
		return fmt.Errorf("invalid mod name: %w", err)
	}
	filename = filepath.Base(filename)
	modId = filepath.Base(modId)

	var credentials Credentials
	status, err := credentials.Load()
	if err != nil {
		log.Printf("error loading credentials: %s", err)
		return err
	}
	if status == false {
		log.Printf("error: credentials are invalid")
		return errors.New("error: credentials are invalid")
	}

	completeURL, err := authenticatedModDownloadURL(downloadPath, credentials)
	if err != nil {
		return err
	}

	response, err := modPortalHTTPClient.Get(completeURL)
	if err != nil {
		log.Printf("error on downloading mod: %s", err)
		return err
	}

	log.Printf("download complete\n StatusCode: %d\n Status: %s", response.StatusCode, response.Status)

	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return fmt.Errorf("%w: mod portal download returned HTTP %d", errModPortalReleaseUnavailable, response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		log.Printf("StatusCode: %d", response.StatusCode)
		body, _ := readBoundedBody(response.Body, maximumModPortalErrorBodyBytes)
		return fmt.Errorf("mod portal download returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if response.ContentLength > maximumModPortalArchiveBytes {
		return fmt.Errorf("mod archive exceeds %d bytes", maximumModPortalArchiveBytes)
	}

	unlockMutation, err := lockActiveModMutation(mods.ModInfoList.Destination)
	if err != nil {
		return err
	}
	defer unlockMutation()
	err = mods.createModWithLimit(modId, filename, response.Body, maximumModPortalArchiveBytes)
	if err != nil {
		log.Printf("error when creating Mod: %s", err)
		return err
	}

	log.Printf("completed copying the response.Body")

	//done everything is made inside the createMod

	return nil
}

func authenticatedModDownloadURL(downloadPath string, credentials Credentials) (string, error) {
	reference, err := url.Parse(downloadPath)
	if err != nil {
		return "", fmt.Errorf("parse mod download path: %w", err)
	}
	if reference.IsAbs() || reference.Host != "" || reference.RawQuery != "" || reference.Fragment != "" || !strings.HasPrefix(reference.Path, "/download/") {
		return "", errors.New("invalid mod portal download path")
	}
	base, err := url.Parse(modPortalBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", errors.New("invalid mod portal base URL")
	}
	target := base.ResolveReference(&url.URL{Path: reference.Path, RawPath: reference.RawPath})
	query := target.Query()
	query.Set("username", credentials.Username)
	query.Set("token", credentials.Userkey)
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (mods *Mods) UploadMod(file multipart.File, header *multipart.FileHeader) error {
	if !strings.EqualFold(filepath.Ext(header.Filename), ".zip") {
		log.Print("The uploaded file wasn't a zip-file")
		return errors.New("the uploaded file wasn't a zip-file")
	}

	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("measure uploaded mod: %w", err)
	}
	if size <= 0 {
		return errors.New("uploaded mod archive is empty")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind uploaded mod: %w", err)
	}
	zipReader, err := zip.NewReader(file, size)
	if err != nil {
		log.Printf("Uploaded file could not put into zip.Reader: %s", err)
		return err
	}

	var modInfo ModInfo
	err = modInfo.getModInfo(zipReader)
	if err != nil {
		log.Printf("Error in getModInfo: %s", err)
		return err
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind validated mod: %w", err)
	}
	unlockMutation, err := lockActiveModMutation(mods.ModInfoList.Destination)
	if err != nil {
		return err
	}
	defer unlockMutation()
	err = mods.createMod(modInfo.Name, header.Filename, file)
	if err != nil {
		log.Printf("error on creating Mod: %s", err)
		return err
	}

	return nil
}

func lockActiveModMutation(destination string) (func(), error) {
	activeMods := profileActiveDirectories()["mods"]
	if activeMods == "" || filepath.Clean(destination) != filepath.Clean(activeMods) {
		return func() {}, nil
	}
	serverLifecycleMutex.Lock()
	if GetFactorioServer().IsBusy() {
		serverLifecycleMutex.Unlock()
		return nil, ErrServerActive
	}
	return serverLifecycleMutex.Unlock, nil
}

func (mods *Mods) UpdateMod(modName string, url string, filename string) error {
	var err error

	err = mods.DownloadMod(url, filename, modName)
	if err != nil {
		log.Printf("updateMod ... error when downloading the new Mod: %s", err)
		return err
	}

	return nil
}

// SaveModConfiguration stages a complete profile-scoped mod configuration
// before replacing it. The internal lifecycle recheck closes the race between
// ServerOffMiddleware and an atomic server start claim.
func SaveModConfiguration(destination, filename string, source io.Reader, maximumBytes int64) error {
	if filename != "mod-list.json" && filename != "mod-settings.dat" {
		return errors.New("unsupported mod configuration file")
	}
	var contents bytes.Buffer
	if err := copyWithHardLimit(&contents, source, maximumBytes); err != nil {
		return err
	}
	if filename == "mod-list.json" {
		var list ModSimpleList
		if err := json.Unmarshal(contents.Bytes(), &list); err != nil {
			return fmt.Errorf("decode mod-list.json: %w", err)
		}
		if list.Mods == nil {
			return errors.New("mod-list.json must contain a mods array")
		}
	}

	serverLifecycleMutex.Lock()
	defer serverLifecycleMutex.Unlock()
	if GetFactorioServer().IsBusy() {
		return ErrServerActive
	}
	return writeFileAtomically(filepath.Join(destination, filename), contents.Bytes(), 0644)
}
