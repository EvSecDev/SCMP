// controller
package main

import (
	"fmt"
	"os"
	"encoding/json"
	"path/filepath"
	"strings"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func createNewRepository(newRepoInfo string) {
	// Split user choices
	userRepoChoices := strings.Split(newRepoInfo, ":")
	repoPath := userRepoChoices[0]
	initialBranchName := userRepoChoices[1]

	// Local os separator char
	OSPathSeparator = string(os.PathSeparator)

	// Only take absolute paths from user choice
	absoluteRepoPath, err := filepath.Abs(repoPath)
	logError("Failed to get absolute path to new repository", err, false)

	// Get individual dir names
	pathDirs := strings.Split(absoluteRepoPath, OSPathSeparator)

	// Error if it already exists
	_, err = os.Stat(absoluteRepoPath)
	if !os.IsNotExist(err) {
		logError("Failed to create new repository", fmt.Errorf("directory '%s' already exists", absoluteRepoPath), false)
	}

	// Create repository directories if missing
	repoPath = ""
	for _, pathDir := range pathDirs {
		// Skip empty
		if pathDir == "" {
			continue
		}

		// Save current dir to main path
		repoPath = repoPath + OSPathSeparator + pathDir

		// Check existence
		_, err := os.Stat(repoPath)
		if os.IsNotExist(err) {
			// Create if not exist
			err := os.Mkdir(repoPath, 0750)
			logError("Failed to create missing directory in repository path", err, false)
		}

		// Go to next dir in array
		pathDirs = pathDirs[:len(pathDirs)-1]
	}

	// Move into new repo directory
	err = os.Chdir(repoPath)
	logError("Failed to change into new repository directory", err, false)

	// Format branch name
	if initialBranchName != "refs/heads/"+initialBranchName {
		initialBranchName = "refs/heads/" + initialBranchName
	}

	// Set initial branch
	initialBranch := plumbing.ReferenceName(initialBranchName)
	initOptions := &git.InitOptions{
		DefaultBranch: initialBranch,
	}

	// Set git initial options
	plainInitOptions := &git.PlainInitOptions{
		InitOptions:  *initOptions,
		Bare:         false,
	}

	// Create git repo
	repo, err := git.PlainInitWithOptions(repoPath, plainInitOptions)
	logError("Failed to init git repository", err, false)

	// Create a working tree
	worktree, err := repo.Worktree()
	logError("Failed to create new git tree", err, false)

	// Example files
	exampleFiles := []string{".example-metadata-header.txt", ".example-metadata-header-noreload.txt"}

	// Create and add example files to repository
	for _, exampleFile := range exampleFiles {
		var metadataHeader MetaHeader

		// Populate metadata JSON with examples
		metadataHeader.TargetFileOwnerGroup = "root:root"
		metadataHeader.TargetFilePermissions = 640

		// Add reloads or dont depending on example file name
		if strings.Contains(exampleFile, "noreload") {
			metadataHeader.ReloadRequired = false
		} else {
			metadataHeader.ReloadRequired = true
			metadataHeader.ReloadCommands = []string{"systemctl restart rsyslog.service", "systemctl is-active rsyslog"}
		}

		// Create example metadata header files
		metadata, err := json.MarshalIndent(metadataHeader, "", "  ")
		logError("Failed to marshal example metadata JSON", err, false)

		// Add full header to string
		exampleHeader := Delimiter + "\n" + string(metadata) + "\n" + Delimiter + "\n"

		// Write example file to repo
		err = os.WriteFile(exampleFile, []byte(exampleHeader), 0640)
		logError("Failed to write example metadata file", err, false)

		// Stage the universal files
		_, err = worktree.Add(exampleFile)
		logError("Failed to add universal file", err, false)
	}

	// Create initial commit
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "SCMPController",
			Email: "scmpc@localhost",
		},
	})
	logError("Failed to create first commit", err, false)
}
