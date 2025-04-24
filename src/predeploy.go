// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Parses and prepares deployment information
func preDeployment(deployMode string, commitID string, hostOverride string, fileOverride string) {
	err := retrieveGitRepoPath()
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

	var commitFiles map[string]string

	switch deployMode {
	case "deployChanges":
		changedFiles, lerr := getChangedFiles(commit)
		logError("Failed to retrieve changed files", lerr, true)

		commitFiles, err = parseChangedFiles(changedFiles, fileOverride)
	case "deployAll":
		commitFiles, err = getRepoFiles(tree, fileOverride)
	case "deployFailures":
		commitFiles, hostOverride, err = getFailedFiles(failures, fileOverride)
	default:
		logError("Unknown deployment mode", fmt.Errorf("mode must be deployChanges, deployAll, or deployFailures"), true)
	}

	if err != nil {
		logError("Failed to retrieve files", err, true)
	} else if len(commitFiles) == 0 {
		// Non-error - can happen under normal operations: When committing files outside of host directories
		printMessage(verbosityStandard, "No files available for deployment.\n")
		return
	}

	allHostsFiles, universalFiles, err := parseAllRepoFiles(tree)
	logError("Failed to track files by host/universal directory", err, true)

	deniedUniversalFiles := mapDeniedUniversalFiles(allHostsFiles, universalFiles)

	allDeploymentHosts, allDeploymentFiles := filterHostsAndFiles(deniedUniversalFiles, commitFiles, hostOverride)

	if len(allDeploymentFiles) == 0 || len(allDeploymentHosts) == 0 {
		// Non-error - can happen under normal operations: if user specifies change deploy mode with a host that didn't have any changes in the specified commit
		printMessage(verbosityStandard, "No deployment files for available hosts.\n")
		return
	}

	rawFileContent, err := loadGitFileContent(allDeploymentFiles, tree)
	logError("Error loading files", err, true)

	allFileMeta, allFileData, err := parseFileContent(allDeploymentFiles, rawFileContent)
	logError("Error parsing loaded files", err, true)

	for _, host := range allDeploymentHosts {
		// Reorder deployment list
		newDeploymentFiles, err := handleFileDependencies(config.hostInfo[host].deploymentFiles, allFileMeta)
		logError("Failed to resolve file dependencies", err, true)

		// Save back to global
		hostInfo := config.hostInfo[host]
		hostInfo.deploymentFiles = newDeploymentFiles
		config.hostInfo[host] = hostInfo
	}

	err = localSystemChecks()
	logError("Error in local system checks", err, true)

	// Retrieve keys and passwords for any hosts that require it
	for _, endpointName := range allDeploymentHosts {
		// Retrieve host secrets
		config.hostInfo[endpointName], err = retrieveHostSecrets(config.hostInfo[endpointName])
		logError("Error retrieving host secrets", err, true)

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := config.hostInfo[endpointName].proxy
		if proxyName != "" {
			config.hostInfo[proxyName], err = retrieveHostSecrets(config.hostInfo[proxyName])
			logError("Error retrieving proxy secrets", err, true)
		}
	}

	// Metric collection
	deployMetrics := &DeploymentMetrics{}
	deployMetrics.hostFiles = make(map[string][]string)
	deployMetrics.hostBytes = make(map[string]int)
	deployMetrics.fileErr = make(map[string]string)
	deployMetrics.hostErr = make(map[string]string)

	printMessage(verbosityStandard, "Deploying %d item(s) to %d host(s)\n", len(allFileMeta), len(allDeploymentHosts))

	// Get current timestamp for deployment elapsed time metric
	deployMetrics.startTime = time.Now().UnixMilli()

	// Concurrency Limiter
	connLimiter := make(chan struct{}, config.options.maxSSHConcurrency)

	// Start SSH Deployments
	var wg sync.WaitGroup
	for _, endpointName := range allDeploymentHosts {
		hostInfo := config.hostInfo[endpointName]
		proxyInfo := config.hostInfo[config.hostInfo[endpointName].proxy]

		// If requesting multithreaded deployment, start go routine, otherwise run without concurrency
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		if config.options.maxSSHConcurrency > 1 {
			go sshDeploy(&wg, connLimiter, hostInfo, proxyInfo, allFileMeta, allFileData, deployMetrics)
		} else {
			sshDeploy(&wg, connLimiter, hostInfo, proxyInfo, allFileMeta, allFileData, deployMetrics)
			if failTracker.buffer.Len() > 0 {
				// Deployment error occured, don't continue with deployments
				break
			}
		}
	}
	wg.Wait()

	// Get final timestamp to mark end of deployment
	deployMetrics.endTime = time.Now().UnixMilli()

	if dryRunRequested {
		printDeploymentInformation(allFileMeta, allDeploymentHosts)
		return
	}

	deploymentSummary, err := deployMetrics.createReport()
	logError("Failed to calculate deployment metrics", err, false)

	if config.options.detailedSummaryRequested {
		// Detailed Summary
		deploymentSummaryJson, err := json.MarshalIndent(deploymentSummary, "", " ")
		logError("Failed to marshal detailed deployment summary JSON", err, false)

		printMessage(verbosityStandard, "%s\n", string(deploymentSummaryJson))
	} else {
		// Short Summary
		printMessage(verbosityStandard,
			"Status: %s. Deployed %d item(s) (%s) to %d host(s). Deployment took %s\n",
			deploymentSummary.Status,
			deploymentSummary.Counters.CompletedItems,
			deploymentSummary.TransferredData,
			deploymentSummary.Counters.CompletedHosts,
			deploymentSummary.ElapsedTime,
		)

		err = printDeploymentFailures()
		logError("Error in printing failures", err, false)
	}

	if failTracker.buffer.Len() > 0 {
		err := recordDeploymentError(commitID)
		logError("Error in failure recording", err, false)
	} else {
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
	}
}
