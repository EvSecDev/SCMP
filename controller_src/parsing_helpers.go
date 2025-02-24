// controller
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ###################################
//      PARSING FUNCTIONS
// ###################################

// Used when an argument has a file:// URI scheme
// Loads file in and separates based on newlines or commas and returns a string csv
func retrieveURIFile(input string) (csv string, err error) {
	// Return early if not a file URI scheme
	if !strings.HasPrefix(input, "file:") {
		csv = input
		return
	}

	printMessage(VerbosityData, "Received File URI '%s'\n", input)

	// Not adhering to actual URI standards -- I just want file paths
	path := strings.TrimPrefix(input, "file://")

	printMessage(VerbosityFullData, "Preprocessed File URI Path '%s'\n", path)

	// Check for ~/ and expand if required
	path = expandHomeDirectory(path)

	printMessage(VerbosityData, "File URI contains path '%s'\n", path)

	// Retrieve the file contents
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Convert file to string
	file := string(fileBytes)

	// Trim newlines/spaces from beginning/end
	file = strings.TrimSpace(file)

	// Split file contens by newlins
	lines := strings.Split(file, "\n")

	// If file is multi-line, convert into CSV
	if len(lines) > 1 {
		csv = strings.Join(lines, ",")
		printMessage(VerbosityFullData, "Extracted Override List from File: %v\n", csv)
		return
	} else if len(lines) == 0 {
		err = fmt.Errorf("file is empty")
		return
	}

	// Compile the regular expression to match space or comma
	separatorRegex := regexp.MustCompile(`[ ,]+`)

	// Use the regular expression to split the string one first line
	lineArray := separatorRegex.Split(lines[0], -1)
	csv = strings.Join(lineArray, ",")
	printMessage(VerbosityFullData, "Extracted Override List from File: %v\n", csv)
	return
}

// Checks for user-chosen host/file override with given host/file
// Returns immediately if override is empty
func checkForOverride(override string, current string) (skip bool) {
	// If input is a host and state is offline and user did not request deployment state override, then skip
	_, inputCheckIsAHost := config.HostInfo[current]
	if inputCheckIsAHost && config.HostInfo[current].DeploymentState == "offline" && !config.IgnoreDeploymentState {
		skip = true
		return
	}

	// Return early if no override
	if override == "" {
		return
	}

	// Allow current item if item is part of a group
	// Only applies to host overrides, but shouldn't affect file overrides
	_, currentItemIsPartofGroup := config.HostInfo[current].UniversalGroups[override]
	if currentItemIsPartofGroup {
		skip = false
		return
	}

	// Split choices on comma
	userHostChoices := strings.Split(override, ",")

	// Check each override specified against current
	for _, userChoice := range userHostChoices {
		// Users choice ends with wildcard, search based on prefix only
		if strings.HasSuffix(userChoice, "*") {
			userChoicePrefix := strings.Replace(userChoice, "*", "", -1)

			// Don't skip current if user choice prefix matches
			if strings.HasPrefix(current, userChoicePrefix) {
				skip = false
				continue
			}
		}

		// Don't skip if current is user choice
		if userChoice == current {
			skip = false
			return
		}
		skip = true
	}

	return
}

// Retrieves file paths in maps per host and universal conf dir
func parseAllRepoFiles(tree *object.Tree) (allHostsFiles map[string]map[string]struct{}, allUniversalFiles map[string]map[string]struct{}, err error) {
	// Retrieve files from commit tree
	repoFiles := tree.Files()

	// Initialize maps
	allHostsFiles = make(map[string]map[string]struct{})
	allUniversalFiles = make(map[string]map[string]struct{})

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

		// Parse out by host/universal
		mapFilesByHostOrUniversal(repoFile.Name, allHostsFiles, allUniversalFiles)
	}
	return
}

// Modifies input maps to divide up repository files between host directories and universal directories
func mapFilesByHostOrUniversal(repoFilePath string, allHostsFiles map[string]map[string]struct{}, allUniversalFiles map[string]map[string]struct{}) {
	// Split host dir and target path
	commitSplit := strings.SplitN(repoFilePath, config.OSPathSeparator, 2)

	// Skip repo files in root of repository
	if len(commitSplit) <= 1 {
		return
	}

	// Get host dir part and target file path part
	topLevelDirName := commitSplit[0]
	tgtFilePath := commitSplit[1]

	// Add files by universal group dirs to map for later deduping
	_, fileIsInUniversalGroup := config.AllUniversalGroups[topLevelDirName]
	if fileIsInUniversalGroup || topLevelDirName == config.UniversalDirectory {
		// Make map if inner map isn't initialized already
		_, dirAlreadyExistsInMap := allUniversalFiles[topLevelDirName]
		if !dirAlreadyExistsInMap {
			allUniversalFiles[topLevelDirName] = make(map[string]struct{})
		}

		// Repo file is under one of the universal group directories
		allUniversalFiles[topLevelDirName][tgtFilePath] = struct{}{}
		return
	}

	// Add files by their host to the map - make map if host map isn't initialized yet
	_, hostAlreadyExistsInMap := allHostsFiles[topLevelDirName]
	if !hostAlreadyExistsInMap {
		allHostsFiles[topLevelDirName] = make(map[string]struct{})
	}
	allHostsFiles[topLevelDirName][tgtFilePath] = struct{}{}
}

// Record universal files that are NOT to be used for each host (host has an override file)
func mapDeniedUniversalFiles(allHostsFiles map[string]map[string]struct{}, universalFiles map[string]map[string]struct{}) (deniedUniversalFiles map[string]map[string]struct{}) {
	// Initialize map
	deniedUniversalFiles = make(map[string]map[string]struct{})

	// Created denied map for each host in config
	for endpointName := range config.HostInfo {
		// Initialize innner map
		deniedUniversalFiles[endpointName] = make(map[string]struct{})

		// Find overlaps between group files and host files - record overlapping group files in denied map
		for groupName, groupFiles := range universalFiles {
			// Skip groups not applicable to this host
			_, hostIsInFilesUniversalGroup := config.HostInfo[endpointName].UniversalGroups[groupName]
			if !hostIsInFilesUniversalGroup && groupName != config.UniversalDirectory {
				continue
			}

			// Find overlap files
			for groupFile := range groupFiles {
				_, hostHasUniversalOverride := allHostsFiles[endpointName][groupFile]
				if hostHasUniversalOverride {
					// Host has a file path that is also present in the group universal dir
					// Should never deploy group universal files if host has an identical file path
					deniedFilePath := filepath.Join(groupName, groupFile)
					deniedUniversalFiles[endpointName][deniedFilePath] = struct{}{}
				}
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
		err = fmt.Errorf("json start delimiter missing")
		return
	}
	startIndex += len(Delimiter)

	endIndex := strings.Index(fileContents[startIndex:], EndDelimiter)
	if endIndex == -1 {
		TestEndIndex := strings.Index(fileContents[startIndex:], Delimiter)
		if TestEndIndex == -1 {
			err = fmt.Errorf("json end delimiter missing")
			return
		}
		err = fmt.Errorf("json end delimiter missing")
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
func validateCommittedFiles(rawFile diff.File, fileOverride string) (path string, FileType string, SkipFile bool, err error) {
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

	// Skip file if not user requested file (if requested)
	skipFile := checkForOverride(fileOverride, path)
	if skipFile {
		printMessage(VerbosityFullData, "  File not desired\n")
		SkipFile = true
		return
	}

	// File exists, but no path - technically valid
	if path == "" {
		return
	}

	// Ensure file is valid against config
	if repoFileIsValid(path) {
		// Not valid, skip
		SkipFile = true
		return
	}

	printMessage(VerbosityData, "  Validated committed file %s\n", path)

	return
}

// Checks to ensure a given repository relative file path is:
//  1. A top-level directory name that is a valid host name as in DeployerEndpoints
//  2. A top-level directory name that is the universal config directory
//  3. A top-level directory name that is the a valid universal config group as in UniversalGroups
//  4. A file inside any directory (i.e. not a file just in root of repo)
//  5. A file not inside any of the IgnoreDirectories
func repoFileIsValid(path string) (fileIsValid bool) {
	// Always ignore files in root of repository
	if !strings.ContainsRune(path, []rune(config.OSPathSeparator)[0]) {
		fileIsValid = true
		printMessage(VerbosityData, "    File is in root of repo, skipping\n")
		return
	}

	// Get top-level directory name
	fileDirNames := strings.SplitN(path, config.OSPathSeparator, 2)
	topLevelDir := fileDirNames[0]

	// fileIsValid if inside ignore directories array
	if len(config.IgnoreDirectories) > 0 {
		// When committed file directory is prefixed by an ignore directory, skip file
		for _, ignoreDir := range config.IgnoreDirectories {
			if topLevelDir == ignoreDir {
				fileIsValid = true
				printMessage(VerbosityData, "    File is in an ignore directory, skipping\n")
				return
			}
		}
	}

	// Ensure directory name is valid against config options
	for configHost := range config.HostInfo {
		// file top-level dir is a valid host or the universal directory
		if topLevelDir == configHost || topLevelDir == config.UniversalDirectory {
			printMessage(VerbosityData, "    File is valid (Dir matches Hostname or is Universal Dir)\n")
			fileIsValid = false
			return
		}
		fileIsValid = true
	}
	_, fileIsInUniversalGroup := config.AllUniversalGroups[topLevelDir]
	if fileIsInUniversalGroup {
		printMessage(VerbosityData, "    File is valid (Dir matches a Universal Group Dir)\n")
		fileIsValid = false
		return
	}

	printMessage(VerbosityData, "    File is not under a valid host directory or a universal directory, skipping\n")
	fileIsValid = true
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
	targetPathArray := strings.SplitN(linkTarget, config.OSPathSeparator, 2)
	linkPathArray := strings.SplitN(filePath, config.OSPathSeparator, 2)

	// Error if link top level directories are not the same (link is between host directories)
	if targetPathArray[0] != linkPathArray[0] {
		err = fmt.Errorf("cannot have symbolic link between host directories")
		return
	}

	// Return target path without top level directory name (host dir name) (this is remote host format now)
	convertedPath := strings.ReplaceAll(targetPathArray[1], config.OSPathSeparator, "/")
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

// Converts symbolic linux permission to numeric representation
// Like rwxr-x-rx -> 755
func permissionsSymbolicToNumeric(permissions string) (perm int) {
	var bits string
	// Loop permission fields
	for _, field := range []string{permissions[:3], permissions[3:6], permissions[6:]} {
		bit := 0
		// Read
		if strings.Contains(field, "r") {
			bit += 4
		}
		// Write
		if strings.Contains(field, "w") {
			bit += 2
		}
		// Execute
		if strings.Contains(field, "x") {
			bit += 1
		}
		// Convert sum'd bits to string to concat with other loop iterations
		bits = bits + strconv.Itoa(bit)
	}

	// Convert back to integer (ignore error, we control all input values)
	perm, _ = strconv.Atoi(bits)
	return
}

// Extracts all file information from ls -lA
// Permissions(as 755), Ownership, type, size, name
func extractMetadataFromLS(lsOutput string) (Type string, Permissions string, Owner string, Group string, Size int, Name string, err error) {
	// Split ls output into fields for this file
	fileInfo := strings.Fields(lsOutput)

	// Skip misc ls output
	if len(fileInfo) < 9 {
		err = fmt.Errorf("ls output not complete, not parsing")
		return
	}

	// Retrieve
	Type = string(fileInfo[0][0])
	Permissions = string(fileInfo[0][1:])
	Owner = string(fileInfo[2])
	Group = string(fileInfo[3])
	Size, err = strconv.Atoi(fileInfo[4])
	if err != nil {
		err = fmt.Errorf("failed to parse size field")
		Size = 0
	}
	Name = fileInfo[8]
	return
}
