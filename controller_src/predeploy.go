// controller
package main

import (
	"fmt"
	"os"
	"sync"
)

// Parses and prepares deployment information
func preDeployment(deployMode string, commitID string, hostOverride string, fileOverride string) {
	// Show progress to user
	printMessage(VerbosityStandard, "%s\n", progCLIHeader)
	var err error

	// Override commitID with one from failtracker if redeploy requested
	var failures []string
	if deployMode == "deployFailures" {
		commitID, failures, err = getFailTrackerCommit()
		logError("Failed to extract commitID/failures from failtracker file", err, false)
	}

	// Open repo and get details - using HEAD commit if commitID is empty
	// Pass by reference to ensure commitID can be used later if user did not specify one
	tree, commit, err := getCommit(&commitID)
	logError("Error retrieving commit details", err, true)

	// Retrieve all files/hosts for deployment
	var commitFiles map[string]string
	if deployMode == "deployChanges" {
		// Use changed files
		commitFiles, err = getCommitFiles(commit, fileOverride)
	} else if deployMode == "deployAll" {
		// Use changed and unchanged files
		commitFiles, err = getRepoFiles(tree, fileOverride)
	} else if deployMode == "deployFailures" {
		// Use failed files/hosts from last failtracker
		commitFiles, hostOverride, err = getFailedFiles(failures, fileOverride)
	} else {
		logError("Unknown deployment mode", fmt.Errorf("mode must be deployChanges, deployAll, or deployFailures"), true)
	}

	// Check error after retrieving files
	if err != nil {
		logError("Failed to retrieve files", err, true)
	}

	// Ensure files were actually retrieved - Non-error because this can happen under normal operations
	// Usually when committing files outside of host directories
	if len(commitFiles) == 0 {
		printMessage(VerbosityStandard, "No files available for deployment.\n")
		printMessage(VerbosityStandard, "================================================\n")
		return
	}

	// Gather map of files per host and per universal directory
	allHostsFiles, universalFiles, err := parseAllRepoFiles(tree)
	logError("Failed to track files by host/universal directory", err, true)

	// Create map of denied Universal files per host
	deniedUniversalFiles := mapDeniedUniversalFiles(allHostsFiles, universalFiles)

	// Create map of deployment files/info per host and list of all deployment files across hosts
	allDeploymentHosts, allDeploymentFiles := filterHostsAndFiles(deniedUniversalFiles, commitFiles, hostOverride)

	// Ensure files/hosts weren't all filtered out - Non-error because this can happen under normal operations
	// Can happen if user specifies change deploy mode with a host that didn't have any changes in the specified commit
	if len(allDeploymentFiles) == 0 || len(allDeploymentHosts) == 0 {
		printMessage(VerbosityStandard, "No deployment files for available hosts.\n")
		printMessage(VerbosityStandard, "================================================\n")
		return
	}

	// Load the files for deployment
	commitFileInfo, err := loadFiles(allDeploymentFiles, tree)
	logError("Error loading files", err, true)

	// Ensure local system is in a state that is able to deploy
	err = localSystemChecks()
	logError("Error in local system checks", err, true)

	// Show progress to user
	printMessage(VerbosityStandard, "Beginning deployment of %d configuration(s) to %d host(s)\n", len(commitFileInfo), len(allDeploymentHosts))

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, config.MaxSSHConcurrency)

	// Start SSH Deployments by host
	var wg sync.WaitGroup
	for _, endpointName := range allDeploymentHosts {
		// Retrieve host secrests (keys,passwords)
		err = retrieveHostSecrets(endpointName)
		logError("Error retrieving host secrets", err, true)

		// If requesting multithreaded deployment, start go routine, otherwise run without concurrency
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		if config.MaxSSHConcurrency > 1 {
			go deployConfigs(&wg, semaphore, config.HostInfo[endpointName], commitFileInfo)
		} else {
			deployConfigs(&wg, semaphore, config.HostInfo[endpointName], commitFileInfo)
			if len(FailTracker) > 0 {
				// Deployment error occured, don't continue with deployments
				break
			}
		}
	}
	wg.Wait()

	// Remove vault cache
	config.Vault = make(map[string]Credential)

	// If user requested dry run - print collected information
	if dryRunRequested {
		printDeploymentInformation(commitFileInfo, allDeploymentHosts)
		printMessage(VerbosityStandard, "================================================\n")
		return
	}

	// Save deployment errors to fail tracker
	if FailTracker != "" {
		err := recordDeploymentError(commitID)
		logError("Error in failure recording", err, false)
		return
	}

	// Remove fail tracker file after successful redeployment - removal errors don't matter, this is just cleaning up.
	if deployMode == "deployFailures" {
		os.Remove(config.FailTrackerFilePath)
	}

	// Show progress to user
	printMessage(VerbosityStandard, "\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	printMessage(VerbosityStandard, "================================================\n")
}
