package factorio

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ValidatePathElement accepts a single user-visible file or directory name,
// never a relative or absolute path.
func ValidatePathElement(name string) error {
	if name == "" || name == "." || name == ".." {
		return errors.New("name must not be empty or relative")
	}
	if strings.ContainsAny(name, `<>:"/\\|?*`) {
		return errors.New("name contains a character forbidden in portable filenames")
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errors.New("name must not contain control characters")
	}
	if strings.TrimRight(name, " .") != name {
		return errors.New("name must not end in a space or dot")
	}
	deviceName := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch deviceName {
	case "CON", "PRN", "AUX", "NUL":
		return fmt.Errorf("%s is a reserved Windows device name", deviceName)
	}
	if len(deviceName) == 4 && (strings.HasPrefix(deviceName, "COM") || strings.HasPrefix(deviceName, "LPT")) && deviceName[3] >= '1' && deviceName[3] <= '9' {
		return fmt.Errorf("%s is a reserved Windows device name", deviceName)
	}
	return nil
}
