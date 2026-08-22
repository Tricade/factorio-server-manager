package factorio

import (
	"errors"
	"strings"
	"unicode"
)

// ValidatePathElement accepts a single user-visible file or directory name,
// never a relative or absolute path.
func ValidatePathElement(name string) error {
	if name == "" || name == "." || name == ".." {
		return errors.New("name must not be empty or relative")
	}
	if strings.ContainsAny(name, `/\\`) {
		return errors.New("name must not contain path separators")
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errors.New("name must not contain control characters")
	}
	return nil
}
