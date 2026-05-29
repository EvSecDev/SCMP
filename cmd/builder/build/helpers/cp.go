package helpers

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Copies srcDir into dstDir recursively (simplified cp -r)
// No permissions, timestamps, or symlink preservation.
func CopyDirRecursive(srcDir, dstDir string) (totalCopiedBytes int64, err error) {
	return copyDirCore(srcDir, dstDir, []string{})
}

// Copies srcDir into dstDir recursively with exclusion list (simplified cp -r)
// Exclude is a contains against the absolute path.
func CopyDirRecursiveWithExclude(srcDir, dstDir string, exclude []string) (totalCopiedBytes int64, err error) {
	return copyDirCore(srcDir, dstDir, exclude)
}

func copyDirCore(srcDir, dstDir string, exclude []string) (totalCopiedBytes int64, err error) {
	srcInfo, err := os.Stat(srcDir)
	if err != nil {
		return
	}
	if !srcInfo.IsDir() {
		err = fmt.Errorf("source is not a directory")
		return
	}

	// Create destination root if needed
	err = os.MkdirAll(dstDir, 0750)
	if err != nil {
		return
	}

	err = filepath.WalkDir(srcDir, fs.WalkDirFunc(func(path string, dirEntry fs.DirEntry, err error) (retErr error) {
		if err != nil {
			retErr = err
			return
		}

		// Compute relative path from source root
		relPath, retErr := filepath.Rel(srcDir, path)
		if retErr != nil {
			return
		}

		// skip root "."
		if relPath == "." {
			return
		}

		// Exclude based on patters
		var pathIsExcluded bool
		for _, excludePattern := range exclude {
			if strings.Contains(relPath, excludePattern) {
				pathIsExcluded = true
				break
			}
		}
		if pathIsExcluded {
			return
		}

		targetPath := filepath.Join(dstDir, relPath)

		if dirEntry.IsDir() {
			// Create directory in destination
			retErr = os.MkdirAll(targetPath, 0750)
			return
		}

		// Copy file
		writtenBytes, err := copyFile(path, targetPath)
		if err != nil {
			return
		}

		// Record size
		totalCopiedBytes += writtenBytes
		return
	}))
	return
}

func copyFile(src, dst string) (written int64, err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer func() {
		_ = in.Close()
	}()

	// Ensure parent directory exists
	err = os.MkdirAll(filepath.Dir(dst), 0750)
	if err != nil {
		return
	}

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	written, err = io.Copy(out, in)
	return
}
