// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

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
		fmt.Printf("Failed to %s git post-commit hook: (%v)\n", toggleAction, err)
	} else {
		fmt.Printf("Git post-commit hook %sd.\n", toggleAction)
	}
}
