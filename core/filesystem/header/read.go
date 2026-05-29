// Package for file metadata (header-aware) modifications
package header

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"scmp/core/filesystem/metadata"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
)

// Extracts metadata header from file
// Prints to stdout or writes back to file
func Print(ctx context.Context, filePath str.LocalRepoPath, compactJSONMode bool) {
	file, err := os.ReadFile(string(filePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	metadata, _, err := metadata.Extract(string(file))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read header from file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	var header []byte
	if compactJSONMode {
		header, err = json.Marshal(metadata)
	} else {
		header, err = json.MarshalIndent(metadata, "", "  ")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse header from file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	header = parsing.UnescapeShellRedirectors(header)

	logctx.LogStdInfo(ctx, "%s\n", string(header))
}
