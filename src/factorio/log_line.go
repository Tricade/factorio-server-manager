package factorio

import (
	"bufio"
	"errors"
	"strings"
)

const omittedLogLine = "[log line omitted: exceeds the size limit]"

// readBoundedLogLine drains oversized records without retaining their contents.
// Unlike Scanner's token-too-long failure, it can resume at the next line so a
// chatty mod cannot permanently stop pipe draining or hide later status lines.
func readBoundedLogLine(reader *bufio.Reader, maximum int) (string, error) {
	var contents []byte
	oversized := false
	for {
		fragment, err := reader.ReadSlice('\n')
		if !oversized {
			if len(contents)+len(fragment) > maximum {
				contents = nil
				oversized = true
			} else {
				contents = append(contents, fragment...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if oversized {
			return omittedLogLine, err
		}
		return strings.TrimSuffix(strings.TrimSuffix(string(contents), "\n"), "\r"), err
	}
}
