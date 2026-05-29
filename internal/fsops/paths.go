package fsops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Ensures variables that contains paths do not have '~/' and is replaced with absolute path
func ExpandHomeDirectory(path string) (absolutePath string, err error) {
	path = strings.Trim(path, `"`)
	path = strings.Trim(path, `'`)

	// Return early if path doesn't have '~/' prefix
	if !strings.HasPrefix(path, "~/") {
		absolutePath = path
		return
	}

	// Remove '~/' prefixes
	path = strings.TrimPrefix(path, "~/")

	userHomeDirectory, err := os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("unable to find home directory: %w", err)
		return
	}

	// Combine Users home directory path with the input path
	absolutePath = filepath.Join(userHomeDirectory, path)
	return
}
