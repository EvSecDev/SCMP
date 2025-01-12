// controller
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Does a couple things
//
//	Moves into repository directory if not already
//	Checks for active network interfaces (can't deploy to remote endpoints if no network)
//	Loads known_hosts file into global variable
func localSystemChecks() (err error) {
	printMessage(VerbosityStandard, "Running local system checks...\n")
	printMessage(VerbosityProgress, "Ensuring system has an active network interface\n")

	// Get list of local systems network interfaces
	systemNetInterfaces, err := net.Interfaces()
	if err != nil {
		err = fmt.Errorf("failed to obtain system network interfaces: %v", err)
		return
	}

	// Ensure system has an active network interface
	var noActiveNetInterface bool
	for _, iface := range systemNetInterfaces {
		// Net interface is up and not loopback
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			noActiveNetInterface = false
			break
		}
		noActiveNetInterface = true
	}
	if noActiveNetInterface {
		err = fmt.Errorf("no active network interfaces found, will not attempt network connections")
		return
	}

	printMessage(VerbosityProgress, "Retrieving known_hosts file contents\n")

	// Read in file
	knownHostFile, err := os.ReadFile(knownHostsFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read known_hosts file: %v", err)
		return
	}

	// Store all known_hosts as array
	knownhosts = strings.Split(string(knownHostFile), "\n")

	return
}

// Opens repository and retrieves details about given commit
// If commitID is empty, will default to using HEAD commit
func getCommit(commitID *string) (tree *object.Tree, commit *object.Commit, err error) {
	printMessage(VerbosityProgress, "Retrieving commit and tree from git repository\n")

	// Open the repository
	repo, err := git.PlainOpen(RepositoryPath)
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
	if !SHA1RegEx.MatchString(*commitID) {
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

// Post-deployment if an error occured
// Takes global failure tracker and current commit id and writes it to the fail tracker file in the root of the repository
// Also prints custom stdout to user to show the errors and how to initiate redeploy when fixed
func recordDeploymentError(commitID string) (err error) {
	// Tell user about error and how to redeploy, writing fails to file in repo
	PathToExe := os.Args[0]

	printMessage(VerbosityStandard, "\nPARTIAL COMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	printMessage(VerbosityStandard, "Failure(s) in deployment (commit: %s):\n\n", commitID)

	// Create decoder for raw failtracker JSON
	failReader := strings.NewReader(FailTracker)
	failDecoder := json.NewDecoder(failReader)

	// Print pretty version of failtracker
	var failures ErrorInfo
	for {
		// unmarshal JSON object using struct
		err = failDecoder.Decode(&failures)
		if err != nil {
			// Done with errors - exit loop
			if err.Error() == "EOF" {
				break
			}

			// Actual error, return
			err = fmt.Errorf("failed to unmarshal failtracker JSON for pretty print: %v", err)
			return
		}

		// Print host name that failed
		printMessage(VerbosityStandard, "Host:  %s\n", failures.EndpointName)

		// Print failed file in local path format
		if len(failures.Files) > 0 {
			printMessage(VerbosityStandard, "Files: %v\n", failures.Files)
		}

		// Print all the errors in a cascading format to show root cause
		errorLayers := strings.Split(failures.ErrorMessage, ": ")
		indentSpaces := 2
		for _, errorLayer := range errorLayers {
			// Print error at this layer with indent
			printMessage(VerbosityStandard, "%s%s\n", strings.Repeat(" ", indentSpaces), errorLayer)

			// Increase indent for next line
			indentSpaces += 2
		}
	}

	printMessage(VerbosityStandard, "\nPlease fix the errors, then run the following command to redeploy OR create new commit if file corrections are needed:\n")
	printMessage(VerbosityStandard, "%s -c %s --deploy-failures\n", PathToExe, configFilePath)

	// Add FailTracker string to repo working directory fail file
	FailTrackerPath := filepath.Join(RepositoryPath, FailTrackerFile)
	FailTrackerFile, err := os.Create(FailTrackerPath)
	if err != nil {
		err = fmt.Errorf("Failed to create FailTracker File - manual redeploy using '--use-failtracker-only' will not work. Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A): %v\n", err)
		return
	}
	defer FailTrackerFile.Close()

	// Add commitid line to top of fail tracker
	FailTrackerAndCommit := "commitid:" + commitID + "\n" + FailTracker

	// Write string to file (overwrite old contents)
	_, err = FailTrackerFile.WriteString(FailTrackerAndCommit)
	if err != nil {
		err = fmt.Errorf("Failed to write FailTracker to File - manual redeploy using '--use-failtracker-only' will not work. Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A): %v\n", err)
		return
	}

	printMessage(VerbosityStandard, "================================================\n")
	return
}

func printDeploymentInformation(hostsAndEndpointInfo map[string]EndpointInfo, commitFileInfo map[string]CommitFileInfo) {
	// Notify user that program is in dry run mode
	printMessage(VerbosityStandard, "Requested dry-run, aborting deployment\n")
	if globalVerbosityLevel > 2 {
		// If not running with higher verbosity, no need to collect deployment information
		return
	}
	printMessage(VerbosityProgress, "Outputting information collected for deployment:\n")

	// Print deployment info by host
	for _, hostInfo := range hostsAndEndpointInfo {
		// Truncate the password if running at a low verbosity level
		// Password will only print at all if more than or equal to 2
		if globalVerbosityLevel < 3 {
			if len(hostInfo.Password) > 6 {
				hostInfo.Password = hostInfo.Password[:6]
			}
			hostInfo.Password += "..."
		}

		// Print out information for this specific host
		printMessage(VerbosityProgress, "Host: %s\n", hostInfo.EndpointName)
		printMessage(VerbosityProgress, "  Options:\n")
		printMessage(VerbosityProgress, "       Endpoint Address: %s\n", hostInfo.Endpoint)
		printMessage(VerbosityProgress, "       SSH User:         %s\n", hostInfo.EndpointUser)
		printMessage(VerbosityProgress, "       SSH Key:          %s\n", hostInfo.PrivateKey.PublicKey())
		printMessage(VerbosityProgress, "       Password:         %s\n", hostInfo.Password)
		printMessage(VerbosityProgress, "       Transfer Buffer:  %s\n", hostInfo.RemoteTransferBuffer)
		printMessage(VerbosityProgress, "       Backup Dir:       %s\n", hostInfo.RemoteBackupDir)
		printMessage(VerbosityProgress, "  Files:\n")

		// Identify maximum indent file name prints will need to be
		var maxFileNameLength int
		for _, filePath := range hostInfo.DeploymentFiles {
			// Format to remote path type
			_, targetFile := separateHostDirFromPath(filePath)

			nameLength := len(targetFile)
			if nameLength > maxFileNameLength {
				maxFileNameLength = nameLength
			}
		}
		// Increment indent so longest file name has at least one space after it
		maxFileNameLength += 1

		// Print out files for this specific host
		for _, file := range hostInfo.DeploymentFiles {
			// Format to remote path type
			_, targetFile := separateHostDirFromPath(file)

			// Determine how many spaces to add after file name
			indentSpaces := maxFileNameLength - len(targetFile)

			// Print what we are going to do, the local file path, and remote file path
			printMessage(VerbosityProgress, "       %s:           %s%s# %s\n", commitFileInfo[file].Action, targetFile, strings.Repeat(" ", indentSpaces), file)
		}
	}
}
