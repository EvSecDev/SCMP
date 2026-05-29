package helpers

import (
	"bytes"
	"fmt"
)

func GetProgVersion(fileContents []byte, versionVariableName string) (progVersion string, err error) {
	lines := bytes.Split(fileContents, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte(versionVariableName)) {
			continue
		}

		keyVal := bytes.Split(line, []byte("="))
		if len(keyVal) != 2 {
			err = fmt.Errorf("expected constant '%s' to have value, but found none in line '%s'", versionVariableName, line)
			return
		}
		value := bytes.TrimSpace(keyVal[1])
		rawValue := bytes.Trim(value, `"`)
		if len(rawValue) == 0 {
			err = fmt.Errorf("expected constant '%s' to have non-empty string value in line '%s'", versionVariableName, line)
			return
		}

		progVersion = string(rawValue)
	}
	if progVersion == "" {
		err = fmt.Errorf("could not find constant '%s' in file", versionVariableName)
		return
	}
	return
}
