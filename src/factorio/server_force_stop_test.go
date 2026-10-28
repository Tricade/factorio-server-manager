package factorio

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForceStopAlreadyStoppedDoesNotPanicOrReserveRuntime(t *testing.T) {
	server := &Server{}
	require.NotPanics(t, func() { require.NoError(t, server.Kill()) })
	require.False(t, server.IsBusy())
}

func TestForceStopWithoutPublishedProcessReturnsError(t *testing.T) {
	for _, command := range []*exec.Cmd{nil, {}} {
		server := &Server{starting: true, Cmd: command}
		require.NotPanics(t, func() { require.Error(t, server.Kill()) })
		require.True(t, server.IsBusy(), "a failed force-stop cannot release an in-flight startup")
		require.False(t, server.IsStopping(), "a rejected signal must not reserve shutdown")
	}
}
