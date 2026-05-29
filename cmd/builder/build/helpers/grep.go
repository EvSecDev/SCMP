package helpers

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type FileMatch struct {
	Path    string
	Line    string
	LineNum int
}

func ScanRepo(
	root string,
	stopOnFirst bool,
	match func(path string, line string) (matches bool),
) (matches []FileMatch, err error) {
	allowedExtensions := []string{".txt", ".md", ".go", ".sh", ".conf", ".c", ".json", ".js", ".ts", ".html"}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Skip unsupported files
		extension := filepath.Ext(path)
		if !slices.Contains(allowedExtensions, extension) {
			return nil
		}

		// Always skip builder code (ourselves)
		builderRoot := filepath.Join(root, "cmd", "builder") + string(os.PathSeparator)
		if strings.HasPrefix(path, builderRoot) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer func() {
			_ = f.Close()
		}()

		scanner := bufio.NewScanner(f)
		lineNum := 0

		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			if match(path, line) {
				matches = append(matches, FileMatch{
					Path:    path,
					Line:    line,
					LineNum: lineNum,
				})

				if stopOnFirst {
					return io.EOF
				}
			}
		}

		if scanner.Err() != nil {
			return fmt.Errorf("scanner failed at file '%s': %w", path, scanner.Err())
		}

		err = f.Close()
		if err != nil {
			return err
		}
		return nil
	})

	if err == io.EOF {
		return
	}
	return
}
