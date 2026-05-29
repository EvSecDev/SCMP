package helpers

import (
	"bytes"
	"strings"
)

func ContainsAll(text string, tokens []string) (allTokensInText bool) {
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			allTokensInText = false
			return
		}
	}
	allTokensInText = true
	return
}

// Takes lines of byte slices and extracts first and second column data.
// Discards line when line contains more/less than 2 fields (space separated)
func DualFieldsIntoMap(lines [][]byte) (allFields map[string]string) {
	allFields = make(map[string]string)
	for _, line := range lines {
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			continue
		}

		key := string(fields[0])
		val := string(fields[1])
		allFields[key] = val
	}
	return
}

// Removes timestamps from semantic version strings
func ShortVersion(version string) (short string) {
	if !strings.Contains(version, "-") {
		// No extra to remove
		short = version
		return
	}

	sections := strings.Split(version, "-")
	if len(sections) != 3 {
		// Already short-ish or unsupported long version
		short = version
		return
	}

	// Omit middle
	short = sections[0] + sections[2]
	return
}
