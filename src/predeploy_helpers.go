// controller
package main

import (
	"fmt"
	"net"
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
		// Net interface is up
		if iface.Flags&net.FlagUp != 0 {
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
	printMessage(verbosityStandard, "       Transfer Dir:      %s\n", hostInfo.remoteBufferDir)
	printMessage(verbosityStandard, "       Backup Dir:        %s\n", hostInfo.remoteBackupDir)
}
