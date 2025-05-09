// controller
package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Parses and prepares deployment information
func preDeployment(deployMode string, commitID string, hostOverride string, fileOverride string) {
	err := retrieveGitRepoPath()
	logError("Repository Error", err, false)

	// Override commitID with one from failtracker if redeploy requested
	var lastDeploymentSummary DeploymentSummary
	if deployMode == "deployFailures" {
		commitID, lastDeploymentSummary, err = getFailTrackerCommit()
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

		commitFiles = parseChangedFiles(changedFiles, fileOverride)
	case "deployAll":
		commitFiles, err = getRepoFiles(tree, fileOverride)
	case "deployFailures":
		commitFiles, hostOverride, err = lastDeploymentSummary.getFailures(fileOverride)
	default:
		logError("Unknown deployment mode", fmt.Errorf("mode must be deployChanges, deployAll, or deployFailures"), false)
	}

	logError("Failed to retrieve files", err, false)

	if len(commitFiles) == 0 {
		// Non-error - can happen under normal operations: When committing files outside of host directories
		printMessage(verbosityStandard, "No files available for deployment.\n")
		return
	}

	allHostsFiles, universalFiles, err := parseAllRepoFiles(tree)
	logError("Failed to track files by host/universal directory", err, true)

	deniedUniversalFiles := mapDeniedUniversalFiles(allHostsFiles, universalFiles)

	allDeploymentHosts, allDeploymentFiles, hostDeploymentFiles := filterHostsAndFiles(deniedUniversalFiles, commitFiles, hostOverride)
	if len(allDeploymentFiles) == 0 || len(allDeploymentHosts) == 0 {
		// Non-error - can happen under normal operations: if user specifies change deploy mode with a host that didn't have any changes in the specified commit
		printMessage(verbosityStandard, "No deployment files for available hosts.\n")
		return
	}

	rawFileContent, err := loadGitFileContent(allDeploymentFiles, tree)
	logError("Error loading files", err, true)

	allFileMeta, allFileData, err := parseFileContent(allDeploymentFiles, rawFileContent)
	logError("Error parsing loaded files", err, true)

	config.hostInfo, err = sortFiles(config.hostInfo, hostDeploymentFiles, allFileMeta)
	logError("Failed sorting deployment files", err, true)

	err = localSystemChecks()
	logError("Error in local system checks", err, true)

	printMessage(verbosityStandard, "Deploying %d item(s) to %d host(s)\n", len(allFileMeta), len(allDeploymentHosts))

	if config.options.dryRunEnabled {
		printDeploymentInformation(allFileMeta, allDeploymentHosts)
		return
	}

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
	deployMetrics.fileAction = make(map[string]string)
	deployMetrics.startTime = time.Now().UnixMilli()

	// Start SSH Deployments
	// All failures and errors from here on are soft stops - program will finish, errors are tracked within deployment metrics, git commit will NOT be rolled back
	var wg sync.WaitGroup
	connLimiter := make(chan struct{}, config.options.maxSSHConcurrency)
	for _, endpointName := range allDeploymentHosts {
		hostInfo := config.hostInfo[endpointName]
		proxyInfo := config.hostInfo[config.hostInfo[endpointName].proxy]

		wg.Add(1)
		if config.options.maxSSHConcurrency > 1 {
			go sshDeploy(&wg, connLimiter, hostInfo, proxyInfo, allFileMeta, allFileData, deployMetrics)
		} else {
			// Max conns of 1 disables using go routine
			sshDeploy(&wg, connLimiter, hostInfo, proxyInfo, allFileMeta, allFileData, deployMetrics)

			// Don't continue to the next host on errors
			if len(deployMetrics.fileErr) > 0 {
				break
			}
		}
	}
	wg.Wait()

	deployMetrics.endTime = time.Now().UnixMilli()
	deploymentSummary := deployMetrics.createReport(commitID)

	if config.options.wetRunEnabled {
		printMessage(verbosityStandard, "Wet-run enabled. No mutating actions taken, theoretical deployment summary:\n")
	}

	// Show user what was done during deployment
	if config.options.detailedSummaryRequested {
		// Detailed Summary
		deploymentSummaryJson, err := json.MarshalIndent(deploymentSummary, "", " ")
		logError("Failed to marshal detailed deployment summary JSON", err, false)

		printMessage(verbosityStandard, "%s\n", string(deploymentSummaryJson))
	} else {
		printMessage(verbosityStandard,
			"Status: %s. Deployed %d item(s) (%s) to %d host(s). Deployment took %s\n",
			deploymentSummary.Status,
			deploymentSummary.Counters.CompletedItems,
			deploymentSummary.TransferredData,
			deploymentSummary.Counters.CompletedHosts,
			deploymentSummary.ElapsedTime,
		)

		err = deploymentSummary.printFailures()
		logError("Error in printing deployment failures", err, false)
	}

	err = deploymentSummary.saveReport()
	logError("Error in recording deployment failures", err, false)

	err = postDeployCleanup(deployMetrics)
	logError("Error running post-deployment cleanup", err, false)
}
