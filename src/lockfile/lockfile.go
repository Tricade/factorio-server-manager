package lockfile

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileLock struct {
	m     sync.Mutex
	Locks map[string]Lock
}

type Lock struct {
	Read  int
	Write int
}

var ErrorLocking error = errors.New("error locking file")
var ErrorAlreadyLocked error = errors.New("file already locked by another routine")

func NewLock() *FileLock {
	lock := &FileLock{
		Locks: make(map[string]Lock),
	}
	return lock
}

var resolveAbsolutePath = makeAbsolutePath

func makeAbsolutePath(target string) (string, error) {
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory for relative file lock path: %w", err)
	}

	return filepath.Join(wd, target), nil
}

func (fl *FileLock) Lock(filePath string) error {
	filePath, err := resolveAbsolutePath(filePath)
	if err != nil {
		return err
	}

	fl.m.Lock()
	defer fl.m.Unlock()

	if fl.Locks[filePath].Read == 0 && fl.Locks[filePath].Write == 0 {
		lock := fl.Locks[filePath]
		lock.Write = 1
		fl.Locks[filePath] = lock
	} else {
		return ErrorAlreadyLocked
	}
	return nil
}

func (fl *FileLock) Unlock(filePath string) error {
	filePath, err := resolveAbsolutePath(filePath)
	if err != nil {
		return err
	}

	fl.m.Lock()
	defer fl.m.Unlock()

	lock := fl.Locks[filePath]
	if lock.Read == 0 && lock.Write == 1 {
		delete(fl.Locks, filePath)
	} else {
		return ErrorAlreadyLocked
	}
	return nil
}

func (fl *FileLock) RLock(filePath string) error {
	filePath, err := resolveAbsolutePath(filePath)
	if err != nil {
		return err
	}

	fl.m.Lock()
	defer fl.m.Unlock()

	lock := fl.Locks[filePath]
	if lock.Write == 0 {
		lock.Read++
		fl.Locks[filePath] = lock
	} else {
		return ErrorAlreadyLocked
	}
	return nil
}

func (fl *FileLock) RUnlock(filePath string) error {
	filePath, err := resolveAbsolutePath(filePath)
	if err != nil {
		return err
	}

	fl.m.Lock()
	defer fl.m.Unlock()

	lock := fl.Locks[filePath]
	if lock.Read > 0 && lock.Write == 0 {
		lock.Read--
		if lock.Read == 0 {
			delete(fl.Locks, filePath)
		} else {
			fl.Locks[filePath] = lock
		}
	} else {
		return ErrorAlreadyLocked
	}
	return nil
}

func (fl *FileLock) LockW(filePath string) error {
	for {
		err := fl.Lock(filePath)
		if errors.Is(err, ErrorAlreadyLocked) {
			time.Sleep(time.Second * 2)
			log.Println("file locked wait two seconds to access write-lock")
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
}

func (fl *FileLock) RLockW(filePath string) error {
	for {
		err := fl.RLock(filePath)
		if errors.Is(err, ErrorAlreadyLocked) {
			time.Sleep(time.Second * 2)
			log.Println("file locked ... wait two seconds to try to access read-lock")
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
}
