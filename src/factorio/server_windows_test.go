package factorio

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWindowsShutdownRestoresConsoleHandlingAfterSignalFailure(t *testing.T) {
	originalSend, originalHandling := sendWindowsControlEvent, setWindowsControlHandling
	t.Cleanup(func() { sendWindowsControlEvent, setWindowsControlHandling = originalSend, originalHandling })
	failure := errors.New("injected console signal failure")
	var calls []bool
	setWindowsControlHandling = func(disabled bool) error { calls = append(calls, disabled); return nil }
	sendWindowsControlEvent = func(int) error { return failure }
	server := &Server{Running: true, Stopping: true, stopSignaled: true}
	require.ErrorIs(t, server.signalStop(), failure)
	require.Equal(t, []bool{true, false}, calls)
	require.False(t, server.IsStopping(), "failed shutdown must be retryable")
}

func TestWindowsShutdownReturnsHandlerRestoreFailure(t *testing.T) {
	originalSend, originalHandling := sendWindowsControlEvent, setWindowsControlHandling
	t.Cleanup(func() { sendWindowsControlEvent, setWindowsControlHandling = originalSend, originalHandling })
	failure := errors.New("injected console handler failure")
	setWindowsControlHandling = func(disabled bool) error {
		if !disabled {
			return failure
		}
		return nil
	}
	sendWindowsControlEvent = func(int) error { return nil }
	server := &Server{Running: true, Stopping: true}
	require.ErrorIs(t, server.signalStop(), failure)
}
