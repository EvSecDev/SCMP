// controller
package main

import (
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
	fmt.Printf("Failure(s) in deployment:\n")
	fmt.Printf("%s\n", FailTracker)
	fmt.Printf("Please fix the errors, then run the following command to redeploy (or create new commit if file corrections are needed):\n")
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
