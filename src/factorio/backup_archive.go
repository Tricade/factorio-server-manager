package factorio

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	backupMaxEntries              = 30000
	backupMaxExpandedBytes uint64 = 16 << 30
	backupMaxJSONBytes            = 2 << 20
	backupMaxSettingsBytes        = 16 << 20
)

var ErrInvalidBackup = errors.New("invalid or unsupported backup archive")

type backupCopyBudget struct {
	bytes   uint64
	entries int
}

func (budget *backupCopyBudget) copy(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) > backupMaxExpandedBytes-budget.bytes || budget.entries >= backupMaxEntries {
		return ErrInvalidBackup
	}
	budget.bytes += uint64(info.Size())
	budget.entries++
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(output, io.LimitReader(input, info.Size()+1))
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if n != info.Size() {
		return ErrInvalidBackup
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chtimes(destination, info.ModTime(), info.ModTime())
}

// Backups have their own format; importing one never migrates the installed
// profile manifest. Untrusted archives are expanded only into private staging.
func backupEntryName(name string) error {
	if len(name) > 1024 || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return ErrInvalidBackup
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if len(part) > 255 || ValidatePathElement(part) != nil {
			return ErrInvalidBackup
		}
	}
	return nil
}

func inspectBackupZip(reader *zip.Reader, allowed func(string) bool) error {
	if len(reader.File) == 0 || len(reader.File) > backupMaxEntries {
		return ErrInvalidBackup
	}
	var total uint64
	seen := map[string]bool{}
	files := map[string]bool{}
	for _, entry := range reader.File {
		name := strings.TrimSuffix(entry.Name, "/")
		key := strings.ToLower(name)
		if backupEntryName(entry.Name) != nil || seen[key] || (!entry.Mode().IsRegular() && !entry.Mode().IsDir()) {
			return ErrInvalidBackup
		}
		seen[key] = true
		if !entry.FileInfo().IsDir() {
			if allowed != nil && !allowed(name) {
				return ErrInvalidBackup
			}
			files[key] = true
			if allowed != nil && ((strings.HasSuffix(name, ".json") && entry.UncompressedSize64 > backupMaxJSONBytes) || (strings.HasSuffix(name, "mod-settings.dat") && entry.UncompressedSize64 > backupMaxSettingsBytes)) {
				return ErrInvalidBackup
			}
		}
		if entry.UncompressedSize64 > backupMaxExpandedBytes-total {
			return ErrInvalidBackup
		}
		total += entry.UncompressedSize64
	}
	for key := range seen {
		for parent := filepath.ToSlash(filepath.Dir(key)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if files[parent] {
				return ErrInvalidBackup
			}
		}
	}
	return nil
}

func extractBackup(reader *zip.Reader, root string, allowed func(string) bool) error {
	if err := inspectBackupZip(reader, allowed); err != nil {
		return err
	}
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		destination := filepath.Join(root, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return ErrInvalidBackup
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return err
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, int64(entry.UncompressedSize64)+1))
		closeErr := errors.Join(input.Close(), output.Close())
		if copyErr != nil || uint64(n) != entry.UncompressedSize64 {
			return ErrInvalidBackup
		}
		if closeErr != nil {
			return closeErr
		}
		if !entry.Modified.IsZero() {
			if err := os.Chtimes(destination, entry.Modified, entry.Modified); err != nil {
				return err
			}
		}
	}
	return nil
}

func readBackupJSON(path string, target interface{}) error {
	data, err := readBackupFile(path, backupMaxJSONBytes)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, target) != nil {
		return ErrInvalidBackup
	}
	return nil
}

func readBackupFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrInvalidBackup
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if int64(len(data)) > limit {
		return nil, ErrInvalidBackup
	}
	return data, err
}

func writeBackupJSON(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > backupMaxJSONBytes {
		return ErrInvalidBackup
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func zipBackupDirectory(root string) (string, error) {
	output, err := os.CreateTemp("", "fsm-backup-*.zip")
	if err != nil {
		return "", err
	}
	name := output.Name()
	success := false
	defer func() {
		output.Close()
		if !success {
			os.Remove(name)
		}
	}()
	writer := zip.NewWriter(output)
	var size uint64
	count := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return ErrInvalidBackup
		}
		count++
		if count > backupMaxEntries || uint64(info.Size()) > backupMaxExpandedBytes-size {
			return ErrInvalidBackup
		}
		size += uint64(info.Size())
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if err := backupEntryName(relative); err != nil {
			return err
		}
		header := &zip.FileHeader{Name: relative, Method: zip.Deflate, Modified: info.ModTime()}
		// Saves and mods are already compressed. Avoid recompressing gigabytes.
		if strings.HasSuffix(relative, ".zip") {
			header.Method = zip.Store
		}
		header.SetMode(0600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		n, err := io.Copy(entry, io.LimitReader(input, info.Size()+1))
		if err == nil && n != info.Size() {
			err = fmt.Errorf("backup source changed during export")
		}
		return err
	})
	closeErr := errors.Join(writer.Close(), output.Close())
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	success = true
	return name, nil
}
