package header

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"scmp/core/filesystem"
	"scmp/internal/global"
	"strings"
)

// Given input string, determines if user wants stdin, file contents, or direct string
// "-" = stdin
// file URI = read from file
// anything else = assume string is json
func getUserMetaHeaderInput(userInput string) (metadataHeader filesystem.MetaHeader, err error) {
	if userInput == "" {
		err = fmt.Errorf("received no input")
		return
	}

	var inputJSON []byte
	if strings.TrimSpace(userInput) == "-" {
		inputJSON, err = io.ReadAll(os.Stdin)
		if err != nil {
			err = fmt.Errorf("error reading standard input: %w", err)
			return
		}
	} else if strings.HasPrefix(userInput, global.FileURIPrefix) {
		filePath := strings.TrimPrefix(userInput, global.FileURIPrefix)

		inputJSON, err = os.ReadFile(filePath)
		if err != nil {
			err = fmt.Errorf("unable to read given file '%s': %w", filePath, err)
			return
		}
	} else {
		inputJSON = []byte(userInput)
	}

	err = json.Unmarshal(inputJSON, &metadataHeader)
	if err != nil {
		err = fmt.Errorf("error parsing supplied JSON: %w", err)
	}
	return
}
