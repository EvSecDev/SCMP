package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/internal/config"
	"scmp/internal/global"
	"slices"
	"strings"
)

// Ensures path is relative to repository
// Returned paths are always relative to repository root
// Root requests are always returned as "."
func validateRequestedFilePath(ctx context.Context, path string) (cleanPath string, err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	if cfg.RepositoryPath == "" {
		err = fmt.Errorf("internal error: repository path not set")
		return
	}

	segments := strings.Split(path, string(os.PathSeparator))
	if slices.Contains(segments, "..") {
		err = fmt.Errorf("illegal path")
		return
	}

	// Clean up request path
	path = filepath.Join(cfg.RepositoryPath, path)
	path = filepath.Clean(path)

	// Ensure all requested paths are within the repository
	if !strings.HasPrefix(path, cfg.RepositoryPath) {
		// Should never get here, but just in case
		err = fmt.Errorf("illegal file path")
		return
	}

	// If absolute path is exactly the root of the repository, add dot
	if path == cfg.RepositoryPath {
		path += string(os.PathSeparator) + "."
	}

	// Remove absolute path and leading path separator (ensures we don't leak system paths to clients)
	cleanPath = strings.TrimPrefix(path, cfg.RepositoryPath)
	cleanPath = strings.TrimPrefix(cleanPath, string(os.PathSeparator))

	// Remove leading slashes
	cleanPath = strings.TrimPrefix(cleanPath, string(os.PathSeparator))

	// Ensure requested relative path is not for the root repositories .git directory
	if strings.HasPrefix(cleanPath, ".git") {
		err = fmt.Errorf("illegal file path")
		return
	}

	return
}
