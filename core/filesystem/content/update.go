package content

import (
	"context"
	"fmt"
	"os"
	"scmp/core/filesystem/metadata"
	"scmp/internal/input"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"
)

// Takes all the data in source file and writes it into the destination file without affecting the destination file's metadata header
func ReplaceData(ctx context.Context, srcFilePath str.LocalRepoPath, dstFilePath str.LocalRepoPath, userConfirmed bool) {
	sourceFile, err := os.ReadFile(string(srcFilePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read source file path: %v\n", err)
		os.Exit(1)
	}

	destinationFile, err := os.ReadFile(string(dstFilePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read destination file path: %v\n", err)
		os.Exit(1)
	}

	// Grab existing metadata header
	jsonMetadata, _, err := metadata.Extract(string(destinationFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to separate metadata from content for file %s: %v\n", destinationFile, err)
		os.Exit(1)
	}

	var clearedToWrite bool
	if userConfirmed {
		clearedToWrite = true
	} else {
		userResponse, err := input.AskUser(ctx, "Type 'yes' to confirm overwrite of file data in "+string(dstFilePath), "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to prompt user for confirmation: %v\n", err)
			os.Exit(1)
		}

		if strings.ToLower(userResponse) == "yes" {
			clearedToWrite = true
		}
	}

	if !clearedToWrite {
		logctx.LogStdInfo(ctx, "Warning: Not writing new data to file '%s' because no confirmation was received\n", dstFilePath)
		return
	}

	// Write file back to destination using new content from source
	err = WriteRepoFile(ctx, dstFilePath, jsonMetadata, &sourceFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write source file contents from %s to destination %s: %v\n", srcFilePath, dstFilePath, err)
		os.Exit(1)
	}
}
