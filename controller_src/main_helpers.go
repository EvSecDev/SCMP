// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Print message to stdout
// Message will only print if the global verbosity level is equal to or smaller than requiredVerbosityLevel
// Can directly take variables as values to print just like fmt.Printf
func printMessage(requiredVerbosityLevel int, message string, vars ...interface{}) {
	// No output for verbosity level 0
	if globalVerbosityLevel == 0 {
		return
	}

	// Add timestamps to verbosity levels 2 and up (but only when the timestamp will get printed)
	if globalVerbosityLevel >= 2 && requiredVerbosityLevel <= globalVerbosityLevel {
		currentTime := time.Now()
		timestamp := currentTime.Format("15:04:05.000000")
		message = timestamp + ": " + message
	}

	// Required stdout message verbosity level is equal to or less than global verbosity level
	if requiredVerbosityLevel <= globalVerbosityLevel {
		fmt.Printf(message, vars...)
	}
}

// Ensure config is not missing required fields
func checkConfigForEmpty(config *Config) (err error) {
	if config.Controller.RepositoryPath == "" {
		err = fmt.Errorf("RepositoryPath")
	} else if config.SSHClient.KnownHostsFile == "" {
		err = fmt.Errorf("KnownHostsFile")
	} else if config.SSHClient.MaximumConcurrency == 0 {
		err = fmt.Errorf("MaximumConcurrency")
	} else if config.UniversalDirectory == "" {
		err = fmt.Errorf("UniversalDirectory")
	}
	return
}

// Enables or disables git post-commit hook by moving the post-commit file
// Takes 'enable' or 'disable' as toggle action
func toggleGitHook(toggleAction string) {
	// Path to enabled/disabled git hook files
	enabledGitHookFile := filepath.Join(RepositoryPath, ".git", "hooks", "post-commit")
	disabledGitHookFile := enabledGitHookFile + ".disabled"

	// Determine how to move file
	var srcFile, dstFile string
	if toggleAction == "enable" {
		srcFile = disabledGitHookFile
		dstFile = enabledGitHookFile
	} else if toggleAction == "disable" {
		srcFile = enabledGitHookFile
		dstFile = disabledGitHookFile
	} else {
		// Refuse to do anything without correct toggle action
		return
	}

	// Check presence of destination file
	_, err := os.Stat(dstFile)

	// Move src to dst only if dst isn't present
	if os.IsNotExist(err) {
		err = os.Rename(srcFile, dstFile)
	}

	// Show progress to user depending on error presence
	if err != nil {
		logError(fmt.Sprintf("Failed to %s git post-commit hook", toggleAction), err, false)
	} else {
		printMessage(VerbosityStandard, "Git post-commit hook %sd.\n", toggleAction)
	}
}
