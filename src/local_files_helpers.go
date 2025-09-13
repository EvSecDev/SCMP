package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Handles re-adding actual redirection characters (<<<, >, >>) in raw JSON bytes of metadata header
func unescapeShellRedirectors(rawJSON []byte) (correctedJSON []byte) {
	// Restore <<< (escaped as \u003c\u003c\u003c)
	rawJSON = bytes.ReplaceAll(rawJSON, []byte(`\u003c\u003c\u003c`), []byte("<<<"))

	// Restore >> (escaped as \u003e\u003e)
	rawJSON = bytes.ReplaceAll(rawJSON, []byte(`\u003e\u003e`), []byte(">>"))

	// Restore > (escaped as \u003e)
	rawJSON = bytes.ReplaceAll(rawJSON, []byte(`\u003e`), []byte(">"))

	correctedJSON = rawJSON
	return
}

// Checks whether given path exists or not (will be false if error)
func fileExists(path string) (exists bool) {
	_, err := os.Stat(path)
	if err == nil {
		exists = true
		return
	}
	if os.IsNotExist(err) {
		exists = false
		return
	}
	exists = false
	return
}

// Given input string, determines if user wants stdin, file contents, or direct string
// "-" = stdin
// file URI = read from file
// anything else = assume string is json
func getUserMetaHeaderInput(userInput string) (metadataHeader MetaHeader, err error) {
	if userInput == "" {
		err = fmt.Errorf("received no input")
		return
	}

	var inputJSON []byte
	if strings.TrimSpace(userInput) == "-" {
		inputJSON, err = io.ReadAll(os.Stdin)
		if err != nil {
			err = fmt.Errorf("error reading standard input: %v", err)
			return
		}
	} else if strings.HasPrefix(userInput, fileURIPrefix) {
		filePath := strings.TrimPrefix(userInput, fileURIPrefix)

		inputJSON, err = os.ReadFile(filePath)
		if err != nil {
			err = fmt.Errorf("unable to read given file '%s': %v", filePath, err)
			return
		}
	} else {
		inputJSON = []byte(userInput)
	}

	err = json.Unmarshal(inputJSON, &metadataHeader)
	if err != nil {
		err = fmt.Errorf("error parsing supplied JSON: %v", err)
	}
	return
}
