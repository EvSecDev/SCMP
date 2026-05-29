package repository

import (
	"scmp/internal/str"

	"github.com/go-git/go-git/v5/plumbing/filemode"
)

// For abstracting file information away from git for testing
type GitChangedFileMetadata struct {
	fromNotOnFS bool
	fromPath    str.LocalRepoPath
	fromMode    filemode.FileMode
	toNotOnFS   bool
	toPath      str.LocalRepoPath
	toMode      filemode.FileMode
}
