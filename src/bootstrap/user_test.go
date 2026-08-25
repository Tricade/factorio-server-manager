package bootstrap

import (
	"encoding/base64"
	"testing"

	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRandomPassword(t *testing.T) {
	first := GenerateRandomPassword()
	second := GenerateRandomPassword()

	assert.NotEqual(t, first, second)
	decoded, err := base64.RawURLEncoding.DecodeString(first)
	require.NoError(t, err)
	assert.Len(t, decoded, 18)
}

func TestUploadLimitUsesMiB(t *testing.T) {
	var config Config
	config.mapFlags(Flags{FactorioMaxUpload: 20})
	assert.Equal(t, int64(20*1024*1024), config.MaxUploadSize)
}

func TestDefaultUploadLimitSupportsExistingSaves(t *testing.T) {
	var options Flags
	parser := flags.NewParser(&options, flags.Default)
	_, err := parser.ParseArgs(nil)
	require.NoError(t, err)
	assert.Equal(t, int64(512), options.FactorioMaxUpload)
}
