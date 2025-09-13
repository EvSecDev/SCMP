package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Function to extract metadata JSON from file contents
func extractMetadata(fileContents string) (metadata MetaHeader, contentSection []byte, err error) {
	printMessage(verbosityData, "    Extracting file metadata\n")

	// Do not allow carriage returns
	fileContents = strings.ReplaceAll(fileContents, "\r", "")

	// Handle comments around metadata header
	fileContents = strings.Replace(fileContents, "/*"+metaDelimiter, metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, metaDelimiter+"*/", metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, "<!--"+metaDelimiter, metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, metaDelimiter+"-->", metaDelimiter, 1)
	fileContents = strings.Replace(fileContents, "#"+metaDelimiter, metaDelimiter, 2)
	fileContents = strings.Replace(fileContents, ";"+metaDelimiter, metaDelimiter, 2)
	fileContents = strings.Replace(fileContents, "//"+metaDelimiter, metaDelimiter, 2)

	// Find the start and end of the metadata section
	startIndex := strings.Index(fileContents, metaDelimiter)
	if startIndex == -1 {
		err = fmt.Errorf("json start delimiter missing")
		return
	}
	startIndex += len(metaDelimiter)

	endIndex := strings.Index(fileContents[startIndex:], metaDelimiter)
	if endIndex == -1 {
		testEndIndex := strings.Index(fileContents[startIndex:], metaDelimiter)
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

	printMessage(verbosityData, "    Parsing metadata header JSON\n")

	err = json.Unmarshal([]byte(metadataSection), &metadata)
	if err != nil {
		err = fmt.Errorf("invalid metadata header: %v", err)
		return
	}

	// Extract the content section
	remainingContent := fileContents[:startIndex-len(metaDelimiter)] + fileContents[endIndex+len(metaDelimiter):]
	remainingContent = strings.TrimPrefix(remainingContent, "\n")

	contentSection = []byte(remainingContent)

	return
}
