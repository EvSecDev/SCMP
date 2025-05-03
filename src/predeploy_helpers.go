// controller
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Checks for active network interfaces (can't deploy to remote endpoints if no network)
func localSystemChecks() (err error) {
	printMessage(verbosityProgress, "Running local system checks...\n")
	printMessage(verbosityProgress, "  Ensuring system has an active network interface\n")

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

	return
}

// Print out deployment information in dry run mode
func printDeploymentInformation(commitFileInfo map[string]FileInfo, allDeploymentHosts []string) {
	// Notify user that program is in dry run mode
	printMessage(verbosityStandard, "Requested dry-run, aborting deployment\n")
	printMessage(verbosityStandard, "Outputting information collected for deployment:\n")

	// Print deployment info by host
	for _, endpointName := range allDeploymentHosts {
		hostInfo := config.hostInfo[endpointName]
		printHostInformation(hostInfo)
		printMessage(verbosityStandard, "  Files:\n")

		// Identify maximum indent file name prints will need to be
		var maxFileNameLength int
		var maxActionLength int
		for _, filePath := range hostInfo.deploymentList.files {
			// Format to remote path type
			_, targetFile := translateLocalPathtoRemotePath(filePath)

			nameLength := len(targetFile)
			if nameLength > maxFileNameLength {
				maxFileNameLength = nameLength
			}

			actionLength := len(commitFileInfo[filePath].action)
			if actionLength > maxActionLength {
				maxActionLength = actionLength
			}
		}
		// Increment indent so longest name has at least some space after it
		maxFileNameLength += 1
		maxActionLength += 9

		// Print out files for this specific host
		for _, file := range hostInfo.deploymentList.files {
			// Format to remote path type
			_, targetFile := translateLocalPathtoRemotePath(file)

			// Determine how many spaces to add after file name
			fileIndentSpaces := maxFileNameLength - len(targetFile)

			// Determine how many spaces to add after action name
			actionIndentSpaces := maxActionLength - len(commitFileInfo[file].action)

			// Print what we are going to do, the local file path, and remote file path
			printMessage(verbosityStandard, "       %s:%s%s%s# %s\n", commitFileInfo[file].action, strings.Repeat(" ", actionIndentSpaces), targetFile, strings.Repeat(" ", fileIndentSpaces), file)
		}
	}
}

// Ties into dry-runs to have a unified print of host information
func printHostInformation(hostInfo EndpointInfo) {
	// Print out information for this specific host
	printMessage(verbosityStandard, "Host: %s\n", hostInfo.endpointName)
	printMessage(verbosityStandard, "  Options:\n")
	printMessage(verbosityStandard, "       Endpoint Address:  %s\n", hostInfo.endpoint)
	printMessage(verbosityStandard, "       SSH User:          %s\n", hostInfo.endpointUser)
	printMessage(verbosityStandard, "       Transfer Buffer:   %s\n", hostInfo.remoteTransferBuffer)
	printMessage(verbosityStandard, "       Backup Dir:        %s\n", hostInfo.remoteBackupDir)
}

func (deployMetrics *DeploymentMetrics) createReport(commitID string) (deploymentSummary DeploymentSummary) {
	deploymentSummary.ElapsedTime = formatElapsedTime(deployMetrics)
	deploymentSummary.StartTime = convertMStoTimestamp(deployMetrics.startTime)
	deploymentSummary.EndTime = convertMStoTimestamp(deployMetrics.endTime)
	deploymentSummary.CommitID = commitID

	var allHostBytes int
	for _, bytes := range deployMetrics.hostBytes {
		allHostBytes += bytes
	}
	deploymentSummary.TransferredData = formatBytes(allHostBytes)

	deploymentSummary.Counters.Hosts = len(deployMetrics.hostFiles)

	for host, files := range deployMetrics.hostFiles {
		var hostSummary HostSummary
		hostSummary.Name = host
		hostSummary.ErrorMsg = deployMetrics.hostErr[host]
		hostSummary.TotalItems = len(files)

		if deploymentSummary.Counters.Hosts > 1 {
			hostSummary.TransferredData = formatBytes(deployMetrics.hostBytes[host])
		}

		deploymentSummary.Counters.Items += hostSummary.TotalItems

		var hostItemsDeployed int
		for _, file := range files {
			var fileSummary ItemSummary
			fileSummary.Name = file
			fileSummary.ErrorMsg = deployMetrics.fileErr[file]
			fileSummary.Action = deployMetrics.fileAction[file]

			if fileSummary.ErrorMsg != "" {
				// Individual file failure
				fileSummary.Status = "Failed"
				deploymentSummary.Counters.FailedItems++
			} else if hostSummary.ErrorMsg != "" {
				// Entire host failures indicate every file failed
				fileSummary.Status = "Failed"
				deploymentSummary.Counters.FailedItems++
			} else {
				// No file errors indicate it was deployed
				fileSummary.Status = "Deployed"
				hostItemsDeployed++
				deploymentSummary.Counters.CompletedItems++
			}

			hostSummary.Items = append(hostSummary.Items, fileSummary)
		}

		if hostItemsDeployed == hostSummary.TotalItems {
			// If all items were successful, whole host deploy was successfuly
			hostSummary.Status = "Deployed"
			deploymentSummary.Counters.CompletedHosts++
		} else if hostItemsDeployed > 0 {
			// If at least one file deployed, host is partially successful
			hostSummary.Status = "Partial"
			deploymentSummary.Counters.FailedHosts++
		} else if hostItemsDeployed == 0 {
			// No successful files, whole host marked failed
			hostSummary.Status = "Failed"
			deploymentSummary.Counters.FailedHosts++
		} else {
			// Catch all
			hostSummary.Status = "Unknown"
			deploymentSummary.Counters.FailedHosts++
		}

		deploymentSummary.Hosts = append(deploymentSummary.Hosts, hostSummary)
	}

	if deploymentSummary.Counters.CompletedHosts == deploymentSummary.Counters.Hosts {
		deploymentSummary.Status = "Deployed"
	} else if deploymentSummary.Counters.CompletedHosts > 0 && deploymentSummary.Counters.FailedHosts > 0 {
		deploymentSummary.Status = "Partial"
	} else if deploymentSummary.Counters.CompletedHosts == 0 && deploymentSummary.Counters.FailedHosts > 0 {
		deploymentSummary.Status = "Failed"
	} else if deploymentSummary.Counters.Hosts == 0 {
		deploymentSummary.Status = "UpToDate"
	} else {
		deploymentSummary.Status = "Unknown"
	}

	return
}

// Prints custom stdout to user to show the root-cause errors
func (deploymentSummary DeploymentSummary) printFailures() (err error) {
	if deploymentSummary.Counters.FailedHosts == 0 && deploymentSummary.Counters.FailedItems == 0 {
		return
	}

	for _, hostDeployReport := range deploymentSummary.Hosts {
		printMessage(verbosityStandard, "Host: %s\n", hostDeployReport.Name)

		if hostDeployReport.ErrorMsg != "" {
			printMessage(verbosityStandard, "  Host Error: %s\n", hostDeployReport.ErrorMsg)
		}

		for _, fileDeployReport := range hostDeployReport.Items {
			fileErrorMessage := fileDeployReport.ErrorMsg
			if fileErrorMessage == "" {
				continue
			}

			printMessage(verbosityStandard, "  File: '%s'\n", fileDeployReport.Name)

			// Print all the errors in a cascading format to show root cause
			errorLayers := strings.Split(fileErrorMessage, ": ")
			indentSpaces := 1
			for _, errorLayer := range errorLayers {
				// Print error at this layer with indent
				printMessage(verbosityStandard, "%s%s\n", strings.Repeat(" ", indentSpaces), errorLayer)

				// Increase indent for next line
				indentSpaces += 1
			}
		}
	}
	return
}

// Writes deployment summary to disk for deploy retry use
func (deploymentSummary DeploymentSummary) saveReport() (err error) {
	if deploymentSummary.Counters.FailedHosts == 0 && deploymentSummary.Counters.FailedItems == 0 {
		return
	}

	defer func() {
		// General warning on any err on return
		if err != nil {
			printMessage(verbosityStandard, "Warning: Recording of deployment failures encountered an error. Manual redeploy using '--deploy-failures' will not work.\n")
			printMessage(verbosityStandard, "  Please use the above errors to create a new commit with ONLY those failed files\n")
		}
	}()

	// Create JSON text
	deploymentSummaryJSON, err := json.MarshalIndent(deploymentSummary, "", " ")
	if err != nil {
		return
	}
	deploymentSummaryText := string(deploymentSummaryJSON)

	// Send error to journald
	err = CreateJournaldLog(deploymentSummaryText, "err")
	if err != nil {
		return
	}

	// Add FailTracker string to fail file
	failTrackerFile, err := os.Create(config.failTrackerFilePath)
	if err != nil {
		return
	}
	defer failTrackerFile.Close()

	deploymentSummaryText = deploymentSummaryText + "\n"

	// Write string to file (overwrite old contents)
	_, err = failTrackerFile.WriteString(deploymentSummaryText)
	if err != nil {
		return
	}
	return
}

func postDeployCleanup(deployMetrics *DeploymentMetrics) (err error) {
	if len(deployMetrics.fileErr) > 0 {
		// Remove fail tracker file after successful redeployment - best effort
		err = os.Remove(config.failTrackerFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				// No warning if the file doesn't exist
			} else {
				err = fmt.Errorf("failed removing failtracker file: %v", err)
				return
			}
		}
	}

	return
}
