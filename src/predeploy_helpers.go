// controller
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

// Does a couple things
//
//	Moves into repository directory if not already
//	Checks for active network interfaces (can't deploy to remote endpoints if no network)
//	Loads known_hosts file into global variable
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

// Prints custom stdout to user to show the root-cause errors
func printDeploymentFailures() (err error) {
	if failTracker.buffer.Len() > 0 {
		return
	}

	failReader := bytes.NewReader(failTracker.buffer.Bytes())
	failDecoder := json.NewDecoder(failReader)

	var failures ErrorInfo
	for {
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

		printMessage(verbosityStandard, "Host:  %s\n", failures.EndpointName)

		if len(failures.Files) > 0 {
			printMessage(verbosityStandard, "Local Files: %v\n", failures.Files)
		}

		// Print all the errors in a cascading format to show root cause
		errorLayers := strings.Split(failures.ErrorMessage, ": ")
		indentSpaces := 1
		for _, errorLayer := range errorLayers {
			// Print error at this layer with indent
			printMessage(verbosityStandard, "%s%s\n", strings.Repeat(" ", indentSpaces), errorLayer)

			// Increase indent for next line
			indentSpaces += 1
		}
	}
	return
}

// Takes global failure tracker and current commit id and writes it to the fail tracker file in the users ssh config directory
func recordDeploymentError(commitID string) (err error) {
	// Convert fail buffer back to string
	failTrackerText := failTracker.buffer.String()
	if failTrackerText == "" {
		printMessage(verbosityStandard, "Warning: Failed to read failtracker buffer. Manual redeploy using '--deploy-failures' will not work.\n")
		printMessage(verbosityStandard, "  Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A)\n")
		err = fmt.Errorf("failTracker buffer is empty")
		return
	}

	// Remove errors that are not root-cause failures before writing to tracker file
	// If a redeploy can't re-attempt the failed action, then it shouldn't be in failtracker file
	var rootCauseErrors []string
	errorLines := strings.SplitSeq(failTrackerText, "\n")
	for errorLine := range errorLines {
		// File restoration errors are not root cause
		if !strings.Contains(errorLine, "failed old config restoration") {
			rootCauseErrors = append(rootCauseErrors, errorLine)
		}
	}
	failTrackerText = strings.Join(rootCauseErrors, "\n")

	// Add FailTracker string to repo working directory fail file
	failTrackerFile, err := os.Create(config.failTrackerFilePath)
	if err != nil {
		printMessage(verbosityStandard, "Warning: Failed to create failtracker file. Manual redeploy using '--use-failtracker-only' will not work.\n")
		printMessage(verbosityStandard, "  Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A)\n")
		return
	}
	defer failTrackerFile.Close()

	// Add commitid line to top of fail tracker
	failTrackerAndCommit := "commitid:" + commitID + "\n" + failTrackerText

	// Write string to file (overwrite old contents)
	_, err = failTrackerFile.WriteString(failTrackerAndCommit)
	if err != nil {
		printMessage(verbosityStandard, "Warning: Failed to create failtracker file. Manual redeploy using '--use-failtracker-only' will not work.\n")
		printMessage(verbosityStandard, "  Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A)\n")
		return
	}
	return
}

// Print out deployment information in dry run mode
func printDeploymentInformation(commitFileInfo map[string]FileInfo, allDeploymentHosts []string) {
	// Notify user that program is in dry run mode
	printMessage(verbosityStandard, "Requested dry-run, aborting deployment\n")
	if globalVerbosityLevel < 2 {
		// If not running with higher verbosity, no need to collect deployment information
		return
	}
	printMessage(verbosityProgress, "Outputting information collected for deployment:\n")

	// Print deployment info by host
	for _, endpointName := range allDeploymentHosts {
		hostInfo := config.hostInfo[endpointName]
		printHostInformation(hostInfo)
		printMessage(verbosityProgress, "  Files:\n")

		// Identify maximum indent file name prints will need to be
		var maxFileNameLength int
		var maxActionLength int
		for _, filePath := range hostInfo.deploymentFiles {
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
		for _, file := range hostInfo.deploymentFiles {
			// Format to remote path type
			_, targetFile := translateLocalPathtoRemotePath(file)

			// Determine how many spaces to add after file name
			fileIndentSpaces := maxFileNameLength - len(targetFile)

			// Determine how many spaces to add after action name
			actionIndentSpaces := maxActionLength - len(commitFileInfo[file].action)

			// Print what we are going to do, the local file path, and remote file path
			printMessage(verbosityProgress, "       %s:%s%s%s# %s\n", commitFileInfo[file].action, strings.Repeat(" ", actionIndentSpaces), targetFile, strings.Repeat(" ", fileIndentSpaces), file)
		}
	}
}

// Ties into dry-runs to have a unified print of host information
// Information only prints when verbosity level is more than or equal to 2
func printHostInformation(hostInfo EndpointInfo) {
	if len(hostInfo.password) == 0 {
		// If password is empty, indicate to user
		hostInfo.password = "*Host Does Not Use Passwords*"
	} else if globalVerbosityLevel == 2 {
		// Truncate passwords at verbosity level 2
		if len(hostInfo.password) > 6 {
			hostInfo.password = hostInfo.password[:6]
		}
		hostInfo.password += "..."
	}

	// Print out information for this specific host
	printMessage(verbosityProgress, "Host: %s\n", hostInfo.endpointName)
	printMessage(verbosityProgress, "  Options:\n")
	printMessage(verbosityProgress, "       Endpoint Address:  %s\n", hostInfo.endpoint)
	printMessage(verbosityProgress, "       SSH User:          %s\n", hostInfo.endpointUser)
	printMessage(verbosityProgress, "       SSH Key:           %s\n", hostInfo.privateKey.PublicKey())
	printMessage(verbosityProgress, "       Password:          %s\n", hostInfo.password)
	printMessage(verbosityProgress, "       Transfer Buffer:   %s\n", hostInfo.remoteTransferBuffer)
	printMessage(verbosityProgress, "       Backup Dir:        %s\n", hostInfo.remoteBackupDir)
}

func (deployMetrics *DeploymentMetrics) createReport() (deploymentSummary DeploymentSummary, err error) {
	deploymentSummary.ElapsedTime = formatElapsedTime(deployMetrics)
	deploymentSummary.StartTime = convertMStoTimestamp(deployMetrics.startTime)
	deploymentSummary.EndTime = convertMStoTimestamp(deployMetrics.endTime)

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

			if fileSummary.ErrorMsg != "" {
				fileSummary.Status = "Failed"
				deploymentSummary.Counters.FailedItems++
			} else {
				fileSummary.Status = "Deployed"
				hostItemsDeployed++
				deploymentSummary.Counters.CompletedItems++
			}

			hostSummary.Items = append(hostSummary.Items, fileSummary)
		}

		if hostItemsDeployed == hostSummary.TotalItems {
			hostSummary.Status = "Deployed"
			deploymentSummary.Counters.CompletedHosts++
		} else if hostItemsDeployed > 0 {
			hostSummary.Status = "Partial"
			deploymentSummary.Counters.FailedHosts++
		} else if hostItemsDeployed == 0 {
			hostSummary.Status = "Failed"
			deploymentSummary.Counters.FailedHosts++
		} else {
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
