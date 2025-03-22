// controller
package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"math"
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
	if !strings.HasPrefix(input, fileURIPrefix) {
		csv = input
		return
	}

	printMessage(verbosityData, "Received File URI '%s'\n", input)

	// Not adhering to actual URI standards -- I just want file paths
	path := strings.TrimPrefix(input, fileURIPrefix)

	printMessage(verbosityFullData, "Preprocessed File URI Path '%s'\n", path)

	// Check for ~/ and expand if required
	path = expandHomeDirectory(path)

	printMessage(verbosityData, "File URI contains path '%s'\n", path)

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
		printMessage(verbosityFullData, "Extracted Override List from File: %v\n", csv)
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
	printMessage(verbosityFullData, "Extracted Override List from File: %v\n", csv)
	return
}

// Checks for user-chosen host/file override with given host/file
// Returns immediately if override is empty
func checkForOverride(override string, current string) (skip bool) {
	// If input is a host and state is offline and user did not request deployment state override, then skip
	_, inputCheckIsAHost := config.hostInfo[current]
	if inputCheckIsAHost && config.hostInfo[current].deploymentState == "offline" && !config.ignoreDeploymentState {
		skip = true
		return
	}

	// Return early if no override
	if override == "" {
		return
	}

	// Allow current item if item is part of a group
	// Only applies to host overrides, but shouldn't affect file overrides
	_, currentItemIsPartofGroup := config.hostInfo[current].universalGroups[override]
	if currentItemIsPartofGroup {
		skip = false
		return
	}

	// Split choices on comma
	userHostChoices := strings.SplitSeq(override, ",")

	// Check each override specified against current
	for userChoice := range userHostChoices {
		// Only assume override choice is regex if user requested it
		if config.regexEnabled {
			// Prepare user choice as regex
			userRegex, err := regexp.Compile(userChoice)
			if err != nil {
				// Invalid regex, always skip (but print high verbosity what happened)
				printMessage(verbosityData, "WARNING: Invalid regular expression: %v", err)
				return
			}

			// Check if user regex matches current item, if so return
			if userRegex.MatchString(current) {
				skip = false
				return
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
	commitSplit := strings.SplitN(repoFilePath, config.osPathSeparator, 2)

	// Skip repo files in root of repository
	if len(commitSplit) <= 1 {
		return
	}

	// Get host dir part and target file path part
	topLevelDirName := commitSplit[0]
	tgtFilePath := commitSplit[1]

	// Add files by universal group dirs to map for later deduping
	_, fileIsInUniversalGroup := config.allUniversalGroups[topLevelDirName]
	if fileIsInUniversalGroup || topLevelDirName == config.universalDirectory {
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
	for endpointName := range config.hostInfo {
		// Initialize innner map
		deniedUniversalFiles[endpointName] = make(map[string]struct{})

		// Find overlaps between group files and host files - record overlapping group files in denied map
		for groupName, groupFiles := range universalFiles {
			// Skip groups not applicable to this host
			_, hostIsInFilesUniversalGroup := config.hostInfo[endpointName].universalGroups[groupName]
			if !hostIsInFilesUniversalGroup && groupName != config.universalDirectory {
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
func extractMetadata(fileContents string) (metadataSection string, contentSection []byte, err error) {
	// Add newline so file content doesnt have empty line at the top
	endDelimiter := metaDelimiter + "\n"

	// Find the start and end of the metadata section
	startIndex := strings.Index(fileContents, metaDelimiter)
	if startIndex == -1 {
		err = fmt.Errorf("json start delimiter missing")
		return
	}
	startIndex += len(metaDelimiter)

	endIndex := strings.Index(fileContents[startIndex:], endDelimiter)
	if endIndex == -1 {
		testEndIndex := strings.Index(fileContents[startIndex:], metaDelimiter)
		if testEndIndex == -1 {
			err = fmt.Errorf("json end delimiter missing")
			return
		}
		err = fmt.Errorf("json end delimiter missing")
		return
	}
	endIndex += startIndex

	// Extract the metadata section and remaining content into their own vars
	metadataSection = fileContents[startIndex:endIndex]
	remainingContent := fileContents[:startIndex-len(metaDelimiter)] + fileContents[endIndex+len(endDelimiter):]
	contentSection = []byte(remainingContent)

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
func validateCommittedFiles(rawFile diff.File, fileOverride string) (path string, fileType string, skipFile bool, err error) {
	// Nothing to validate
	if rawFile == nil {
		return
	}

	// Retrieve integer representation of the files mode
	mode := fmt.Sprintf("%v", rawFile.Mode())

	// Retrieve the type for this file
	fileType = determineFileType(mode)

	// Skip processing if file is unsupported
	if fileType == "unsupported" {
		skipFile = true
		return
	}

	// Get the path
	path = rawFile.Path()

	printMessage(verbosityData, "  Validating committed file %s\n", path)

	// Skip file if not user requested file (if requested)
	skipFile = checkForOverride(fileOverride, path)
	if skipFile {
		printMessage(verbosityFullData, "  File not desired\n")
		skipFile = true
		return
	}

	// File exists, but no path - technically valid
	if path == "" {
		return
	}

	// Ensure file is valid against config
	if repoFileIsValid(path) {
		// Not valid, skip
		skipFile = true
		return
	}

	printMessage(verbosityData, "  Validated committed file %s\n", path)

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
	if !strings.ContainsRune(path, []rune(config.osPathSeparator)[0]) {
		fileIsValid = true
		printMessage(verbosityData, "    File is in root of repo, skipping\n")
		return
	}

	// Get top-level directory name
	fileDirNames := strings.SplitN(path, config.osPathSeparator, 2)
	topLevelDir := fileDirNames[0]

	// fileIsValid if inside ignore directories array
	if len(config.ignoreDirectories) > 0 {
		// When committed file directory is prefixed by an ignore directory, skip file
		for _, ignoreDir := range config.ignoreDirectories {
			if topLevelDir == ignoreDir {
				fileIsValid = true
				printMessage(verbosityData, "    File is in an ignore directory, skipping\n")
				return
			}
		}
	}

	// Ensure directory name is valid against config options
	for configHost := range config.hostInfo {
		// file top-level dir is a valid host or the universal directory
		if topLevelDir == configHost || topLevelDir == config.universalDirectory {
			printMessage(verbosityData, "    File is valid (Dir matches Hostname or is Universal Dir)\n")
			fileIsValid = false
			return
		}
		fileIsValid = true
	}
	_, fileIsInUniversalGroup := config.allUniversalGroups[topLevelDir]
	if fileIsInUniversalGroup {
		printMessage(verbosityData, "    File is valid (Dir matches a Universal Group Dir)\n")
		fileIsValid = false
		return
	}

	printMessage(verbosityData, "    File is not under a valid host directory or a universal directory, skipping\n")
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
	targetPathArray := strings.SplitN(linkTarget, config.osPathSeparator, 2)
	linkPathArray := strings.SplitN(filePath, config.osPathSeparator, 2)

	// Error if link top level directories are not the same (link is between host directories)
	if targetPathArray[0] != linkPathArray[0] {
		err = fmt.Errorf("cannot have symbolic link between host directories")
		return
	}

	// Return target path without top level directory name (host dir name) (this is remote host format now)
	convertedPath := strings.ReplaceAll(targetPathArray[1], config.osPathSeparator, "/")
	targetPath = "/" + convertedPath
	return
}

// Splits host directory name from the expected target file path
// Requires localRepoPath be a relative path without leading slashes
// Returned targetFilePath will contain a leading slash
// Path separators are linux ("/")
// Function does not return errors, but unexpected input will return nil outputs
func translateLocalPathtoRemotePath(localRepoPath string) (hostDir string, targetFilePath string) {
	// Remove .remote-artifact extension if applicable
	localRepoPath = strings.TrimSuffix(localRepoPath, artifactPointerFileExtension)

	// Format repoFilePath with the expected host path separators
	localRepoPath = strings.ReplaceAll(localRepoPath, config.osPathSeparator, "/")

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
func permissionsSymbolicToNumeric(permissions string) (perm int, err error) {
	// Validate length
	if len(permissions) < 6 || len(permissions) > 9 {
		err = fmt.Errorf("invalid permissions string: lenght must be between 6 and 9 characters (length is %d)", len(permissions))
		return
	}

	// Loop permission fields
	var bits string
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
	perm, err = strconv.Atoi(bits)
	return
}

// Parses custom format used with stat command
// Relies on the stat formatting found in global constatnt statCmd
func extractMetadataFromStat(statOutput string) (fileInfo RemoteFileInfo, err error) {
	// Index Names:
	// - 0 = name
	// - 1 = type - see global const
	// - 2 = User
	// - 3 = Group
	// - 4 = PermissionBits
	// - 5 = Size in bytes
	// - 6 = Derefenced name if applicable, otherwise just file name in single quotes
	//[/etc/rmt],[symbolic link],[root],[root],[777],[13],['/etc/rmt' -> '/usr/sbin/rmt']
	const linkDelimiter string = "' -> '"

	// Trim stray newlines from input if they exist
	statOutput = strings.TrimSuffix(statOutput, "\n")

	// Separate CSV into fields
	statFields := strings.Split(statOutput, ",")
	if len(statFields) != 7 {
		// Refuse any stat that does not have the exact expected number of fields
		err = fmt.Errorf("invalid file metadata: expected 7 fields, received %d fields", len(statFields))
		return
	}

	// Extract data from each field, validating field is within bounds
	for fieldIndex, field := range statFields {
		// Ensure Prefix is present
		if !strings.HasPrefix(field, "[") {
			err = fmt.Errorf("incorrect field prefix: missing prefix character '[' in value '%s'", field)
			return
		}

		// Ensure Suffix is present
		if !strings.HasSuffix(field, "]") {
			err = fmt.Errorf("incorrect field suffix: missing suffix character ']' in value '%s'", field)
			return
		}

		// Trim prefix and suffix from field text
		statFields[fieldIndex] = strings.TrimPrefix(statFields[fieldIndex], "[")
		statFields[fieldIndex] = strings.TrimSuffix(statFields[fieldIndex], "]")
	}

	// Handle symlink field parsing if present
	if strings.Contains(statFields[6], linkDelimiter) {
		// Split on the link point string
		dereferencedFields := strings.Split(statFields[6], linkDelimiter)

		// Ensure string was properly separated
		if len(dereferencedFields) != 2 {
			err = fmt.Errorf("could not identify dereferenced link target name from value '%s'", statFields[6])
			return
		}

		// Trim single quotes from stat output
		dereferencedFields[1] = strings.TrimPrefix(dereferencedFields[1], "'")
		dereferencedFields[1] = strings.TrimSuffix(dereferencedFields[1], "'")

		// Save back into array
		statFields[6] = dereferencedFields[1]
	} else if !strings.Contains(statFields[6], linkDelimiter) {
		// Zero out the string
		statFields[6] = ""
	}

	// Reject file names with newlines
	if strings.Contains(statFields[0], "\n") || strings.Contains(statFields[6], "\n") {
		err = fmt.Errorf("file names with newlines are unsupported")
		return
	}

	// Put all parsed data into structured return
	fileInfo.name = statFields[0]
	fileInfo.fsType = statFields[1]
	fileInfo.owner = statFields[2]
	fileInfo.group = statFields[3]
	fileInfo.linkTarget = statFields[6]

	// Assert permission string as integer
	permissionBits, err := strconv.Atoi(statFields[4])
	if err != nil {
		err = fmt.Errorf("permission bits not a number: %v", err)
		return
	}
	fileInfo.permissions = permissionBits

	// Assert file size string as integer
	fileSizeBytes, err := strconv.Atoi(statFields[5])
	if err != nil {
		err = fmt.Errorf("file size not a number: %v", err)
		return
	}
	fileInfo.size = fileSizeBytes

	// Valid input to this function implies it exists
	fileInfo.exists = true
	return
}

// Compares compiled metadata from local and remote file and compares them and reports what is different
// Only compares hashes, owner+group, and permission bits
func checkForDiff(remoteMetadata RemoteFileInfo, localMetadata FileInfo) (contentDiffers bool, metadataDiffers bool) {
	// If user requested force, return early, as deployment will be atomic
	if config.forceEnabled {
		contentDiffers = true
		metadataDiffers = true
		return
	}

	// Check if remote content differs from local
	if remoteMetadata.hash != localMetadata.hash {
		contentDiffers = true
	} else if remoteMetadata.hash == localMetadata.hash {
		contentDiffers = false
	}

	// Check if remote permissions differs from expected
	var permissionsDiffer bool
	if remoteMetadata.permissions != localMetadata.permissions {
		permissionsDiffer = true
	} else if remoteMetadata.permissions == localMetadata.permissions {
		permissionsDiffer = false
	}

	// Prevent comparing the literal character ':' against local metadata
	var remoteOwnerGroup string
	if remoteMetadata.owner != "" && remoteMetadata.group != "" {
		remoteOwnerGroup = remoteMetadata.owner + ":" + remoteMetadata.group
	}

	// Check if remote ownership match expected
	var ownershipDiffers bool
	if remoteOwnerGroup != localMetadata.ownerGroup {
		ownershipDiffers = true
	} else if remoteOwnerGroup == localMetadata.ownerGroup {
		ownershipDiffers = false
	}

	// If either piece of metadata differs, whole metdata is different
	if ownershipDiffers || permissionsDiffer {
		metadataDiffers = true
	} else if !ownershipDiffers && !permissionsDiffer {
		metadataDiffers = false
	}

	return
}

// Groups deployment files by identical reload commands
// Returns a map that keys on a reload ID and the array of commands
// Returns a regular list of files that do not need reloads
func groupFilesByReloads(allFileInfo map[string]FileInfo, repoFilePaths []string) (commitFileByCommand map[string][]string, commitFilesNoReload []string) {
	commitFileByCommand = make(map[string][]string)
	for _, repoFilePath := range repoFilePaths {
		// New files with reload commands
		if allFileInfo[repoFilePath].reloadRequired {
			// Create an ID based on the command array to uniquely identify the group that files will belong to
			reloadCommands := fmt.Sprintf("%v", allFileInfo[repoFilePath].reload)
			cmdArrayID := base64.StdEncoding.EncodeToString([]byte(reloadCommands))

			// Add file to array based on its unique set of reload commands
			commitFileByCommand[cmdArrayID] = append(commitFileByCommand[cmdArrayID], repoFilePath)
		} else {
			// All other files - no reloads - in its own array
			commitFilesNoReload = append(commitFilesNoReload, repoFilePath)
		}
	}
	return
}

// Format elapsed millisecond time to its max unit size plus one smaller unit
func formatElapsedTime(elapsed int64) (elapsedWithUnits string) {
	// Handle days
	days := elapsed / (1000 * 60 * 60 * 24)
	elapsed %= (1000 * 60 * 60 * 24)

	// Handle hours
	hours := elapsed / (1000 * 60 * 60)
	elapsed %= (1000 * 60 * 60)

	// Handle minutes
	minutes := elapsed / (1000 * 60)
	elapsed %= (1000 * 60)

	// Handle seconds
	seconds := elapsed / 1000
	milliseconds := elapsed % 1000

	// Format based on the largest unit available
	if days > 0 {
		elapsedWithUnits = fmt.Sprintf("%d days and %d hours", days, hours)
	} else if hours > 0 {
		elapsedWithUnits = fmt.Sprintf("%dh and %dm", hours, minutes)
	} else if minutes > 0 {
		elapsedWithUnits = fmt.Sprintf("%dm and %ds", minutes, seconds)
	} else if seconds > 0 {
		elapsedWithUnits = fmt.Sprintf("%ds %dms", seconds, milliseconds)
	} else {
		elapsedWithUnits = fmt.Sprintf("%dms", milliseconds)
	}

	return
}

// FormatBytes takes a raw byte integer and converts it to a human-readable format with appropriate units
func formatBytes(bytes int) (bytesWithUnits string) {
	units := []string{"Bytes", "KiB", "MiB", "GiB", "TiB", "PiB"}
	if bytes == 0 {
		return fmt.Sprintf("0 %s", units[0])
	}

	// Determine the appropriate unit
	unitIndex := int(math.Floor(math.Log(float64(bytes)) / math.Log(1024)))
	if unitIndex >= len(units) {
		unitIndex = len(units) - 1
	}

	// Calculate the value in the appropriate unit
	value := float64(bytes) / math.Pow(1024, float64(unitIndex))

	// Return the formatted string
	bytesWithUnits = fmt.Sprintf("%.2f %s", value, units[unitIndex])
	return
}
