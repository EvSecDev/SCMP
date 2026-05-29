// Package for standalone text or data parsing logic
package parsing

import (
	"fmt"
	"math"
	"strings"
)

// Takes a raw byte integer and converts it to a human-readable format with appropriate units
func FormatBytes(bytes int) (bytesWithUnits string) {
	units := []string{"Bytes", "KiB", "MiB", "GiB", "TiB", "PiB"}
	if bytes == 0 {
		return fmt.Sprintf("0 %s", units[0])
	}

	// Determine the appropriate unit
	unitIndex := int(math.Floor(math.Log(float64(bytes)) / math.Log(1024)))
	if unitIndex >= len(units) {
		unitIndex = len(units) - 1
	}

	// Calculate the value in the appropriate unit
	value := float64(bytes) / math.Pow(1024, float64(unitIndex))

	// Return the formatted string
	bytesWithUnits = fmt.Sprintf("%.2f %s", value, units[unitIndex])
	return
}

// Determines which file types in the commit are allowed to be deployed
// Marks file type based on mode
func DetermineFileType(fileMode string) (fileType string) {
	// Set type of file in commit - skip unsupported
	switch fileMode {
	case "0100644":
		// Text file
		fileType = "regular"
	case "0100755":
		// Executable file - treated same as regular
		fileType = "regular"
	case "0120000":
		// Special - links
		fileType = "unsupported"
	case "0040000":
		// Directory
		fileType = "unsupported"
	case "0160000":
		// Git submodule
		fileType = "unsupported"
	case "0100664":
		// Deprecated
		fileType = "unsupported"
	case "0":
		// Empty (no file)
		fileType = "unsupported"
	default:
		// Unknown - don't process
		fileType = "unsupported"
	}

	return
}

// Divides up CLI command arguments into separate strings respecting single and double quotes
func HandleQuotedArgs(rawArguments string) (distinctArguments []string, err error) {
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escapeNext := false

	for pos := 0; pos < len(rawArguments); pos++ {
		char := rawArguments[pos]

		if escapeNext {
			current.WriteByte(char)
			escapeNext = false
			continue
		}

		switch char {
		case '\\':
			// Only escape next char if outside single quotes
			if !inSingleQuote {
				escapeNext = true
			} else {
				current.WriteByte(char)
			}
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
				continue // don't include quote char
			}
			current.WriteByte(char)
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
				continue // don't include quote char
			}
			current.WriteByte(char)
		case ' ', '\t':
			if inSingleQuote || inDoubleQuote {
				current.WriteByte(char)
			} else if current.Len() > 0 {
				distinctArguments = append(distinctArguments, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(char)
		}
	}

	if current.Len() > 0 {
		distinctArguments = append(distinctArguments, current.String())
	}

	if inSingleQuote || inDoubleQuote {
		err = fmt.Errorf("unclosed quote in arguments: '%s'", rawArguments)
		return
	}
	if escapeNext {
		err = fmt.Errorf("unfinished escape in arguments: '%s'", rawArguments)
		return
	}

	return
}
