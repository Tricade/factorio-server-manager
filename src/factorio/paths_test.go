package factorio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePathElement(t *testing.T) {
	for _, name := range []string{"world.zip", "Mein Spielstand.zip", "krastorio_2.0.zip"} {
		assert.NoError(t, ValidatePathElement(name), name)
	}
	for _, name := range []string{"", ".", "..", "../world.zip", `..\\world.zip`, "folder/world.zip", "bad\nname.zip", "CON", "nul.zip", "COM1.txt", "LPT9", "trailing. ", "bad:name.zip"} {
		assert.Error(t, ValidatePathElement(name), name)
	}
}
