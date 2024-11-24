// controller
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/diff"
)

// ###################################
//      PARSING FUNCTIONS
// ###################################

// Checks for user-chosen host override with given host
func checkForHostOverride(hostOverride string, currentHost string) (SkipHost bool) {
	// Allow user override hosts
	if hostOverride != "" {
		userHostChoices := strings.Split(hostOverride, ",")
		for _, userHostChoice := range userHostChoices {
			if userHostChoice == currentHost {
				SkipHost = false
				return
			}
			SkipHost = true
		}
	}
	return
}

// Deduplicates and creates host endpoint information map
// Compares a hosts deployer endpoints info against the SSH client defaults
func retrieveEndpointInfo(endpointInfo DeployerEndpoints, SSHClientDefault SSHClientDefaults) (info EndpointInfo, err error) {
	// First item must be present (IP required, cannot use default)
	endpointAddr := endpointInfo.Endpoint
	if endpointAddr == "" {
		err = fmt.Errorf("endpoint address cannot be empty")
		return
	}

	// Get port from endpoint or if missing use default
	endpointPort := endpointInfo.EndpointPort
	if endpointPort == 0 {
		endpointPort = SSHClientDefault.EndpointPort
	}

	// Network Address Parsing
	info.Endpoint, err = ParseEndpointAddress(endpointAddr, endpointPort)
	if err != nil {
		err = fmt.Errorf("failed parsing network address: %v", err)
		return
	}

	// Get user from endpoint or if missing use default
	info.EndpointUser = endpointInfo.EndpointUser
	if info.EndpointUser == "" {
		info.EndpointUser = SSHClientDefault.EndpointUser
	}

	// Get identity file from endpoint or if missing use default
	identityFile := endpointInfo.SSHIdentityFile
	if identityFile == "" {
		identityFile = SSHClientDefault.SSHIdentityFile
	}

	// Get sshagent bool from endpoint or if missing use default
	var useSSHAgent bool
	if endpointInfo.UseSSHAgent != nil {
		useSSHAgent = *endpointInfo.UseSSHAgent
	} else {
		useSSHAgent = SSHClientDefault.UseSSHAgent
	}

	// Get SSH Private Key from the supplied identity file
	info.PrivateKey, info.KeyAlgo, err = SSHIdentityToKey(identityFile, useSSHAgent)
	if err != nil {
		err = fmt.Errorf("failed to retrieve private key: %v", err)
		return
	}

	// Get sudo password from endpoint or if missing use default
	info.SudoPassword = endpointInfo.SudoPassword
	if info.SudoPassword == "" {
		info.SudoPassword = SSHClientDefault.SudoPassword
	}

	// Get remote transfer buffer file path from endpoint or if missing use default
	info.RemoteTransferBuffer = endpointInfo.RemoteTransferBuffer
	if info.RemoteTransferBuffer == "" {
		info.RemoteTransferBuffer = SSHClientDefault.RemoteTransferBuffer
	}

	return
}

// Function to extract and validate metadata JSON from file contents
func extractMetadata(fileContents string) (metadataSection string, remainingContent string, err error) {
	// Add newline so file content doesnt have empty line at the top
	EndDelimiter := Delimiter + "\n"

	// Find the start and end of the metadata section
	startIndex := strings.Index(fileContents, Delimiter)
	if startIndex == -1 {
		err = fmt.Errorf("json start delimter missing")
		return
	}
	startIndex += len(Delimiter)

	endIndex := strings.Index(fileContents[startIndex:], EndDelimiter)
	if endIndex == -1 {
		TestEndIndex := strings.Index(fileContents[startIndex:], Delimiter)
		if TestEndIndex == -1 {
			err = fmt.Errorf("no newline after json end delimiter")
			return
		}
		err = fmt.Errorf("json end delimiter missing ")
		return
	}
	endIndex += startIndex

	// Extract the metadata section and remaining content into their own vars
	metadataSection = fileContents[startIndex:endIndex]
	remainingContent = fileContents[:startIndex-len(Delimiter)] + fileContents[endIndex+len(EndDelimiter):]

	return
}

// Ensures files in the new commit are valid
// Invalid files include
//
//	non-existent
//	unsupported file type (device, socket, pipe, ect)
//	any files in the root of the repository
//	dirs present in global ignoredirectories array
//	dirs that do not have a match in the controllers config
func validateCommittedFiles(commitHostNames *[]string, DeployerEndpoints map[string]DeployerEndpoints, rawFile diff.File) (path string, FileType string, SkipFile bool, err error) {
	// Nothing to validate
	if rawFile == nil {
		return
	}

	// Retrieve integer representation of the files mode
	mode := fmt.Sprintf("%v", rawFile.Mode())

	// Retrieve the type for this file
	FileType = determineFileType(mode)

	// Skip processing if file is unsupported
	if FileType == "unsupported" {
		SkipFile = true
		return
	}

	// Get the path
	path = rawFile.Path()

	// File exists, but no path - technically valid
	if path == "" {
		return
	}

	// Always ignore files in root of repository
	if !strings.ContainsRune(path, []rune(OSPathSeparator)[0]) {
		SkipFile = true
		return
	}

	// SkipFile if inside ignore directories array
	if len(IgnoreDirectories) > 0 {
		// Get just the dirs
		commitDir := filepath.Dir(path)

		// When committed file directory is prefixed by an ignore directory, skip file
		for _, ignoreDir := range IgnoreDirectories {
			if strings.HasPrefix(commitDir, ignoreDir) {
				SkipFile = true
				return
			}
		}
	}

	// Retrieve the host directory name for this file
	fileDirNames := strings.SplitN(path, OSPathSeparator, 2)
	hostDirName := fileDirNames[0]

	// Ensure the commit host directory name is a valid hostname in yaml DeployerEndpoints
	var NoHostMatch bool
	for availableHost := range DeployerEndpoints {
		if hostDirName == availableHost || hostDirName == UniversalDirectory {
			NoHostMatch = false
			break
		}
		NoHostMatch = true
	}
	if NoHostMatch {
		err = fmt.Errorf("repository host directory '%s' has no matching host in YAML config", hostDirName)
	}

	// Check if host dir is already in the slice (avoid adding many duplicates to slice
	var HostAlreadyPresent bool
	for _, host := range *commitHostNames {
		if host == hostDirName {
			HostAlreadyPresent = true
		}
	}

	// Add the hosts directory name to the slice to filter deployer endpoints later
	if !HostAlreadyPresent {
		*commitHostNames = append(*commitHostNames, hostDirName)
	}

	return
}

// Determines which file types in the commit are allowed to be deployed
// Marks file type based on mode
func determineFileType(fileMode string) (fileType string) {
	// Set type of file in commit - skip unsupported
	if fileMode == "0100644" {
		// Text file
		fileType = "regular"
	} else if fileMode == "0120000" {
		// Special, but able to be handled
		fileType = "symlink"
	} else if fileMode == "0040000" {
		// Directory
		fileType = "unsupported"
	} else if fileMode == "0160000" {
		// Git submodule
		fileType = "unsupported"
	} else if fileMode == "0100755" {
		// Executable
		fileType = "unsupported"
	} else if fileMode == "0100664" {
		// Deprecated
		fileType = "unsupported"
	} else if fileMode == "0" {
		// Empty (no file)
		fileType = "unsupported"
	} else {
		// Unknown - dont process
		fileType = "unsupported"
	}

	return
}

// Retrieve the symbolic link target path and check for validity
// Valid link means the links target is not outside of the link's host directory
func ResolveLinkToTarget(filePath string) (targetPath string, err error) {
	// Get link target path
	linkTarget, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return
	}

	// Get top level directory name for sym link and target
	targetPathArray := strings.SplitN(linkTarget, OSPathSeparator, 2)
	linkPathArray := strings.SplitN(filePath, OSPathSeparator, 2)

	// Error if link top level directories are not the same (link is between host directories)
	if targetPathArray[0] != linkPathArray[0] {
		err = fmt.Errorf("cannot have symbolic link between host directories")
		return
	}

	// Return target path without top level directory name (host dir name) (this is remote host format now)
	convertedPath := strings.ReplaceAll(targetPathArray[1], OSPathSeparator, "/")
	targetPath = "/" + convertedPath
	return
}
