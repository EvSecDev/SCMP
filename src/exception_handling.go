// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/coreos/go-systemd/journal"
)

// ###################################
//      EXCEPTION HANDLING
// ###################################

// Logs non-nil errors to stdout and journal(if requested in conf)
// If cleanup is needed, will roll the git repository back one commit
// Rollbacks should only be requested when entire program is not concurrent (i.e. before deploy go routines)
func logError(errorDescription string, errorMessage error, cleanupNeeded bool) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}
	// Attempt to put error in journald
	err := CreateJournaldLog(fmt.Sprintf("%s: %v", errorDescription, errorMessage), "err")
	if err != nil {
		fmt.Printf("Failed to create journald entry: %v\n", err)
	}

	// Print the error
	fmt.Printf("%s: %v\n", errorDescription, errorMessage)

	// Only roll back commit if the program was started by a hook and if the commit rollback is requested
	// Reset commit because the current commit should reflect what is deployed in the network
	// Conceptually, the rough equivalent of this command: git reset --soft HEAD~1
	if calledByGitHook && cleanupNeeded {
		err = gitRollBackOneCommit()
		if err != nil {
			fmt.Printf("Error rolling back commit. %v\n", err)
			os.Exit(1)
		}

		// Tell user how to continue
		fmt.Printf("Please fix the above error then `git add` and `git commit` to restart deployment process.\n")
	}

	os.Exit(1)
}

// Create log entry in journald
func CreateJournaldLog(errorMessage string, requestedPriority string) (err error) {
	// Priority by request input
	msgPriority := journal.PriAlert
	if requestedPriority == "err" {
		msgPriority = journal.PriErr
	} else if requestedPriority == "info" {
		msgPriority = journal.PriInfo
	} else {
		// No priority, dont create a log entry
		return
	}

	// Send entry to journald
	err = journal.Send(errorMessage, msgPriority, nil)
	if err != nil {
		// Don't send error back if journald is unavailable
		if strings.Contains(err.Error(), "could not initialize socket") {
			err = nil
		}
	}
	return
}

// Called from within go routines
// Creates JSON line of error host, files, and err
// Writes into global failure tracker
// Always returns
func recordDeploymentFailure(endpointName string, allFileArray []string, fileIndex int, errorMessage error) {
	// Ensure multiline error messages dont make their way into json
	message := errorMessage.Error()
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\r", " ")

	// Array to hold files that failed
	var fileArray []string

	// Determine which file to add to array
	if fileIndex < 0 {
		// Add all files to failtracker if host failed early (index -1)
		fileArray = allFileArray
	} else {
		// Specific file that failed
		fileArray = append(fileArray, allFileArray[fileIndex])
	}

	// Parseable one line json for failures
	info := ErrorInfo{
		EndpointName: endpointName,
		Files:        fileArray,
		ErrorMessage: message,
	}

	// Marshal info string to a json format
	failedInfo, err := json.Marshal(info)
	if err != nil {
		printMessage(verbosityStandard, "Failed to create Fail Tracker Entry for host %s file(s) %v\n", endpointName, fileArray)
		printMessage(verbosityStandard, "    Error: %s\n", message)
		return
	}

	// Send error to journald
	err = CreateJournaldLog(string(failedInfo), "err")
	if err != nil {
		printMessage(verbosityStandard, "Failed to create journald entry: %v\n", err)
	}

	// Write (append) fail info for this go routine to global failures - dont conflict with other host go routines
	failTracker.mutex.Lock()
	failTracker.buffer.WriteString(string(failedInfo) + "\n")
	failTracker.mutex.Unlock()
}
