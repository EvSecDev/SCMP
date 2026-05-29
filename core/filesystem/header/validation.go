package header

import (
	"context"
	"fmt"
	"os"
	"scmp/core/filesystem/metadata"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

// Extracts and validates existing metadata headers (including JSON syntax) in files
func Verify(ctx context.Context, fileInput str.LocalRepoPath) {
	csv, err := parsing.RetrieveURIFile(ctx, string(fileInput))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read file contents for verification input files: %v\n", err)
		os.Exit(1)
	}

	var files []string
	if str.Contains(csv, ",") {
		files = strings.Split(csv, ",")
	} else {
		files = append(files, csv)
	}

	for _, filePath := range files {
		inputFileContents, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read contents of specified file '%s': %v\n", filePath, err)
			os.Exit(1)
		}

		// Ignoring all outputs, just checking to make sure it works
		_, _, err = metadata.Extract(string(inputFileContents))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to extract contents from the specified file '%s': %v\n", filePath, err)
			os.Exit(1)
		}

		logctx.LogStdInfo(ctx, "Metadata header in '%s' is valid\n", filePath)
	}
}
