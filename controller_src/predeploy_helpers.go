// controller
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Post-deployment if an error occured
// Takes global failure tracker and current commit id and writes it to the fail tracker file in the root of the repository
// Also prints custom stdout to user to show the errors and how to initiate redeploy when fixed
func recordDeploymentError(commitID string) (err error) {
	// Tell user about error and how to redeploy, writing fails to file in repo
	PathToExe := os.Args[0]

	fmt.Printf("\nPARTIAL COMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
	fmt.Printf("Failure(s) in deployment (commit: %s):\n\n", commitID)

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
		fmt.Printf("Host:  %s\n", failures.EndpointName)

		// Print failed file in local path format
		if len(failures.Files) > 0 {
			fmt.Printf("Files: %v\n", failures.Files)
		}

		// Print all the errors in a cascading format to show root cause
		errorLayers := strings.Split(failures.ErrorMessage, ": ")
		indentSpaces := 2
		for _, errorLayer := range errorLayers {
			// Print error at this layer with indent
			fmt.Printf("%s%s\n", strings.Repeat(" ", indentSpaces), errorLayer)

			// Increase indent for next line
			indentSpaces += 2
		}
	}

	fmt.Printf("\nPlease fix the errors, then run the following command to redeploy OR create new commit if file corrections are needed:\n")
	fmt.Printf("%s -c %s --manual-deploy --use-failtracker-only\n", PathToExe, configFilePath)

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

	fmt.Print("================================================\n")
	return
}

// Does a couple things
//
//	Moves into repository directory if not already
//	Checks for active network interfaces (can't deploy to remote endpoints if no network)
//	Loads known_hosts file into global variable
func localSystemChecks() (err error) {
	// Ensure current working directory is root of git repository from config
	pwd, err := os.Getwd()
	if err != nil {
		err = fmt.Errorf("failed to obtain current working directory: %v", err)
		return
	}

	// If current directory is not repo, change to it
	if filepath.Clean(pwd) != filepath.Clean(RepositoryPath) {
		err = os.Chdir(RepositoryPath)
		if err != nil {
			err = fmt.Errorf("failed to change directory to repository path: %v", err)
			return
		}
	}

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

	// Check if known hosts file exists
	_, err = os.Stat(knownHostsFilePath)
	if os.IsNotExist(err) {
		var knownFile *os.File
		// Known hosts file does not exist, create it
		knownFile, err = os.Create(knownHostsFilePath)
		if err != nil {
			err = fmt.Errorf("failed to create known_hosts file at '%s'", knownHostsFilePath)
			return
		}
		defer knownFile.Close()
	} else if err != nil {
		err = fmt.Errorf("failed to create known_hosts file at %s", knownHostsFilePath)
		return
	}

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

func printDeploymentInformation(targetEndpoints []string, hostsAndFilePaths map[string][]string, hostsAndEndpointInfo map[string]EndpointInfo, commitFileInfo map[string]CommitFileInfo) {
	// Notify user that program is in dry run mode
	fmt.Printf("\nRequested dry-run, aborting deployment - outputting information collected for deployment:\n\n")

	// Print deployment info by host
	for _, endpointName := range targetEndpoints {
		hostInfo := hostsAndEndpointInfo[endpointName]
		// Try to avoid printing whole passwords to stdout
		var truncatedSudoPass string
		if len(hostInfo.SudoPassword) > 6 {
			truncatedSudoPass = hostInfo.SudoPassword[:6]
		}
		truncatedSudoPass += "..."

		// Print out information for this specific host
		fmt.Printf("Host: %s\n", endpointName)
		fmt.Printf("  Options:\n")
		fmt.Printf("       Endpoint Address: %s\n", hostInfo.Endpoint)
		fmt.Printf("       SSH User:         %s\n", hostInfo.EndpointUser)
		fmt.Printf("       SSH Key:          %s\n", hostInfo.PrivateKey.PublicKey())
		fmt.Printf("       Sudo Password:    %s\n", truncatedSudoPass)
		fmt.Printf("       Transfer Buffer:  %s\n", hostInfo.RemoteTransferBuffer)
		fmt.Printf("       Backup Dir:       %s\n", hostInfo.RemoteBackupDir)
		fmt.Printf("  Files:\n")

		// Identify maximum indent file name prints will need to be
		var maxFileNameLength int
		for _, filePath := range hostsAndFilePaths[endpointName] {
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
		for _, file := range hostsAndFilePaths[endpointName] {
			// Format to remote path type
			_, targetFile := separateHostDirFromPath(file)

			// Determine how many spaces to add after file name
			indentSpaces := maxFileNameLength - len(targetFile)

			// Print what we are going to do, the local file path, and remote file path
			fmt.Printf("       %s:           %s%s(%s)\n", commitFileInfo[file].Action, targetFile, strings.Repeat(" ", indentSpaces), file)
		}
	}
}
