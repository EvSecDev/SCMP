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
}

// Parse out options from config file into global
func parseConfig() (err error) {
	// Config agnostic configuration options
	config.OSPathSeparator = string(os.PathSeparator)
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

	printMessage(VerbosityProgress, "Retrieving Configurations for Hosts\n")

	// Array of Hosts and their info
	config.HostInfo = make(map[string]EndpointInfo)
	config.AllUniversalGroups = make(map[string][]string)
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

		printMessage(VerbosityData, "    Retrieving Host Ignore Universal State\n")

		// Get all groups this host is a part of
		universalGroupsCSV, _ := sshConfig.Get(hostPattern, "GroupTags")

		// Get yes/no if host ignores main universal
		ignoreUniversalString, _ := sshConfig.Get(hostPattern, "IgnoreUniversal")

		// Parse config host groups into necessary global/host variables
		HostIgnoresUniversal, HostUniversalGroups := filterHostGroups(hostPattern, universalGroupsCSV, ignoreUniversalString)
		hostInfo.IgnoreUniversal = HostIgnoresUniversal
		hostInfo.UniversalGroups = HostUniversalGroups

		// Save host back into global map
		config.HostInfo[hostPattern] = hostInfo
	}

	return
}

// Creates two maps relating to host groups
// First map: key'd on group and contains only groups that the host is a part of (values are empty)
// Second map: global key'd on group and contains array of hosts belonging to that group
func filterHostGroups(endpointName string, universalGroupsCSV string, ignoreUniversalString string) (HostIgnoresUniversal bool, HostUniversalGroups map[string]struct{}) {
	// Convert CSV of host groups to array
	universalGroupsList := strings.Split(universalGroupsCSV, ",")

	// If host ignores universal configs
	if strings.ToLower(ignoreUniversalString) == "yes" {
		HostIgnoresUniversal = true
	} else {
		HostIgnoresUniversal = false

		// Not ignoring, make this host a part of the universal group
		universalGroupsList = append(universalGroupsList, config.UniversalDirectory)
	}

	// Get universal groups this host is a part of
	HostUniversalGroups = make(map[string]struct{})
	for _, universalGroup := range universalGroupsList {
		// Skip empty hosts' group
		if universalGroup == "" {
			continue
		}

		// Map of groups that this host is a part of
		HostUniversalGroups[universalGroup] = struct{}{}

		// Add this hosts name to the global universal map for groups this host is a part of
		config.AllUniversalGroups[universalGroup] = append(config.AllUniversalGroups[universalGroup], endpointName)
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
