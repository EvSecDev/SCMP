// controller
package main

import (
	"fmt"
	"os"
	"sync"
)

// Parses and prepares deployment information
func preDeployment(deployMode string, deployerEndpoints map[string]DeployerEndpoints, SSHClientDefault SSHClientDefaults, commitID string, hostOverride string, fileOverride string) {
	// Show progress to user
	printMessage(VerbosityStandard, "%s\n", progCLIHeader)

	// Ensure local system is in a state that is able to deploy
	err := localSystemChecks()
	logError("Error in local system checks", err, true)

	// Override commitID with one from failtracker if redeploy requested
	var failTrackerPath string
	var failures []string
	if deployMode == "deployFailures" {
		commitID, failTrackerPath, failures, err = getFailTrackerCommit()
		logError("Failed to extract commitID/failures from failtracker file", err, false)
	}

	// Open repo and get details - using HEAD commit if commitID is empty
	// Pass by reference to ensure commitID can be used later if user did not specify one
	tree, commit, err := getCommit(&commitID)
	logError("Error retrieving commit details", err, true)

	// Retrieve all files/hosts for deployment
	var commitFiles map[string]string
	var commitHosts map[string]struct{}
	if deployMode == "deployChanges" {
		// Use changed files
		commitFiles, commitHosts, err = getCommitFiles(commit, deployerEndpoints, fileOverride)
	} else if deployMode == "deployAll" {
		// Use changed and unchanged files
		commitFiles, commitHosts, err = getRepoFiles(tree, fileOverride)
	} else if deployMode == "deployFailures" {
		// Use failed files/hosts from last failtracker
		commitFiles, commitHosts, err = getFailedFiles(failures, fileOverride)
	} else {
		logError("Unknown deployment mode", fmt.Errorf("mode must be deployChanges, deployAll, or deployFailures"), true)
	}

	// Check error after retrieving files
	if err != nil {
		logError("Failed to retrieve files", err, true)
	}

	// Ensure files were actually retrieved
	if len(commitFiles) == 0 {
		// Not an error, usually when committing files outside of host directories.
		printMessage(VerbosityStandard, "No files available for deployment.\n")
		printMessage(VerbosityStandard, "================================================\n")
		return
	}

	// Create map of deployment files/info per host and list of all deployment files across hosts
	hostsAndEndpointInfo, allDeploymentFiles, err := filterHostsAndFiles(tree, commitFiles, commitHosts, deployerEndpoints, hostOverride, SSHClientDefault)
	logError("Failed to get host and files", err, true)

	// Ensure files/hosts weren't all filtered out
	// Can happen if user specifies change deploy mode with a host that didn't have any changes in the specified commit
	if len(allDeploymentFiles) == 0 || len(hostsAndEndpointInfo) == 0 {
		printMessage(VerbosityStandard, "No deployment files for available hosts.\n")
		printMessage(VerbosityStandard, "================================================\n")
		return
	}

	// Load the files for deployment
	commitFileInfo, err := loadFiles(allDeploymentFiles, tree)
	logError("Error loading files", err, true)

	// Nothing left after loading files
	if len(commitFileInfo) == 0 {
		printMessage(VerbosityStandard, "No deployment files for available hosts.\n")
		printMessage(VerbosityStandard, "================================================\n")
		return
	}

	// Show progress to user
	printMessage(VerbosityStandard, "Beginning deployment of %d configuration(s) to %d host(s)\n", len(commitFileInfo), len(hostsAndEndpointInfo))

	// If user requested dry run - print collected information so far and gracefully abort deployment
	if dryRunRequested {
		printDeploymentInformation(hostsAndEndpointInfo, commitFileInfo)
		printMessage(VerbosityStandard, "================================================\n")
		return
	}

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, MaxSSHConcurrency)

	// Start go routines for each remote host ssh
	var wg sync.WaitGroup
	for _, endpointInfo := range hostsAndEndpointInfo {
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		go deployConfigs(&wg, semaphore, endpointInfo, commitFileInfo)
	}
	wg.Wait()

	// Save deployment errors to fail tracker
	if FailTracker != "" {
		err := recordDeploymentError(commitID)
		logError("Error in failure recording", err, false)
		return
	}

	// Remove fail tracker file after successful redeployment - removal errors don't matter, this is just cleaning up.
	if deployMode == "deployFailures" {
		os.Remove(failTrackerPath)
	}

	// Show progress to user
	printMessage(VerbosityStandard, "\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	printMessage(VerbosityStandard, "================================================\n")
}
