// controller
package main

import (
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
	printMessage(VerbosityProgress, "Running local system checks...\n")
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

	return
}

// Post-deployment if an error occured
// Takes global failure tracker and current commit id and writes it to the fail tracker file in the root of the repository
// Also prints custom stdout to user to show the errors and how to initiate redeploy when fixed
func recordDeploymentError(commitID string, postDeployMetrics *PostDeploymentMetrics) (err error) {
	// Tell user about error and how to redeploy, writing fails to file in repo
	PathToExe := os.Args[0]

	printMessage(VerbosityStandard, "PARTIAL COMPLETE: %d files(s) deployed to %d host(s) - (%s transferred)\n", postDeployMetrics.files, postDeployMetrics.hosts, postDeployMetrics.sizeTransferred)
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
			printMessage(VerbosityStandard, "Local Files: %v\n", failures.Files)
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

	// Remove errors that are not root-cause failures before writing to tracker file
	// If a redeploy can't re-attempt the failed action, then it shouldn't be in failtracker file
	var rootCauseErrors []string
	errorLines := strings.Split(FailTracker, "\n")
	for _, errorLine := range errorLines {
		// File restoration errors are not root cause
		if !strings.Contains(errorLine, "failed old config restoration") {
			rootCauseErrors = append(rootCauseErrors, errorLine)
		}
	}
	FailTracker = strings.Join(rootCauseErrors, "\n")

	// Add FailTracker string to repo working directory fail file
	FailTrackerFile, err := os.Create(config.FailTrackerFilePath)
	if err != nil {
		printMessage(VerbosityStandard, "Warning: Failed to create failtracker file. Manual redeploy using '--use-failtracker-only' will not work.\n")
		printMessage(VerbosityStandard, "  Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A)\n")
		return
	}
	defer FailTrackerFile.Close()

	// Add commitid line to top of fail tracker
	FailTrackerAndCommit := "commitid:" + commitID + "\n" + FailTracker

	// Write string to file (overwrite old contents)
	_, err = FailTrackerFile.WriteString(FailTrackerAndCommit)
	if err != nil {
		printMessage(VerbosityStandard, "Warning: Failed to create failtracker file. Manual redeploy using '--use-failtracker-only' will not work.\n")
		printMessage(VerbosityStandard, "  Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A)\n")
		return
	}

	printMessage(VerbosityStandard, "Please fix the errors, then run the following command to redeploy OR create new commit if file corrections are needed:\n")
	printMessage(VerbosityStandard, "%s -c %s --deploy-failures\n", PathToExe, config.FilePath)
	printMessage(VerbosityStandard, "================================================\n")
	return
}

// Print out deployment information in dry run mode
func printDeploymentInformation(commitFileInfo map[string]FileInfo, allDeploymentHosts []string) {
	// Notify user that program is in dry run mode
	printMessage(VerbosityStandard, "Requested dry-run, aborting deployment\n")
	if globalVerbosityLevel < 2 {
		// If not running with higher verbosity, no need to collect deployment information
		return
	}
	printMessage(VerbosityProgress, "Outputting information collected for deployment:\n")

	// Print deployment info by host
	for _, endpointName := range allDeploymentHosts {
		hostInfo := config.HostInfo[endpointName]
		printHostInformation(hostInfo)
		printMessage(VerbosityProgress, "  Files:\n")

		// Identify maximum indent file name prints will need to be
		var maxFileNameLength int
		var maxActionLength int
		for _, filePath := range hostInfo.DeploymentFiles {
			// Format to remote path type
			_, targetFile := translateLocalPathtoRemotePath(filePath)

			nameLength := len(targetFile)
			if nameLength > maxFileNameLength {
				maxFileNameLength = nameLength
			}

			actionLength := len(commitFileInfo[filePath].Action)
			if actionLength > maxActionLength {
				maxActionLength = actionLength
			}
		}
		// Increment indent so longest name has at least some space after it
		maxFileNameLength += 1
		maxActionLength += 9

		// Print out files for this specific host
		for _, file := range hostInfo.DeploymentFiles {
			// Format to remote path type
			_, targetFile := translateLocalPathtoRemotePath(file)

			// Determine how many spaces to add after file name
			fileIndentSpaces := maxFileNameLength - len(targetFile)

			// Determine how many spaces to add after action name
			actionIndentSpaces := maxActionLength - len(commitFileInfo[file].Action)

			// Print what we are going to do, the local file path, and remote file path
			printMessage(VerbosityProgress, "       %s:%s%s%s# %s\n", commitFileInfo[file].Action, strings.Repeat(" ", actionIndentSpaces), targetFile, strings.Repeat(" ", fileIndentSpaces), file)
		}
	}
}

// Ties into dry-runs to have a unified print of host information
// Information only prints when verbosity level is more than or equal to 2
func printHostInformation(hostInfo EndpointInfo) {
	if len(hostInfo.Password) == 0 {
		// If password is empty, indicate to user
		hostInfo.Password = "*Host Does Not Use Passwords*"
	} else if globalVerbosityLevel == 2 {
		// Truncate passwords at verbosity level 2
		if len(hostInfo.Password) > 6 {
			hostInfo.Password = hostInfo.Password[:6]
		}
		hostInfo.Password += "..."
	}

	// Print out information for this specific host
	printMessage(VerbosityProgress, "Host: %s\n", hostInfo.EndpointName)
	printMessage(VerbosityProgress, "  Options:\n")
	printMessage(VerbosityProgress, "       Endpoint Address:  %s\n", hostInfo.Endpoint)
	printMessage(VerbosityProgress, "       SSH User:          %s\n", hostInfo.EndpointUser)
	printMessage(VerbosityProgress, "       SSH Key:           %s\n", hostInfo.PrivateKey.PublicKey())
	printMessage(VerbosityProgress, "       Password:          %s\n", hostInfo.Password)
	printMessage(VerbosityProgress, "       Transfer Buffer:   %s\n", hostInfo.RemoteTransferBuffer)
	printMessage(VerbosityProgress, "       Backup Dir:        %s\n", hostInfo.RemoteBackupDir)
}
