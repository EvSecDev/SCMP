// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Sets up new git repository based on controller-expected directory format
// Also creates intial commit so the first deployment will have something to compare against
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

	printMessage(VerbosityProgress, "Creating new repository at %v\n", absoluteRepoPath)

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

	printMessage(VerbosityProgress, "Setting initial branch name to %v\n", initialBranchName)

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
		InitOptions: *initOptions,
		Bare:        false,
	}

	printMessage(VerbosityProgress, "Initializing git repository\n")

	// Create git repo
	repo, err := git.PlainInitWithOptions(repoPath, plainInitOptions)
	logError("Failed to init git repository", err, false)

	// Read existing config options
	gitConfigPath := repoPath + "/.git/config"
	gitConfigFileBytes, err := os.ReadFile(gitConfigPath)
	logError("Failed to read git config file", err, false)

	printMessage(VerbosityProgress, "Setting initial git repository configuration options\n")

	// Write options to config file if no garbage collection section
	if !strings.Contains(string(gitConfigFileBytes), "[gc]") {
		// Open git config file - APPEND
		gitConfigFile, err := os.OpenFile(gitConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		logError("Failed to open git config file", err, false)
		defer gitConfigFile.Close()

		// Define garbage collection section and options
		repoGCOptions := `[gc]
        auto = 10
        reflogExpire = 8.days
        reflogExpireUnreachable = 8.days
        pruneExpire = 16.days
`

		// Write (append) string
		_, err = gitConfigFile.WriteString(repoGCOptions + "\n")
		logError("Failed to write git garbage collection options", err, false)
		gitConfigFile.Close()
	}

	printMessage(VerbosityProgress, "Adding example config metadata header files\n")

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

	printMessage(VerbosityProgress, "Creating an initial commit in repository\n")

	// Create initial commit
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "SCMPController",
			Email: "scmpc@localhost",
		},
	})
	logError("Failed to create first commit", err, false)

	printMessage(VerbosityProgress, "Adding a (disabled) git post-commit hook to call controller on commits\n")

	// Post commit path and contents
	postCommitFilePath := absoluteRepoPath + "/.git/hooks/post-commit.disabled"
	postCommit := fmt.Sprintf(`#!/bin/bash
	%v --git-hook-mode --deploy-changes -c %v`, os.Args[0], configFilePath)

	// Open post-commit file path
	file, err := os.OpenFile(postCommitFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0750)
	logError("Error opening post-commit file", err, false)
	defer file.Close()

	// Write the post-commit to the hook file
	_, err = file.WriteString(postCommit)
	logError("Error writing to post-commit file", err, false)
}
