// controller
package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Parses and prepares deployment information
func preDeployment(deployMode string, commitID string, hostOverride string, fileOverride string) {
	// Show progress to user
	printMessage(verbosityStandard, "%s\n", progCLIHeader)
	var err error

	// Check working dir for git repo
	err = retrieveGitRepoPath()
	logError("Repository Error", err, false)

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
		printMessage(verbosityStandard, "No files available for deployment.\n")
		printMessage(verbosityStandard, "================================================\n")
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
		printMessage(verbosityStandard, "No deployment files for available hosts.\n")
		printMessage(verbosityStandard, "================================================\n")
		return
	}

	// Load the files for deployment
	allFileInfo, allFileData, err := loadFiles(allDeploymentFiles, tree)
	logError("Error loading files", err, true)

	// Correct order of file deployment to account for file dependency
	for _, host := range allDeploymentHosts {
		// Reorder deployment list
		newDeploymentFiles, err := handleFileDependencies(config.hostInfo[host].deploymentFiles, allFileInfo)
		logError("Failed to resolve file dependencies", err, true)

		// Save back to global
		hostInfo := config.hostInfo[host]
		hostInfo.deploymentFiles = newDeploymentFiles
		config.hostInfo[host] = hostInfo
	}

	// Ensure local system is in a state that is able to deploy
	err = localSystemChecks()
	logError("Error in local system checks", err, true)

	// Show progress to user
	printMessage(verbosityStandard, "Beginning deployment of %d files(s) to %d host(s)\n", len(allFileInfo), len(allDeploymentHosts))

	// Post deployment metrics
	postDeployMetrics := &PostDeploymentMetrics{}

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, config.maxSSHConcurrency)

	// Retrieve keys and passwords for any hosts that require it
	for _, endpointName := range allDeploymentHosts {
		// Retrieve host secrests (keys,passwords)
		err = retrieveHostSecrets(endpointName)
		logError("Error retrieving host secrets", err, true)
	}

	// Get current timestamp for deployment elapsed time metric
	deploymentStartTime := time.Now().UnixMilli()

	// Start SSH Deployments by host
	var wg sync.WaitGroup
	for _, endpointName := range allDeploymentHosts {
		// If requesting multithreaded deployment, start go routine, otherwise run without concurrency
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		if config.maxSSHConcurrency > 1 {
			go sshDeploy(&wg, semaphore, config.hostInfo[endpointName], allFileInfo, allFileData, postDeployMetrics)
		} else {
			sshDeploy(&wg, semaphore, config.hostInfo[endpointName], allFileInfo, allFileData, postDeployMetrics)
			if failTracker.buffer.Len() > 0 {
				// Deployment error occured, don't continue with deployments
				break
			}
		}
	}
	wg.Wait()

	// Get final timestamp to mark end of deployment
	deploymentEndTime := time.Now().UnixMilli()

	// Diff deployment time
	deploymentElapsedTime := deploymentEndTime - deploymentStartTime

	// Make pretty string with units for user
	postDeployMetrics.timeElapsed = formatElapsedTime(deploymentElapsedTime)

	// If user requested dry run - print collected information
	if dryRunRequested {
		printDeploymentInformation(allFileInfo, allDeploymentHosts)
		printMessage(verbosityStandard, "================================================\n")
		return
	}

	// Format byte metric to string
	postDeployMetrics.sizeTransferred = formatBytes(postDeployMetrics.bytes)

	// Save deployment errors to fail tracker
	if failTracker.buffer.Len() > 0 {
		printMessage(verbosityStandard, "PARTIAL COMPLETE: %d item(s) deployed to %d host(s) in %s - (%s transferred)\n", postDeployMetrics.files, postDeployMetrics.hosts, postDeployMetrics.timeElapsed, postDeployMetrics.sizeTransferred)
		printMessage(verbosityStandard, "Failure(s) in deployment (commit: %s):\n\n", commitID)

		err := recordDeploymentError(commitID)
		logError("Error in failure recording", err, false)
		return
	}

	// Remove fail tracker file after successful redeployment if it exists - best effort
	err = os.Remove(config.failTrackerFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No warning if the file doesn't exist
		} else {
			// Print a warning for any other error
			printMessage(verbosityStandard, "Warning: Failed to remove file %s: %v\n", config.failTrackerFilePath, err)
		}
	}

	// Show progress to user
	printMessage(verbosityStandard, "\nCOMPLETE: %d item(s) deployed to %d host(s) in %s - (%s transferred)\n", postDeployMetrics.files, postDeployMetrics.hosts, postDeployMetrics.timeElapsed, postDeployMetrics.sizeTransferred)
	printMessage(verbosityStandard, "================================================\n")
}
