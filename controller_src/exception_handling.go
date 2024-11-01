// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/coreos/go-systemd/v22/journal"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ###################################
//      EXCEPTION HANDLING
// ###################################

func logError(errorDescription string, errorMessage error, CleanupNeeded bool) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}
	// If requested, put error in journald
	if LogToJournald {
		err := CreateJournaldLog("", "", fmt.Sprintf("%s: %v", errorDescription, errorMessage))
		if err != nil {
			fmt.Printf("Failed to create journald entry: %v\n", err)
		}
	}

	// Print the error
	fmt.Printf("\n\n%s: %v\n", errorDescription, errorMessage)

	// Only roll back commit if the program was started by a hook and if the commit rollback is requested
	// Reset commit because the current commit should reflect what is deployed in the network
	// Conceptually, the rough equivalent of this command: git reset --soft HEAD~1
	if CalledByGitHook && CleanupNeeded {
		// Warn user
		fmt.Printf("WARNING: Removing current repository commit due to processing error.\n")
		fmt.Printf("         Working directory is **NOT** affected.\n")

		// Open the repo
		repo, err := git.PlainOpen(RepositoryPath)
		if err != nil {
			fmt.Printf("Error rolling back commit. Failed to open repository: %v\n", err)
			os.Exit(1)
		}

		// Get the current branch reference
		currentBranchReference, err := repo.Reference(plumbing.ReferenceName("HEAD"), true)
		if err != nil {
			fmt.Printf("Error rolling back commit. Failed to get branch name from HEAD commit: %v\n", err)
			os.Exit(1)
		}

		// Get the branch HEAD commit
		currentBranchHeadCommit, err := repo.CommitObject(currentBranchReference.Hash())
		if err != nil {
			fmt.Printf("Error rolling back commit. Failed to get HEAD commit: %v\n", err)
			os.Exit(1)
		}

		// Ensure a previous commit exists before retrieve the hash
		if len(currentBranchHeadCommit.ParentHashes) == 0 {
			fmt.Printf("Error rolling back commit. HEAD does not have a previous commit\n")
			os.Exit(1)
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
			fmt.Printf("Failed to roll back current commit to previous commit: %v\n", err)
			os.Exit(1)
		}

		// Tell user how to continue
		fmt.Printf("Please fix the above error then `git add` and `git commit` to restart deployment process.\n")
	}

	fmt.Printf("================================================\n")
	os.Exit(1)
}

// Create log entry in journald
func CreateJournaldLog(endpointName string, filePath string, errorMessage string) (err error) {
	// Send entry to journald
	err = journal.Send(errorMessage, journal.PriErr, map[string]string{
		"endpointName": endpointName,
		"filePath":     filePath,
	})
	if err != nil {
		return
	}

	return
}

// Called from within go routines
func hostDeployFailCleanup(endpointName string, filePath string, errorMessage error) {
	// Set file path to N/A if a host failed before any files were deployed
	if filePath == "" {
		filePath = "N/A"
	}

	// Ensure multiline error messages dont make their way into json
	Message := errorMessage.Error()
	Message = strings.ReplaceAll(Message, "\n", " ")
	Message = strings.ReplaceAll(Message, "\r", " ")

	// Send error to journald
	if LogToJournald {
		err := CreateJournaldLog(endpointName, filePath, Message)
		if err != nil {
			fmt.Printf("Failed to create journald entry: %v\n", err)
		}
	}

	// Parseable one line json for failures
	info := ErrorInfo{
		EndpointName: endpointName,
		FilePath:     filePath,
		ErrorMessage: Message,
	}

	// Marshal info string to a json format
	FailedInfo, err := json.Marshal(info)
	if err != nil {
		fmt.Printf("Failed to create Fail Tracker Entry for host %s file %s\n", endpointName, filePath)
		fmt.Printf("    Error: %s\n", Message)
		return
	}

	// Write (append) fail info for this go routine to global failures - dont conflict with other host go routines
	FailTrackerMutex.Lock()
	FailTracker += string(FailedInfo) + "\n"
	FailTrackerMutex.Unlock()
}

// Ensure config is not missing fields
func checkConfigForEmpty(config *Config) (err error) {
	if config.Controller.RepositoryPath == "" {
		err = fmt.Errorf("RepositoryPath")
	} else if config.SSHClient.SSHIdentityFile == "" {
		err = fmt.Errorf("SSHIdentityFile")
	} else if config.SSHClient.KnownHostsFile == "" {
		err = fmt.Errorf("KnownHostsFile")
	} else if config.SSHClient.RemoteTransferBuffer == "" {
		err = fmt.Errorf("RemoteTransferBuffer")
	} else if config.SSHClient.MaximumConcurrency == 0 {
		err = fmt.Errorf("MaximumConcurrency")
	} else if config.SSHClient.SudoPassword == "" {
		err = fmt.Errorf("SudoPassword")
	} else if config.TemplateDirectory == "" {
		err = fmt.Errorf("TemplateDirectory")
	}
	return
}
