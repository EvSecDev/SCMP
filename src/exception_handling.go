// controller
package main

import (
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

	// Append message to global log
	config.eventLogMutex.Lock()
	config.eventLog = append(config.eventLog, fmt.Sprintf("%s: %v", errorDescription, errorMessage))
	config.eventLogMutex.Unlock()

	// Write global logs to disk
	if config.logFile != nil {
		defer config.logFile.Close()

		allEvents := strings.Join(config.eventLog, "")
		_, err = config.logFile.WriteString(allEvents + "\n")
		if err != nil {
			fmt.Printf("Failed to write to log file: %v\n", err)
		}
	}

	// Print the error
	fmt.Fprintf(os.Stderr, "%s: %v\n", errorDescription, errorMessage)

	// Only roll back commit if the program was started by a hook and if the commit rollback is requested
	// Reset commit because the current commit should reflect what is deployed in the network
	// Conceptually, the rough equivalent of this command: git reset --soft HEAD~1
	if config.options.calledByGitHook && cleanupNeeded {
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
	// Return early when journal is not available
	if !journal.Enabled() {
		return
	}

	// Priority by request input
	msgPriority := journal.PriAlert
	switch requestedPriority {
	case "err":
		msgPriority = journal.PriErr
	case "info":
		msgPriority = journal.PriInfo
	default:
		// No priority, don't create a log entry
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
