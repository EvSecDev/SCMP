// controller
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ###################################
//      PARSING FUNCTIONS
// ###################################

// Checks for user-chosen host/file override with given host/file
// Returns immediately if override is empty
func checkForOverride(override string, current string) (skip bool) {
	// Allow user override hosts or files
	if override != "" {
		userHostChoices := strings.Split(override, ",")
		for _, userChoice := range userHostChoices {
			// Don't skip if current is user choice
			if userChoice == current {
				skip = false
				return
			}
			skip = true
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

	printMessage(VerbosityFullData, "      Address: %s\n", endpointAddr)

	// Get port from endpoint or if missing use default
	endpointPort := endpointInfo.EndpointPort
	if endpointPort == 0 {
		endpointPort = SSHClientDefault.EndpointPort
	}

	printMessage(VerbosityFullData, "      Port: %d\n", endpointPort)

	// Network Address Parsing
	info.Endpoint, err = ParseEndpointAddress(endpointAddr, endpointPort)
	if err != nil {
		err = fmt.Errorf("failed parsing network address: %v", err)
		return
	}

	printMessage(VerbosityFullData, "      Socket: %s\n", info.Endpoint)

	// Get user from endpoint or if missing use default
	info.EndpointUser = endpointInfo.EndpointUser
	if info.EndpointUser == "" {
		info.EndpointUser = SSHClientDefault.EndpointUser
	}

	printMessage(VerbosityFullData, "      User: %s\n", info.EndpointUser)

	// Get identity file from endpoint or if missing use default
	identityFile := endpointInfo.SSHIdentityFile
	if identityFile == "" {
		identityFile = SSHClientDefault.SSHIdentityFile
	}

	printMessage(VerbosityFullData, "      SSH Identity File: %s\n", identityFile)

	// Get sshagent bool from endpoint or if missing use default
	var useSSHAgent bool
	if endpointInfo.UseSSHAgent != nil {
		useSSHAgent = *endpointInfo.UseSSHAgent
	} else {
		useSSHAgent = SSHClientDefault.UseSSHAgent
	}

	printMessage(VerbosityData, "      Using SSH Agent?: %t\n", useSSHAgent)

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

	printMessage(VerbosityFullData, "      Sudo Password: %s\n", info.SudoPassword)

	// Get remote transfer buffer file path from endpoint or if missing use default
	info.RemoteTransferBuffer = endpointInfo.RemoteTransferBuffer
	if info.RemoteTransferBuffer == "" {
		info.RemoteTransferBuffer = SSHClientDefault.RemoteTransferBuffer
	}

	printMessage(VerbosityFullData, "      Remote Transfer Buffer: %s\n", info.RemoteTransferBuffer)

	// Get remote backup buffer file path from endpoint or if missing use default
	info.RemoteBackupDir = endpointInfo.RemoteBackupDir
	if info.RemoteBackupDir == "" {
		info.RemoteBackupDir = SSHClientDefault.RemoteBackupDir
	}
	// Ensure trailing slashes don't make their way into the path
	info.RemoteBackupDir = strings.TrimSuffix(info.RemoteBackupDir, "/")

	printMessage(VerbosityFullData, "      Remote Backup Directory: %s\n", info.RemoteBackupDir)

	return
}

// Retrieves file paths in maps per host and universal conf dir
func mapAllRepoFiles(tree *object.Tree) (allHostsFiles map[string]map[string]struct{}, universalFiles map[string]struct{}, universalGroupFiles map[string]map[string]struct{}, err error) {
	// Retrieve files from commit tree
	repoFiles := tree.Files()

	// Initialize maps
	allHostsFiles = make(map[string]map[string]struct{})
	universalFiles = make(map[string]struct{})
	universalGroupFiles = make(map[string]map[string]struct{})

	// Retrieve all non-changed repository files for this host (and universal dir) for later deduping
	for {
		// Go to next file in list
		var repoFile *object.File
		repoFile, err = repoFiles.Next()
		if err != nil {
			// Break at end of list
			if err == io.EOF {
				err = nil
				break
			}

			// Fail if next file doesnt work
			err = fmt.Errorf("failed retrieving commit file: %v", err)
			return
		}

		// Split host dir and target path
		commitSplit := strings.SplitN(repoFile.Name, OSPathSeparator, 2)

		// Skip repo files in root of repository
		if len(commitSplit) <= 1 {
			continue
		}

		// Get host dir part and target file path part
		commitHost := commitSplit[0]
		commitPath := commitSplit[1]

		// Add tgt file path in main Universal directory to map for later deduping
		if commitHost == UniversalDirectory {
			universalFiles[commitPath] = struct{}{}
		}

		// Add files by universal group dirs to map for later deduping
		for universalGroup, _ := range UniversalGroups {
			if commitHost == universalGroup {
				// Repo file is under one of the universal group directories
				universalGroupFiles[universalGroup] = make(map[string]struct{})
				universalGroupFiles[universalGroup][commitPath] = struct{}{}
			}
		}

		// Add files by their host to the map
		if _, hostExists := allHostsFiles[commitHost]; !hostExists {
			allHostsFiles[commitHost] = make(map[string]struct{})
		}
		allHostsFiles[commitHost][commitPath] = struct{}{}
	}

	return
}

// Searches through all repository files and ensures that hosts that have an identical file to a universal file only deploy the host file
func findDeniedUniversalFiles(endpointName string, hostFiles map[string]struct{}, universalFiles map[string]struct{}, universalGroupFiles map[string]map[string]struct{}) (deniedUniversalFiles map[string]struct{}) {
	deniedUniversalFiles = make(map[string]struct{})

	// Record denied files for global universal files
	for universalFile, _ := range universalFiles {
		_, hostHasUniversalOverride := hostFiles[universalFile]
		if hostHasUniversalOverride {
			// host has a file path that is also present in the universal dir
			// should not deploy universal files if host has an identical file path
			deniedFilePath := filepath.Join(UniversalDirectory, universalFile)
			deniedUniversalFiles[deniedFilePath] = struct{}{}
		}
	}

	// Get universal groups this host is a part of
	hostUniversalGroups := make(map[string]struct{}) // Store group names for this host
	for universalGroup, hosts := range UniversalGroups {
		for _, host := range hosts {
			if endpointName == host {
				hostUniversalGroups[universalGroup] = struct{}{}
			}
		}
	}

	// Find overlaps between group files and host files - record overlapping group files in denied map
	for groupName, groupFiles := range universalGroupFiles {
		// Skip groups not applicable to this host
		_, hostIsInGroup := hostUniversalGroups[groupName]
		if !hostIsInGroup {
			continue
		}

		// Find overlap files
		for groupFile, _ := range groupFiles {
			_, hostHasUniversalOverride := hostFiles[groupFile]
			if hostHasUniversalOverride {
				// host has a file path that is also present in the group universal dir
				// should not deploy group universal files if host has an identical file path
				deniedFilePath := filepath.Join(groupName, groupFile)
				deniedUniversalFiles[deniedFilePath] = struct{}{}
			}
		}
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
func validateCommittedFiles(commitHosts map[string]struct{}, DeployerEndpoints map[string]DeployerEndpoints, rawFile diff.File) (path string, FileType string, SkipFile bool, err error) {
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

	printMessage(VerbosityData, "  Validating committed file %s\n", path)

	// File exists, but no path - technically valid
	if path == "" {
		return
	}

	// Ensure file is valid against config
	hostDirName, SkipFile := validateRepoFile(path, DeployerEndpoints)
	if SkipFile {
		// Not valid, skip
		return
	}

	// Add host to map
	commitHosts[hostDirName] = struct{}{}

	printMessage(VerbosityData, "  Validated committed file %s\n", path)

	return
}

// Checks to ensure a given repository relative file path is:
//  1. A top-level directory name that is a valid host name as in deployerEndpoints
//  2. A top-level directory name that is the universal config directory
//  3. A top-level directory name that is the a valid universal config group as in UniversalGroups
//  4. A file inside any directory (i.e. not a file just in root of repo)
//  5. A file not inside any of the IgnoreDirectories
func validateRepoFile(path string, deployerEndpoints map[string]DeployerEndpoints) (topLevelDir string, SkipFile bool) {
	// Always ignore files in root of repository
	if !strings.ContainsRune(path, []rune(OSPathSeparator)[0]) {
		SkipFile = true
		printMessage(VerbosityData, "    File is in root of repo, skipping\n")
		return
	}

	// Get top-level directory name
	fileDirNames := strings.SplitN(path, OSPathSeparator, 2)
	topLevelDir = fileDirNames[0]

	// SkipFile if inside ignore directories array
	if len(IgnoreDirectories) > 0 {
		// When committed file directory is prefixed by an ignore directory, skip file
		for _, ignoreDir := range IgnoreDirectories {
			if topLevelDir == ignoreDir {
				SkipFile = true
				printMessage(VerbosityData, "    File is in an ignore directory, skipping\n")
				return
			}
		}
	}

	// Ensure directory name is valid against config options
	for configHost, _ := range deployerEndpoints {
		// file top-level dir is a valid host or the universal directory
		if topLevelDir == configHost || topLevelDir == UniversalDirectory {
			SkipFile = false
			return
		}
		SkipFile = true
	}
	for universalGroup, _ := range UniversalGroups {
		// file top-level dir is a universal group
		if topLevelDir == universalGroup {
			SkipFile = false
			return
		}
		SkipFile = true
	}

	printMessage(VerbosityData, "    File is not in deployerEndpoints or a Universal, skipping\n")
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

// Splits host directory name from the expected target file path
// Requires localRepoPath be a relative path without leading slashes
// Returned targetFilePath will contain a leading slash
// Path separators are linux ("/")
// Function does not return errors, but unexpected input will return nil outputs
func separateHostDirFromPath(localRepoPath string) (hostDir string, targetFilePath string) {
	// Bad - not a path, just a name
	if !strings.Contains(localRepoPath, "/") {
		return
	}

	// Separate on first occurence of path separator
	pathSplit := strings.SplitN(localRepoPath, "/", 2)

	// Bad - only accept length of 2
	if len(pathSplit) != 2 {
		return
	}

	// Retrieve the first array item as the host directory name
	hostDir = pathSplit[0]

	// Retrieve the second array item as the expected target path
	targetFilePath = pathSplit[1]

	// Add leading slash to path
	targetFilePath = "/" + targetFilePath
	return
}

// Takes a string input, and returns a SHA256 hexadecimal hash string
func SHA256Sum(input string) (hash string) {
	// Convert input string to byte array
	inputBytes := []byte(input)

	// Create new hashing function
	hasher := sha256.New()

	// Write input bytes into hasher
	hasher.Write(inputBytes)

	// Retrieve the raw hash
	rawHash := hasher.Sum(nil)

	// Format raw hash into hex
	hash = hex.EncodeToString(rawHash)

	return
}
