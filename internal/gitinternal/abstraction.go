package gitinternal

import (
	"bytes"
	"context"
	"os"
	"scmp/core/deployment"
	"scmp/internal/fsops"
	"scmp/internal/str"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// PathWalker built from git tree
func NewTreeWalker(tree *object.Tree, repositoryPath string) (walker fsops.PathWalker) {
	walker = func() (paths []str.LocalRepoPath, err error) {
		var files []str.LocalRepoPath
		err = tree.Files().ForEach(func(fileObj *object.File) (err error) {
			rel := strings.TrimPrefix(fileObj.Name, repositoryPath)
			files = append(files, str.LocalRepoPath(rel))
			return
		})
		return
	}
	return
}

// FileSearcher built from git tree
func NewTreeSearcher(tree *object.Tree) (searcher fsops.FileSearcher) {
	searcher = func(ctx context.Context, searchTerms [][]byte) (results map[string]map[string]int, err error) {
		results = make(map[string]map[string]int)
		err = tree.Files().ForEach(func(fileObj *object.File) (err error) {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Always ignore files in root of repository
			if !strings.ContainsRune(fileObj.Name, os.PathSeparator) {
				return
			}

			// Always ignore (files under) directories with underscore prefix
			if strings.HasPrefix(fileObj.Name, string(deployment.IgnoreDirectoryPrefix)) {
				return
			}

			data, err := fileObj.Contents()
			if err != nil {
				return
			}
			for _, search := range searchTerms {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if bytes.Contains([]byte(data), search) {
					rel := str.LocalRepoPath(fileObj.Name)

					match, ok := results[string(rel)]
					if !ok {
						match = make(map[string]int)
						results[string(rel)] = match
					}

					match[string(search)]++
				}
			}
			return
		})
		return
	}
	return
}

// FileReader built from git tree
func NewTreeReader(tree *object.Tree) (readFile fsops.FileReader) {
	gitTree := tree // captured
	readFile = func(relPath str.LocalRepoPath) (content []byte, err error) {
		fileObj, err := gitTree.File(string(relPath))
		if err != nil {
			return
		}
		contentText, err := fileObj.Contents()
		if err != nil {
			return
		}
		content = []byte(contentText)
		return
	}
	return
}
