// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
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

	// Only use global buffer when log file is present
	if config.logFile != nil && requiredVerbosityLevel <= globalVerbosityLevel {
		// Append message to global log
		config.eventLogMutex.Lock()
		config.eventLog = append(config.eventLog, fmt.Sprintf(message, vars...))
		config.eventLogMutex.Unlock()
	}
}

// Parse out options from config file into global
func (config *Config) extractOptions(configFilePath string) (err error) {
	const failTrackerFile string = ".scmp-failtracker.json" // file name for recording deployment failure details

	// Config agnostic configuration options
	config.osPathSeparator = string(os.PathSeparator)
	config.userHomeDirectory, err = os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("unable to find home directory: %v", err)
		return
	}

	// Load Config File
	config.filePath = expandHomeDirectory(configFilePath)
	sshConfigFile, err := os.ReadFile(config.filePath)
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

	// Set globals - see global section at top for descriptions

	// Open log file
	if config.logFilePath != "" {
		config.logFile, err = os.OpenFile(config.logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
	}

	// Set path to failtracker file (in config directory)
	configDirectory := filepath.Dir(config.filePath)
	config.failTrackerFilePath = filepath.Join(configDirectory, failTrackerFile)

	// Retrieve known_hosts file path
	config.knownHostsFilePath, _ = sshConfig.Get("*", "UserKnownHostsFile")
	if config.knownHostsFilePath == "" {
		err = fmt.Errorf("known_hosts file path must be present")
		return
	}

	// Format known_hosts path correctly
	config.knownHostsFilePath = expandHomeDirectory(config.knownHostsFilePath)

	// Ensure known_hosts file exists, if not create it
	_, err = os.Stat(config.knownHostsFilePath)
	if os.IsNotExist(err) {
		var knownHostsFile *os.File
		knownHostsFile, err = os.Create(config.knownHostsFilePath)
		if err != nil {
			return
		}
		knownHostsFile.Close()
	} else if err != nil {
		return
	}

	// Read in file
	knownHostFile, err := os.ReadFile(config.knownHostsFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read known_hosts file: %v", err)
		return
	}

	// Store all known_hosts as array
	config.knownHosts = strings.Split(string(knownHostFile), "\n")

	// All config dir names in repo
	config.universalDirectory, _ = sshConfig.Get("", "UniversalDirectory")
	if strings.Contains(config.universalDirectory, config.osPathSeparator) {
		err = fmt.Errorf("UniversalDirectory should be a relative path from the root of repository")
		return
	}

	// Ignored Dirs in repo
	ignoreDirectoryNames, _ := sshConfig.Get("", "IgnoreDirectories")
	config.ignoreDirectories = strings.Split(ignoreDirectoryNames, ",")
	if strings.Contains(ignoreDirectoryNames, config.osPathSeparator) {
		err = fmt.Errorf("IgnoreDirectories should be relative paths from the root of repository")
		return
	}

	// Check maxconns is valid
	if config.maxSSHConcurrency == 0 {
		err = fmt.Errorf("max connections cannot be 0")
		return
	}

	// Password vault file
	vaultRelPath, _ := sshConfig.Get("", "PasswordVault")
	config.vaultFilePath = expandHomeDirectory(vaultRelPath)

	// Initialize vault map
	config.vault = make(map[string]Credential)

	// Array of Hosts and their info
	config.hostInfo = make(map[string]EndpointInfo)
	config.allUniversalGroups = make(map[string][]string)
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

		// Save hostname into info map
		hostInfo.endpointName = hostPattern

		// Save user into info map
		hostInfo.endpointUser, _ = sshConfig.Get(hostPattern, "User")

		// First item must be present
		endpointAddr, _ := sshConfig.Get(hostPattern, "Hostname")

		// Get port from endpoint
		endpointPort, _ := sshConfig.Get(hostPattern, "Port")

		// Network Address Parsing - only if address
		if endpointAddr != "" && endpointPort != "" {
			hostInfo.endpoint, err = parseEndpointAddress(endpointAddr, endpointPort)
			if err != nil {
				err = fmt.Errorf("failed parsing network address: %v", err)
				return
			}
		}

		// Get proxy
		hostInfo.proxy, _ = sshConfig.Get(hostPattern, "ProxyJump")

		// Get identity file path
		hostInfo.identityFile, _ = sshConfig.Get(hostPattern, "IdentityFile")

		// Create list of hosts that would need vault access
		passwordRequired, _ := sshConfig.Get(hostPattern, "PasswordRequired")
		if strings.ToLower(passwordRequired) == "yes" {
			hostInfo.requiresVault = true
		} else {
			hostInfo.requiresVault = false
		}

		// Save remote transfer buffer and backup dir into host info map
		hostInfo.remoteBackupDir, _ = sshConfig.Get(hostPattern, "RemoteBackupDir")
		hostInfo.remoteTransferBuffer, _ = sshConfig.Get(hostPattern, "RemoteTransferBuffer")

		// Ensure trailing slashes don't make their way into the path
		hostInfo.remoteTransferBuffer = strings.TrimSuffix(hostInfo.remoteTransferBuffer, "/")

		// Save deployment state of this host
		hostInfo.deploymentState, _ = sshConfig.Get(hostPattern, "DeploymentState")

		// Get all groups this host is a part of
		universalGroupsCSV, _ := sshConfig.Get(hostPattern, "GroupTags")

		// Get yes/no if host ignores main universal
		ignoreUniversalString, _ := sshConfig.Get(hostPattern, "IgnoreUniversal")

		// Parse config host groups into necessary global/host variables
		hostInfo.ignoreUniversal, hostInfo.universalGroups = filterHostGroups(hostPattern, universalGroupsCSV, ignoreUniversalString)

		// write into config
		config.hostInfo[hostPattern] = hostInfo
	}

	return
}

// Creates two maps relating to host groups
// First map: key'd on group and contains only groups that the host is a part of (values are empty)
// Second map: global key'd on group and contains array of hosts belonging to that group
func filterHostGroups(endpointName string, universalGroupsCSV string, ignoreUniversalString string) (hostIgnoresUniversal bool, hostUniversalGroups map[string]struct{}) {
	// Convert CSV of host groups to array
	universalGroupsList := strings.Split(universalGroupsCSV, ",")

	// If host ignores universal configs
	if strings.ToLower(ignoreUniversalString) == "yes" {
		hostIgnoresUniversal = true
	} else {
		hostIgnoresUniversal = false

		// Not ignoring, make this host a part of the universal group
		universalGroupsList = append(universalGroupsList, config.universalDirectory)
	}

	// Get universal groups this host is a part of
	hostUniversalGroups = make(map[string]struct{})
	for _, universalGroup := range universalGroupsList {
		// Skip empty hosts' group
		if universalGroup == "" {
			continue
		}

		// Map of groups that this host is a part of
		hostUniversalGroups[universalGroup] = struct{}{}

		// Add this hosts name to the global universal map for groups this host is a part of
		config.allUniversalGroups[universalGroup] = append(config.allUniversalGroups[universalGroup], endpointName)
	}

	return
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
	absolutePath = filepath.Join(config.userHomeDirectory, path)
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
func promptUserForSecret(userPrompt string, printVars ...interface{}) (userResponse []byte, err error) {
	// Create PTY if not in terminal
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	// Regular prompt
	fmt.Printf(userPrompt, printVars...)
	userResponse, err = term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return
	}

	fmt.Println()
	return
}
