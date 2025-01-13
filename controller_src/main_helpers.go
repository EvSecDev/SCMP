// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/term"
)

// Print message to stdout
// Message will only print if the global verbosity level is equal to or smaller than requiredVerbosityLevel
// Can directly take variables as values to print just like fmt.Printf
func printMessage(requiredVerbosityLevel int, message string, vars ...interface{}) {
	// No output for verbosity level 0
	if globalVerbosityLevel == 0 {
		return
	}

	// Attempt to put only verbosity level 1 in journald
	if globalVerbosityLevel == 1 && requiredVerbosityLevel == 1 {
		err := CreateJournaldLog(fmt.Sprintf(message, vars...), "info")
		if err != nil {
			fmt.Printf("Failed to create journald entry: %v\n", err)
		}
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

// Parse out options from config file
func parseConfig() (err error) {
	// Config agnostic configuration options
	OSPathSeparator = string(os.PathSeparator)
	SHA256RegEx = regexp.MustCompile(`^[a-fA-F0-9]{64}`)
	SHA1RegEx = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	userHomeDirectory, err = os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("unable to find home directory: %v", err)
		return
	}

	// Load Config File
	configFile, err := os.ReadFile(expandHomeDirectory(configFilePath))
	if err != nil {
		err = fmt.Errorf("reading config failed: %v", err)
		return
	}
	configContents := string(configFile)

	// Retrieve SSH Config file options
	config, err = ssh_config.Decode(strings.NewReader(configContents))
	if err != nil {
		err = fmt.Errorf("failed decoding config file: %v", err)
		return
	}

	// Set globals - see global section at top for descriptions
	knownHostsFilePath, _ = config.Get("*", "UserKnownHostsFile")
	if knownHostsFilePath == "" {
		err = fmt.Errorf("known_hosts file path must be present")
		return
	}

	// Format known_hosts path correctly
	knownHostsFilePath = expandHomeDirectory(knownHostsFilePath)

	// Ensure known_hosts file exists, if not create it
	_, err = os.Stat(knownHostsFilePath)
	if os.IsNotExist(err) {
		var knownHostsFile *os.File
		knownHostsFile, err = os.Create(knownHostsFilePath)
		if err != nil {
			return
		}
		knownHostsFile.Close()
	} else if err != nil {
		return
	}

	// Get current dir (expected to be root of git repo)
	currentWorkingDir, err := os.Getwd()
	if err != nil {
		return
	}
	expectedDotGitPath := filepath.Join(currentWorkingDir, ".git")

	// Error if .git directory is not present in current directory
	_, err = os.Stat(expectedDotGitPath)
	if os.IsNotExist(err) {
		err = fmt.Errorf("not in a git repository, unable to deploy")
		return
	} else if err != nil {
		return
	}
	// Current dir is absolute git repo path
	RepositoryPath = currentWorkingDir

	// All config dir names in repo
	UniversalDirectory, _ = config.Get("", "UniversalDirectory")
	if strings.Contains(UniversalDirectory, OSPathSeparator) {
		err = fmt.Errorf("UniversalDirectory should be a relative path from the root of repository")
		return
	}

	// Ignored Dirs in repo
	IgnoreDirectoryNames, _ := config.Get("", "IgnoreDirectories")
	IgnoreDirectories = strings.Split(IgnoreDirectoryNames, ",")
	if strings.Contains(IgnoreDirectoryNames, OSPathSeparator) {
		err = fmt.Errorf("IgnoreDirectories should be relative paths from the root of repository")
		return
	}

	// Check maxconns is valid
	if MaxSSHConcurrency == 0 {
		err = fmt.Errorf("max connections cannot be 0")
		return
	}

	// Array of Hosts Names
	hostsRequireVault = make(map[string]struct{})
	for _, host := range config.Hosts {
		// Skip host patterns with more than one pattern
		if len(host.Patterns) != 1 {
			continue
		}

		// Convert host pattern to string
		hostPattern := host.Patterns[0].String()

		// Create list of hosts that would need vault access
		PasswordRequired, _ := config.Get(hostPattern, "PasswordRequired")
		if strings.ToLower(PasswordRequired) == "yes" {
			hostsRequireVault[hostPattern] = struct{}{}
		}

		// If a wildcard pattern, skip
		if strings.Contains(hostPattern, "*") {
			continue
		}

		DeployerEndpoints = append(DeployerEndpoints, hostPattern)
	}

	// Password vault file
	vaultRelPath, _ := config.Get("", "PasswordVault")
	vaultFilePath = expandHomeDirectory(vaultRelPath)

	// Group dir names in repo
	GroupDirectories, _ := config.Get("", "GroupDirs")
	if strings.Contains(GroupDirectories, OSPathSeparator) {
		err = fmt.Errorf("GroupDirs should be relative paths from the root of repository")
		return
	}
	GroupNames := strings.Split(GroupDirectories, ",")

	// Create map of groups and all hosts that belong to that group
	UniversalGroups = make(map[string][]string)
	for _, GroupName := range GroupNames {
		for _, endpointName := range DeployerEndpoints {
			hostGroups, _ := config.Get(endpointName, "GroupTags")
			hostGroupList := strings.Split(hostGroups, ",")
			for _, hostGroup := range hostGroupList {
				if hostGroup != GroupName {
					continue
				}
				UniversalGroups[GroupName] = append(UniversalGroups[GroupName], endpointName)
			}
		}
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

// Ensures variables that contains paths do not have '~/' and is replaced with absolute path
func expandHomeDirectory(path string) (absolutePath string) {
	// Return early if path doesn't have '~/' prefix
	if !strings.HasPrefix(path, "~/") {
		return
	}

	// Remove '~/' prefixes
	path = strings.TrimPrefix(path, "~/")

	// Combine Users home directory path with the input path
	absolutePath = filepath.Join(userHomeDirectory, path)
	return
}

// Prompts user to enter something
func promptUser(userPrompt string, printVars ...interface{}) (userResponse string, err error) {
	// Throw error if not in terminal - stdin not available outside terminal for users
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	printMessage(VerbosityStandard, userPrompt, printVars...)
	fmt.Scanln(&userResponse)
	userResponse = strings.ToLower(userResponse)
	return
}

// Prompts user for a secret value (does not echo back entered text)
func promptUserForSecret(userPrompt string, printVars ...interface{}) (userResponse string, err error) {
	// Create PTY if not in terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	// Regular prompt
	printMessage(VerbosityStandard, userPrompt, printVars...)
	userResponseBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return
	}

	// Convert to string for return
	userResponse = string(userResponseBytes)
	fmt.Println()
	return
}
