package fsops

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Searches recursively all files and returns a list of files that contain the search.
// Result contains a mapping of relative file path (from rootDir) to the matching search strings and their count.
func FilesContaining(ctx context.Context, rootDir string, searches [][]byte) (result map[string]map[string]int, err error) {
	result = make(map[string]map[string]int)
	var resultMutex sync.RWMutex

	workers := runtime.NumCPU()

	jobs := make(chan string, workers*8)

	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			for path := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}

				for _, search := range searches {
					if bytes.Contains(data, search) {
						path = strings.TrimPrefix(path, rootDir)
						path = strings.TrimPrefix(path, string(os.PathSeparator))

						resultMutex.Lock()
						matchingSearches, ok := result[path]
						if !ok {
							matchingSearches = make(map[string]int)
						}
						matchingSearches[string(search)]++
						result[path] = matchingSearches
						resultMutex.Unlock()
					}
				}
			}
		})
	}

	walkErrCh := make(chan error, 1)

	go func() {
		defer close(jobs)

		err = filepath.WalkDir(rootDir, func(path string, entry fs.DirEntry, lerr error) (err error) {
			if lerr != nil {
				err = lerr
				return
			}

			if entry.IsDir() {
				return
			}

			select {
			case jobs <- path:
				return
			case <-ctx.Done():
				err = ctx.Err()
				return
			}
		})

		walkErrCh <- err
	}()

	wg.Wait()

	err = <-walkErrCh
	if err != nil && !errors.Is(err, context.Canceled) {
		return
	}

	return
}
