package factorio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUsableSave(t *testing.T) {
	tests := []struct {
		name string
		size int
		want bool
	}{
		{name: "world.zip", size: 4, want: true},
		{name: "WORLD.ZIP", size: 4, want: true},
		{name: "world.tmp.zip", size: 4, want: false},
		{name: "world.zip", size: 0, want: false},
		{name: "notes.txt", size: 4, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name)
			require.NoError(t, os.WriteFile(path, make([]byte, test.size), 0600))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, test.want, isUsableSave(info))
		})
	}
}
