package transfer

import (
	"strings"
)

func ParseArgs(args []string) (sourceHost, sourcePath, destHost, destPath string) {
	source := args[0]
	if strings.Contains(source, ":") {
		parts := strings.SplitN(source, ":", 2)
		sourceHost = parts[0]
		sourcePath = parts[1]
	} else {
		sourcePath = source
	}

	destination := args[len(args)-1]
	if strings.Contains(destination, ":") {
		parts := strings.SplitN(destination, ":", 2)
		destHost = parts[0]
		destPath = parts[1]
	} else {
		destPath = destination
	}

	return
}
