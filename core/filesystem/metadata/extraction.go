// Package for parsing metadata headers
package metadata

import (
	"encoding/json"
	"fmt"
	"scmp/core/filesystem"
	"strings"
)

// Function to extract metadata JSON from file contents
func Extract(fileContents string) (metadata filesystem.MetaHeader, contentSection []byte, err error) {
	// Do not allow carriage returns
	fileContents = strings.ReplaceAll(fileContents, "\r", "")

	// Handle comments around metadata header
	fileContents = strings.Replace(fileContents, "/*"+filesystem.MetaDelimiter, filesystem.MetaDelimiter, 1)
	fileContents = strings.Replace(fileContents, filesystem.MetaDelimiter+"*/", filesystem.MetaDelimiter, 1)
	fileContents = strings.Replace(fileContents, "<!--"+filesystem.MetaDelimiter, filesystem.MetaDelimiter, 1)
	fileContents = strings.Replace(fileContents, filesystem.MetaDelimiter+"-->", filesystem.MetaDelimiter, 1)
	fileContents = strings.Replace(fileContents, "#"+filesystem.MetaDelimiter, filesystem.MetaDelimiter, 2)
	fileContents = strings.Replace(fileContents, ";"+filesystem.MetaDelimiter, filesystem.MetaDelimiter, 2)
	fileContents = strings.Replace(fileContents, "//"+filesystem.MetaDelimiter, filesystem.MetaDelimiter, 2)

	// Find the start and end of the metadata section
	startIndex := strings.Index(fileContents, filesystem.MetaDelimiter)
	if startIndex == -1 {
		err = fmt.Errorf("json start delimiter missing")
		return
	}
	startIndex += len(filesystem.MetaDelimiter)

	endIndex := strings.Index(fileContents[startIndex:], filesystem.MetaDelimiter)
	if endIndex == -1 {
		testEndIndex := strings.Index(fileContents[startIndex:], filesystem.MetaDelimiter)
		if testEndIndex == -1 {
			err = fmt.Errorf("json end delimiter missing")
			return
		}
		err = fmt.Errorf("json end delimiter missing")
		return
	}
	endIndex += startIndex

	// Extract the metadata section
	metadataSection := fileContents[startIndex:endIndex]

	// Handle commented out metadata lines
	metadataSection = strings.ReplaceAll(metadataSection, "\n#", "\n")
	metadataSection = strings.ReplaceAll(metadataSection, "\n//", "\n")
	metadataSection = strings.ReplaceAll(metadataSection, "\n;", "\n")

	err = json.Unmarshal([]byte(metadataSection), &metadata)
	if err != nil {
		err = fmt.Errorf("invalid metadata header: %w", err)
		return
	}

	// Extract the content section
	remainingContent := fileContents[:startIndex-len(filesystem.MetaDelimiter)] + fileContents[endIndex+len(filesystem.MetaDelimiter):]
	remainingContent = strings.TrimPrefix(remainingContent, "\n")

	contentSection = []byte(remainingContent)

	return
}
