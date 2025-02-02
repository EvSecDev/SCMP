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

// Parse out options from config file into global
func parseConfig() (err error) {
	// Config agnostic configuration options
	config.OSPathSeparator = string(os.PathSeparator)
	SHA256RegEx = regexp.MustCompile(`^[a-fA-F0-9]{64}`)
	SHA1RegEx = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	config.UserHomeDirectory, err = os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("unable to find home directory: %v", err)
		return
	}

	// Load Config File
	configAbsolutePath := expandHomeDirectory(config.FilePath)
	sshConfigFile, err := os.ReadFile(configAbsolutePath)
	if err != nil {
		err = fmt.Errorf("reading config failed: %v", err)
		return
	}
	sshConfigContents := string(sshConfigFile)

	// Retrieve SSH Config file options
	sshConfig, err := ssh_config.Decode(strings.NewReader(sshConfigContents))
	if err != nil {
		err = fmt.Errorf("failed decoding config file: %v", err)
		return
	}

	printMessage(VerbosityProgress, "Retrieving known_hosts file contents\n")

	// Set globals - see global section at top for descriptions

	// Set path to failtracker file (in config directory)
	configDirectory := filepath.Dir(configAbsolutePath)
	config.FailTrackerFilePath = filepath.Join(configDirectory, FailTrackerFile)

	// Retrieve known_hosts file path
	config.KnownHostsFilePath, _ = sshConfig.Get("*", "UserKnownHostsFile")
	if config.KnownHostsFilePath == "" {
		err = fmt.Errorf("known_hosts file path must be present")
		return
	}

	// Format known_hosts path correctly
	config.KnownHostsFilePath = expandHomeDirectory(config.KnownHostsFilePath)

	// Ensure known_hosts file exists, if not create it
	_, err = os.Stat(config.KnownHostsFilePath)
	if os.IsNotExist(err) {
		var knownHostsFile *os.File
		knownHostsFile, err = os.Create(config.KnownHostsFilePath)
		if err != nil {
			return
		}
		knownHostsFile.Close()
	} else if err != nil {
		return
	}

	// Read in file
	knownHostFile, err := os.ReadFile(config.KnownHostsFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read known_hosts file: %v", err)
		return
	}

	// Store all known_hosts as array
	config.KnownHosts = strings.Split(string(knownHostFile), "\n")

	// All config dir names in repo
	config.UniversalDirectory, _ = sshConfig.Get("", "UniversalDirectory")
	if strings.Contains(config.UniversalDirectory, config.OSPathSeparator) {
		err = fmt.Errorf("UniversalDirectory should be a relative path from the root of repository")
		return
	}

	printMessage(VerbosityProgress, "Retrieving ignored directories config option\n")

	// Ignored Dirs in repo
	IgnoreDirectoryNames, _ := sshConfig.Get("", "IgnoreDirectories")
	config.IgnoreDirectories = strings.Split(IgnoreDirectoryNames, ",")
	if strings.Contains(IgnoreDirectoryNames, config.OSPathSeparator) {
		err = fmt.Errorf("IgnoreDirectories should be relative paths from the root of repository")
		return
	}

	// Check maxconns is valid
	if config.MaxSSHConcurrency == 0 {
		err = fmt.Errorf("max connections cannot be 0")
		return
	}

	// Password vault file
	vaultRelPath, _ := sshConfig.Get("", "PasswordVault")
	config.VaultFilePath = expandHomeDirectory(vaultRelPath)

	// Initialize vault map
	config.Vault = make(map[string]Credential)

	// Group dir names in repo
	GroupDirectories, _ := sshConfig.Get("", "GroupDirs")
	if strings.Contains(GroupDirectories, config.OSPathSeparator) {
		err = fmt.Errorf("GroupDirs should be relative paths from the root of repository")
		return
	}
	GroupNames := strings.Split(GroupDirectories, ",")

	// Create list of all universal group names
	config.AllUniversalGroups = make(map[string]struct{})
	for _, GroupName := range GroupNames {
		config.AllUniversalGroups[GroupName] = struct{}{}
	}

	printMessage(VerbosityProgress, "Retrieving Configurations for Hosts\n")

	// Array of Hosts and their info
	config.HostInfo = make(map[string]EndpointInfo)
	var hostInfo EndpointInfo
	for _, host := range sshConfig.Hosts {
		// Skip host patterns with more than one pattern
		if len(host.Patterns) != 1 {
			continue
		}

		// Convert host pattern to string
		hostPattern := host.Patterns[0].String()

		// If a wildcard pattern, skip
		if strings.Contains(hostPattern, "*") {
			continue
		}

		printMessage(VerbosityData, "  Host: %s\n", hostPattern)

		// Save hostname into info map
		hostInfo.EndpointName = hostPattern

		printMessage(VerbosityData, "    Retrieving Username\n")

		// Save user into info map
		hostInfo.EndpointUser, _ = sshConfig.Get(hostPattern, "User")

		printMessage(VerbosityData, "    Retrieving Address\n")

		// First item must be present
		endpointAddr, _ := sshConfig.Get(hostPattern, "Hostname")

		printMessage(VerbosityData, "    Retrieving Port\n")

		// Get port from endpoint
		endpointPort, _ := sshConfig.Get(hostPattern, "Port")

		// Network Address Parsing - only if address
		if endpointAddr != "" && endpointPort != "" {
			printMessage(VerbosityData, "    Parsing endpoint address\n")

			hostInfo.Endpoint, err = ParseEndpointAddress(endpointAddr, endpointPort)
			if err != nil {
				err = fmt.Errorf("failed parsing network address: %v", err)
				return
			}
		}

		printMessage(VerbosityData, "    Retrieving Identity File Path\n")

		// Get identity file path
		hostInfo.IdentityFile, _ = sshConfig.Get(hostPattern, "IdentityFile")

		printMessage(VerbosityData, "    Retrieving Remote Temp Dirs\n")

		// Save remote transfer buffer and backup dir into host info map
		hostInfo.RemoteBackupDir, _ = sshConfig.Get(hostPattern, "RemoteBackupDir")
		hostInfo.RemoteTransferBuffer, _ = sshConfig.Get(hostPattern, "RemoteTransferBuffer")

		// Ensure trailing slashes don't make their way into the path
		hostInfo.RemoteTransferBuffer = strings.TrimSuffix(hostInfo.RemoteTransferBuffer, "/")

		printMessage(VerbosityData, "    Retrieving Deployment State\n")

		// Save deployment state of this host
		hostInfo.DeploymentState, _ = sshConfig.Get(hostPattern, "DeploymentState")

		printMessage(VerbosityData, "    Retrieving Host Ignore Universal State\n")

		// If host ignores universal configs
		ignoreUniversalString, _ := sshConfig.Get(hostPattern, "IgnoreUniversal")
		if strings.ToLower(ignoreUniversalString) == "yes" {
			hostInfo.IgnoreUniversal = true
		} else {
			hostInfo.IgnoreUniversal = false
		}

		// Get universal groups this host is a part of
		// Makes for easy quick lookups if host is part of a group
		universalGroupsCSV, _ := sshConfig.Get(hostPattern, "GroupTags")
		universalGroupsList := strings.Split(universalGroupsCSV, ",")
		hostInfo.UniversalGroups = make(map[string]struct{})
		for _, universalGroup := range universalGroupsList {
			hostInfo.UniversalGroups[universalGroup] = struct{}{}
		}

		printMessage(VerbosityData, "    Retrieving if host requires vault password\n")

		// Create list of hosts that would need vault access
		PasswordRequired, _ := sshConfig.Get(hostPattern, "PasswordRequired")
		if strings.ToLower(PasswordRequired) == "yes" {
			printMessage(VerbosityData, "     Host requires vault password\n")
			hostInfo.RequiresVault = true
		} else {
			printMessage(VerbosityData, "     Host does not require vault password\n")
			hostInfo.RequiresVault = false
		}

		// Save host back into global map
		config.HostInfo[hostPattern] = hostInfo
	}

	return
}

func retrieveGitRepoPath() (err error) {
	printMessage(VerbosityProgress, "Retrieving repository file path\n")

	// Get current dir (expected to be root of git repo)
	currentWorkingDir, err := os.Getwd()
	if err != nil {
		return
	}
	expectedDotGitPath := filepath.Join(currentWorkingDir, ".git")

	// Error if .git directory is not present in current directory
	_, err = os.Stat(expectedDotGitPath)
	if os.IsNotExist(err) {
		err = fmt.Errorf("not in a git repository, unable to continue")
		return
	} else if err != nil {
		return
	}

	// Current dir is absolute git repo path
	config.RepositoryPath = currentWorkingDir
	return
}

// Enables or disables git post-commit hook by moving the post-commit file
// Takes 'enable' or 'disable' as toggle action
func toggleGitHook(toggleAction string) {
	// Path to enabled/disabled git hook files
	enabledGitHookFile := filepath.Join(config.RepositoryPath, ".git", "hooks", "post-commit")
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
		absolutePath = path
		return
	}

	// Remove '~/' prefixes
	path = strings.TrimPrefix(path, "~/")

	// Combine Users home directory path with the input path
	absolutePath = filepath.Join(config.UserHomeDirectory, path)
	return
}

// Prompts user to enter something
func promptUser(userPrompt string, printVars ...interface{}) (userResponse string, err error) {
	// Throw error if not in terminal - stdin not available outside terminal for users
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	fmt.Printf(userPrompt, printVars...)
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
	fmt.Printf(userPrompt, printVars...)
	userResponseBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return
	}

	// Convert to string for return
	userResponse = string(userResponseBytes)
	fmt.Println()
	return
}
