package parsing

import (
	"context"
	"fmt"
	"os"
	"scmp/internal/fsops"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"strings"
)

// Used when an argument has a file:// URI scheme
// Loads file in and separates based on newlines or commas and returns a string csv
func RetrieveURIFile(ctx context.Context, input string) (csv string, err error) {
	// Return early if not a file URI scheme
	if !strings.HasPrefix(input, global.FileURIPrefix) {
		csv = input
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Received File URI '%s'\n", input)

	// Not adhering to actual URI standards -- I just want file paths
	path := strings.TrimPrefix(input, global.FileURIPrefix)

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Preprocessed File URI Path '%s'\n", path)

	// Check for ~/ and expand if required
	path, err = fsops.ExpandHomeDirectory(path)
	if err != nil {
		err = fmt.Errorf("failed to resolve absolute path for '%s': %w", path, err)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "File URI contains path '%s'\n", path)

	// Retrieve the file contents
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Convert file to string
	file := string(fileBytes)

	// Trim newlines/spaces from beginning/end
	file = strings.TrimSpace(file)

	// Split file contents by newlines
	lines := strings.Split(file, "\n")

	// If file is multi-line, convert into CSV
	if len(lines) > 1 {
		csv = strings.Join(lines, ",")
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Extracted Override List from File: %v\n", csv)
		return
	} else if len(lines) == 0 {
		err = fmt.Errorf("file is empty")
		return
	}

	// Replace commas with spaces to unify the separator
	replacer := strings.NewReplacer(",", " ")
	normalized := replacer.Replace(csv)

	// Split on whitespace
	fields := strings.Fields(normalized)

	// Join back with a single comma
	csv = strings.Join(fields, ",")

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Extracted Override List from File: %v\n", csv)
	return
}
