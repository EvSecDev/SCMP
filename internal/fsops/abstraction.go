package fsops

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"scmp/internal/str"
)

// Function types that allow associations/enumerations to operate against any file source
// (current filesystem, git tree, in-memory mock, etc.)
type (
	// Gets all file paths relative to the repository root
	PathWalker func() (paths []str.LocalRepoPath, err error)

	// Searches for byte patterns across repository files and returns
	// a map of relative path to search string occurrence counts
	FileSearcher func(ctx context.Context, searchTerms [][]byte) (results map[string]map[string]int, err error)

	// Reads the content of a single file by its relative path
	FileReader func(relPath str.LocalRepoPath) (content []byte, err error)
)

// Returns a PathWalker that walks the live filesystem
func NewFileSystemWalker(repoRoot string) PathWalker {
	return func() (paths []str.LocalRepoPath, err error) {
		return GetAllRepoFiles(repoRoot)
	}
}

// Returns a FileSearcher that searches the live filesystem
func NewFileSystemSearcher(repoRoot string) FileSearcher {
	return func(ctx context.Context, searchTerms [][]byte) (results map[string]map[string]int, err error) {
		return FilesContaining(ctx, repoRoot, searchTerms)
	}
}

// Returns a FileReader that reads from the live filesystem
func NewFileSystemReader(repoRoot string) FileReader {
	return func(relPath str.LocalRepoPath) (content []byte, err error) {
		return os.ReadFile(filepath.Join(repoRoot, string(relPath)))
	}
}

// Walks entire repository for all file paths
func GetAllRepoFiles(repoRoot string) (foundFiles []str.LocalRepoPath, err error) {
	err = filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, lerr error) (err error) {
		if lerr != nil {
			err = lerr
			return
		}
		if entry.IsDir() {
			return
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return
		}
		foundFiles = append(foundFiles, str.LocalRepoPath(rel))
		return
	})
	return
}
