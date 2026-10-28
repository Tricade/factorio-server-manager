package lockfile

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPathResolutionErrorsReturnWithoutCreatingLocksOrRetrying(t *testing.T) {
	lock := NewLock()
	resolutionError := errors.New("working directory unavailable")
	originalResolver := resolveAbsolutePath
	resolveAbsolutePath = func(string) (string, error) { return "", resolutionError }
	t.Cleanup(func() { resolveAbsolutePath = originalResolver })

	require.ErrorIs(t, lock.Lock("relative.zip"), resolutionError)
	require.ErrorIs(t, lock.RLock("relative.zip"), resolutionError)
	require.ErrorIs(t, lock.LockW("relative.zip"), resolutionError)
	require.ErrorIs(t, lock.RLockW("relative.zip"), resolutionError)
	require.Empty(t, lock.Locks)
}

func TestFileLockCanonicalPath(t *testing.T) {
	lock := NewLock()
	directory := t.TempDir()
	path := filepath.Join(directory, "save.zip")
	alias := directory + string(filepath.Separator) + "unused" + string(filepath.Separator) + ".." + string(filepath.Separator) + "save.zip"
	require.NoError(t, lock.Lock(path))
	require.ErrorIs(t, lock.RLock(alias), ErrorAlreadyLocked)
	require.ErrorIs(t, lock.Lock(alias), ErrorAlreadyLocked)
	require.NoError(t, lock.Unlock(alias))
	require.NoError(t, lock.RLock(path))
	require.NoError(t, lock.RUnlock(path))
}

func TestFileLockReleasesIdleEntries(t *testing.T) {
	lock := NewLock()
	path := filepath.Join(t.TempDir(), "save.zip")
	require.NoError(t, lock.RLock(path))
	require.NoError(t, lock.RLock(path))
	require.ErrorIs(t, lock.Lock(path), ErrorAlreadyLocked)
	require.NoError(t, lock.RUnlock(path))
	require.ErrorIs(t, lock.Lock(path), ErrorAlreadyLocked)
	require.NoError(t, lock.RUnlock(path))
	require.Empty(t, lock.Locks)
	require.NoError(t, lock.Lock(path))
	require.ErrorIs(t, lock.RLock(path), ErrorAlreadyLocked)
	require.NoError(t, lock.Unlock(path))
	require.Empty(t, lock.Locks)
	require.Error(t, lock.Unlock(path))
	require.Error(t, lock.RUnlock(path))
	require.Empty(t, lock.Locks)
}

func TestFileLockConcurrentReaders(t *testing.T) {
	lock := NewLock()
	path := filepath.Join(t.TempDir(), strings.Repeat("s", 10)+".zip")
	var workers sync.WaitGroup
	for index := 0; index < 20; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for iteration := 0; iteration < 50; iteration++ {
				if err := lock.RLock(path); err != nil {
					t.Error(err)
					return
				}
				if err := lock.RUnlock(path); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	workers.Wait()
	require.Empty(t, lock.Locks)
}

func TestBlockingLockHelpersRetryContention(t *testing.T) {
	for _, test := range []struct {
		name   string
		waiter func(*FileLock, string) error
	}{
		{name: "writer", waiter: (*FileLock).LockW},
		{name: "reader", waiter: (*FileLock).RLockW},
	} {
		t.Run(test.name, func(t *testing.T) {
			lock := NewLock()
			path := filepath.Join(t.TempDir(), "save.zip")
			require.NoError(t, lock.Lock(path))

			result := make(chan error, 1)
			go func() { result <- test.waiter(lock, path) }()
			time.Sleep(25 * time.Millisecond)
			require.NoError(t, lock.Unlock(path))

			select {
			case err := <-result:
				require.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("lock waiter did not retry after contention was released")
			}
			if test.name == "writer" {
				require.NoError(t, lock.Unlock(path))
			} else {
				require.NoError(t, lock.RUnlock(path))
			}
		})
	}
}
