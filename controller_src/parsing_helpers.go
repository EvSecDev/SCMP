// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ###################################
//      PARSING FUNCTIONS
// ###################################

// Checks for user host override with given host
func checkForHostOverride(hostOverride string, currentHost string) (SkipHost bool) {
	// Allow user override hosts
	if hostOverride != "" {
		userHostChoices := strings.Split(hostOverride, ",")
		for _, userHostChoice := range userHostChoices {
			if userHostChoice == currentHost {
				break
			}
			SkipHost = true
		}
	}
	return
}

// Map value deleter
func removeValueFromMapSlice(HostsAndFilesMap map[string][]string, key, valueToRemove string) {
	if values, ok := HostsAndFilesMap[key]; ok {
		newValues := []string{}
		for _, value := range values {
			if value != valueToRemove {
				newValues = append(newValues, value)
			}
		}
		HostsAndFilesMap[key] = newValues
	}
}

// Ensures template configs dont get deployed where they shouldn't
func deDupsHostsandTemplateCommits(HostsAndFiles map[string]string, TemplateDirectory string, AllHostsAndFilesMap map[string][]string, endpointName string, OSPathSeparator string, ignoreTemplates bool) (FilteredCommitFilePaths []string) {
	// Filter down committed files to only ones that are allowed for this host and create map for deduping
	HostsAndFilesMap := make(map[string][]string)
	for filePath := range HostsAndFiles {
		// Skip files in root of repository - only files inside host directories should be considered
		if !strings.ContainsRune(filePath, []rune(OSPathSeparator)[0]) {
			continue
		}

		// Get the host name from the repository top level directory
		commitSplit := strings.SplitN(filePath, OSPathSeparator, 2)
		commitHost := commitSplit[0]
		commitPath := commitSplit[1]

		// Skip files that arent in this hosts directory or in the template directory
		if commitHost != endpointName && commitHost != TemplateDirectory {
			continue
		}

		// Skip template files for hosts that dont want templates
		if commitHost == TemplateDirectory && ignoreTemplates {
			continue
		}

		// Append path to the map
		HostsAndFilesMap[commitHost] = append(HostsAndFilesMap[commitHost], commitPath)
	}

	// Map to track duplicates
	confFileCount := make(map[string]int)

	// Count occurences of each conf file in entire repo
	for _, conffiles := range AllHostsAndFilesMap {
		for _, conf := range conffiles {
			confFileCount[conf]++
		}
	}

	// Remove duplicate confs for host in template dir
	for hostdir, conffiles := range AllHostsAndFilesMap {
		for _, conf := range conffiles {
			// Only remove if multiple same config paths AND the hostdir part is the template dir
			if confFileCount[conf] > 1 && hostdir == TemplateDirectory {
				// Maps always passed by reference; function will edit original map
				removeValueFromMapSlice(AllHostsAndFilesMap, hostdir, conf)
			}
		}
	}

	// Compare the confs allowed to deploy in the repo with the confs in the actual commit
	hostFiles, hostExists := AllHostsAndFilesMap[endpointName]
	goldenFiles, templateExists := HostsAndFilesMap[TemplateDirectory]
	if hostExists && templateExists {
		// Create a map to track files in the host
		hostFileMap := make(map[string]struct{})
		for _, file := range hostFiles {
			hostFileMap[file] = struct{}{}
		}

		// Filter out files in the golden template that also exist in the host
		var newTemplateFiles []string
		for _, file := range goldenFiles {
			if _, exists := hostFileMap[file]; !exists {
				newTemplateFiles = append(newTemplateFiles, file)
			}
		}

		// Update the HostsAndFilesMap map with the filtered files
		HostsAndFilesMap[TemplateDirectory] = newTemplateFiles
	}

	// Convert map into desired formats for further processing
	for host, paths := range HostsAndFilesMap {
		for _, path := range paths {
			// Paths in correct format for loading from git
			FilteredCommitFilePaths = append(FilteredCommitFilePaths, host+OSPathSeparator+path)
		}
	}

	return
}

// Function to extract and validate metadata JSON from file contents
func extractMetadata(fileContents string) (metadataSection string, remainingContent string, err error) {
	// Define the delimiters
	StartDelimiter := "#|^^^|#"
	EndDelimiter := "#|^^^|#\n" // trims newline from actual file contents
	Delimiter := "#|^^^|#"

	// Find the start and end of the metadata section
	startIndex := strings.Index(fileContents, StartDelimiter)
	if startIndex == -1 {
		err = fmt.Errorf("json start delimter missing")
		return
	}
	startIndex += len(StartDelimiter)

	endIndex := strings.Index(fileContents[startIndex:], EndDelimiter)
	if endIndex == -1 {
		TestEndIndex := strings.Index(fileContents[startIndex:], Delimiter)
		if TestEndIndex == -1 {
			err = fmt.Errorf("no newline after json end delimiter")
			return
		}
		err = fmt.Errorf("json end delimter missing ")
		return
	}
	endIndex += startIndex

	// Extract the metadata section and remaining content into their own vars
	metadataSection = fileContents[startIndex:endIndex]
	remainingContent = fileContents[:startIndex-len(StartDelimiter)] + fileContents[endIndex+len(EndDelimiter):]

	return
}

// Determines which file types in the commit are allowed to be deployed
func determineFileType(filePath string) (fileType string, err error) {
	// Get the file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return
	}
	fileMode := fileInfo.Mode()

	// Set type of file in commit - skip unsupported
	if fileMode.IsRegular() {
		// Text file
		fileType = "regular"
	} else if fileMode&os.ModeSymlink != 0 {
		// Special, but able to be handled
		fileType = "symlink"
	} else if fileMode.IsDir() {
		// like drw-rw----
		fileType = "unsupported"
	} else if fileMode&os.ModeDevice != 0 {
		// like brw-rw----
		fileType = "unsupported"
	} else if fileMode&os.ModeCharDevice != 0 {
		// like crw-rw----
		fileType = "unsupported"
	} else if fileMode&os.ModeNamedPipe != 0 {
		// like prw-rw----
		fileType = "unsupported"
	} else if fileMode&os.ModeSocket != 0 {
		// like Srw-rw----
		fileType = "unsupported"
	} else {
		// Unknown - dont process
		fileType = "unsupported"
	}

	return
}

// Retrieve the symbolic link target path and check for validity
func ResolveLinkToTarget(filePath string, OSPathSeparator string) (targetPath string, err error) {
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
