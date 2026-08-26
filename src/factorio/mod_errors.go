package factorio

import "errors"

var (
	errModPortalReleaseUnavailable = errors.New("requested mod release is unavailable")
	errModArchiveIdentityMismatch  = errors.New("downloaded mod archive identifies a different mod")
)
