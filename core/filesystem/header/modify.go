package header

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"scmp/core/filesystem"
	"scmp/core/filesystem/content"
	"scmp/core/filesystem/metadata"
	"scmp/core/filesystem/terminal"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

func Modify(ctx context.Context, filePath str.LocalRepoPath, input string, editInPlace bool) {
	inputFileContents, err := os.ReadFile(string(filePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read contents of specified file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	oldHeader, fileContents, err := metadata.Extract(string(inputFileContents))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to extract contents from the specified file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	// User controls when we read the JSON from stdin via special flag
	var newHeader filesystem.MetaHeader
	if input != "" {
		newHeader, err = getUserMetaHeaderInput(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to retrieve metadata input: %v\n", err)
			os.Exit(1)
		}
	} else {
		newHeader, err = terminal.HeaderEditor(oldHeader, filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to run interactive header editor: %v\n", err)
			os.Exit(1)
		}
	}

	if editInPlace {
		err = content.WriteRepoFile(ctx, filePath, newHeader, &fileContents)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write modified header to existing file '%s': %v\n", filePath, err)
			os.Exit(1)
		}
	} else {
		metaHeaderBytes, err := json.MarshalIndent(newHeader, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create new header: %v\n", err)
			os.Exit(1)
		}

		metaHeaderBytes = parsing.UnescapeShellRedirectors(metaHeaderBytes)
		header := string(metaHeaderBytes)

		var fullFileContent strings.Builder
		fullFileContent.WriteString(filesystem.MetaDelimiter)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(header)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(filesystem.MetaDelimiter)
		fullFileContent.WriteString("\n")
		if fileContents != nil {
			fullFileContent.Write(fileContents)
		}

		logctx.LogStdInfo(ctx, "%s", fullFileContent.String())
	}
}

func AddToExistingFile(ctx context.Context, filePath str.LocalRepoPath, input string, editInPlace bool) {
	// Pull file contents and grab just the data
	existingFileContents, err := os.ReadFile(string(filePath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read contents of specified file '%s': %v\n", filePath, err)
		os.Exit(1)
	}

	// Use extraction function as canary to determine if file has a header
	_, _, err = metadata.Extract(string(existingFileContents))
	if err == nil {
		fmt.Fprintf(os.Stderr, "Existing metadata header detected in file '%s': cannot overwrite headers with add subcommand, please use modify subcommand to change headers", filePath)
		os.Exit(1)
	}

	// User controls when we read the JSON from stdin via special flag
	var inputHeader filesystem.MetaHeader
	if input != "" {
		inputHeader, err = getUserMetaHeaderInput(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to retrieve metadata input: %v\n", err)
			os.Exit(1)
		}
	} else {
		inputHeader, err = terminal.HeaderEditor(inputHeader, filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to run interactive header editor: %v\n", err)
			os.Exit(1)
		}
	}

	// Write back or straight to stdout
	if editInPlace {
		err := content.WriteRepoFile(ctx, filePath, inputHeader, &existingFileContents)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write header to existing file: %v\n", err)
			os.Exit(1)
		}
	} else {
		metaHeaderBytes, err := json.MarshalIndent(inputHeader, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create new header: %v\n", err)
			os.Exit(1)
		}

		metaHeaderBytes = parsing.UnescapeShellRedirectors(metaHeaderBytes)
		header := string(metaHeaderBytes)

		var fullFileContent strings.Builder
		fullFileContent.WriteString(filesystem.MetaDelimiter)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(header)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(filesystem.MetaDelimiter)
		fullFileContent.WriteString("\n")
		if existingFileContents != nil {
			fullFileContent.Write(existingFileContents)
		}

		logctx.LogStdInfo(ctx, "%s", fullFileContent.String())
	}
}
