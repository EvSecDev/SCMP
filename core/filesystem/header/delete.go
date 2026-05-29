package header

import (
	"context"
	"fmt"
	"os"
	"scmp/core/filesystem/metadata"
	"scmp/internal/logctx"
	"scmp/internal/str"
)

// Removes header from file to get just the contents
// Prints to stdout or writes back to file
func Strip(ctx context.Context, filePath str.LocalRepoPath, editInPlace bool) {
	// Pull file contents and grab just the data
	inputFileContents, err := os.ReadFile(string(filePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read contents of specified file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	_, ouputFileContents, err := metadata.Extract(string(inputFileContents))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to extract contents from the specified file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	// Write back or straight to stdout
	if editInPlace {
		err = os.WriteFile(string(filePath), ouputFileContents, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write stripped contents to existing file: %v\n", err)
			os.Exit(1)
		}
	} else {
		logctx.LogStdInfo(ctx, "%s", string(ouputFileContents))
	}
}
