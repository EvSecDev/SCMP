// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Gets absolute path to the root of the git repository using the current working directory
// Value is saved to global config structure
func retrieveGitRepoPath() (err error) {
	printMessage(verbosityProgress, "Retrieving repository file path\n")

	// Get current dir (expected to be root of git repo)
	currentWorkingDir, err := os.Getwd()
	if err != nil {
		return
	}
	expectedDotGitPath := filepath.Join(currentWorkingDir, ".git")

	// Error if .git directory is not present in current directory
	_, err = os.Stat(expectedDotGitPath)
	if os.IsNotExist(err) {
		err = fmt.Errorf("not in a git repository, unable to continue")
		return
	} else if err != nil {
		return
	}

	// Guard against empty repo path
	if currentWorkingDir == "" {
		err = fmt.Errorf("failed to retrieve git repository path")
		return
	}

	// Current dir is absolute git repo path
	config.repositoryPath = currentWorkingDir
	return
}

// Retrieves working tree and worktree status using git repository in current working directory
func gitOpenCWD() (worktree *git.Worktree, status git.Status, err error) {
	// Check working dir for git repo
	err = retrieveGitRepoPath()
	if err != nil {
		return
	}

	// Open repository
	repo, err := git.PlainOpen(config.repositoryPath)
	if err != nil {
		return
	}

	// Get working tree
	worktree, err = repo.Worktree()
	if err != nil {
		return
	}

	// Get worktree status
	status, err = worktree.Status()

	return
}

// Adds changes based on user glob to the working tree
func gitAdd(addGlob string) (err error) {
	// Need repository path for artifact processing
	err = retrieveGitRepoPath()
	if err != nil {
		return
	}

	// Check for artifacts and update pointers if required
	gitArtifactTracking()

	// Retrieve working tree
	worktree, status, err := gitOpenCWD()
	if err != nil {
		return
	}

	// Return early if nothing to add
	if status.IsClean() {
		printMessage(verbosityStandard, "nothing to add, working tree clean\n")
		return
	}

	printMessage(verbosityFullData, "Raw add option: '%s'\n", addGlob)

	// Exit if dry-run requested
	if config.options.dryRunEnabled {
		printMessage(verbosityStandard, "Dry-run requested, not altering worktree\n")
		return
	}

	// Add all files to worktree
	err = worktree.AddGlob(addGlob)
	if err != nil {
		return
	}

	return
}

// Commit only already added worktree items
func gitCommit(gitCommitAction string) (err error) {
	// Retrieve working tree
	worktree, status, err := gitOpenCWD()
	if err != nil {
		return
	}

	// Return early if nothing to commit
	if status.IsClean() {
		printMessage(verbosityStandard, "nothing to commit, working tree clean\n")
		return
	}

	printMessage(verbosityFullData, "Raw commit option: '%s'\n", gitCommitAction)

	// Retrieve commit message from user supplied file
	var commitMessage string
	if strings.HasPrefix(gitCommitAction, fileURIPrefix) {
		// Not adhering to actual URI standards -- I just want file paths
		pathToCommitMessage := strings.TrimPrefix(gitCommitAction, fileURIPrefix)

		// Check for ~/ and expand if required
		pathToCommitMessage = expandHomeDirectory(pathToCommitMessage)

		printMessage(verbosityData, "Retrieving commit message from file: '%s'\n", pathToCommitMessage)

		// Retrieve the file contents
		var fileBytes []byte
		fileBytes, err = os.ReadFile(pathToCommitMessage)
		if err != nil {
			return
		}

		// Convert file to string
		commitMessage = string(fileBytes)
	} else {
		// Correct text
		commitMessage = strings.Trim(gitCommitAction, "'\" \n\r")
	}

	// Return if dry-run requested
	if config.options.dryRunEnabled {
		printMessage(verbosityStandard, "Dry-run requested, not committing\n")
		printMessage(verbosityStandard, "Received commit message: '%s'\n", commitMessage)
		return
	}

	// Commit changes
	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		AllowEmptyCommits: false,
	})
	if err != nil {
		return
	}

	// Set global to true, deployment might occur after this commit
	config.options.calledByGitHook = true

	return
}

// Opens repository and retrieves details about given commit
// If commitID is empty, will default to using HEAD commit
func getCommit(commitID *string) (tree *object.Tree, commit *object.Commit, err error) {
	printMessage(verbosityProgress, "Retrieving commit and tree from git repository\n")

	// Open the repository
	repo, err := git.PlainOpen(config.repositoryPath)
	if err != nil {
		err = fmt.Errorf("unable to open repository: %v", err)
		return
	}

	// If no commitID, assume they want to use the HEAD commit
	if *commitID == "" {
		// Get the pointer to the HEAD commit
		var ref *plumbing.Reference
		ref, err = repo.Head()
		if err != nil {
			err = fmt.Errorf("unable to get HEAD reference: %v", err)
			return
		}

		// Set HEAD commitID
		*commitID = ref.Hash().String()
	}

	// Verify commit ID string content
	if !isHex40(*commitID) {
		err = fmt.Errorf("invalid commit ID: hash is not 40 characters and/or is not hexadecimal")
		return
	}

	// Set hash
	commitHash := plumbing.NewHash(*commitID)

	// Get the commit
	commit, err = repo.CommitObject(commitHash)
	if err != nil {
		err = fmt.Errorf("unabke to get commit object: %v", err)
		return
	}

	// Get the tree from the commit
	tree, err = commit.Tree()
	if err != nil {
		err = fmt.Errorf("unable to get commit tree: %v", err)
		return
	}

	return
}

// Resets HEAD to previous commit without changing working directory
func gitRollBackOneCommit() (err error) {
	// Warn user
	fmt.Printf("WARNING: Removing current repository commit due to processing error.\n")
	fmt.Printf("         Working directory is **NOT** affected.\n")

	// Open the repo
	repo, err := git.PlainOpen(config.repositoryPath)
	if err != nil {
		err = fmt.Errorf("failed to open repository: %v", err)
		return
	}

	// Get the current branch reference
	currentBranchReference, err := repo.Reference(plumbing.ReferenceName("HEAD"), true)
	if err != nil {
		err = fmt.Errorf("failed to get branch name from HEAD commit: %v", err)
		return
	}

	// Get the branch HEAD commit
	currentBranchHeadCommit, err := repo.CommitObject(currentBranchReference.Hash())
	if err != nil {
		err = fmt.Errorf("failed to get HEAD commit: %v", err)
		return
	}

	// Ensure a previous commit exists before retrieve the hash
	if len(currentBranchHeadCommit.ParentHashes) == 0 {
		err = fmt.Errorf("head does not have a previous commit")
		return
	}

	// Get the previous commit hash
	previousCommitHash := currentBranchHeadCommit.ParentHashes[0]

	// Get the branch short name
	currentBranchNameString := currentBranchReference.Name()

	// Create new reference with the current branch and previous commit hash
	newBranchReference := plumbing.NewHashReference(plumbing.ReferenceName(currentBranchNameString), previousCommitHash)

	// Reset HEAD of current branch to previous commit
	err = repo.Storer.SetReference(newBranchReference)
	if err != nil {
		err = fmt.Errorf("failed to roll back current commit to previous commit: %v", err)
		return
	}
	return
}
