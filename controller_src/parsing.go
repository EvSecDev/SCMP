// controller
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

func getCommitFiles(commit *object.Commit, DeployerEndpoints map[string][]DeployerEndpoints, fileOverride string) (commitFiles map[string]string, commitHostNames []string, err error) {
	// Show progress to user
	fmt.Print("Retrieving files from commit... ")

	// Get the parent commit
	parentCommit, err := commit.Parents().Next()
	if err != nil {
		err = fmt.Errorf("failed retrieving parent commit: %v", err)
		return
	}

	// Get the diff between the commits
	patch, err := parentCommit.Patch(commit)
	if err != nil {
		err = fmt.Errorf("failed retrieving difference between commits: %v", err)
		return
	}

	// Store files, actions, and host names relevant to this commit
	commitFiles = make(map[string]string)

	// File override array
	fileOverrides := strings.Split(fileOverride, ",")

	// Determine what to do with each file in the commit
	for _, file := range patch.FilePatches() {
		// Get the old file and new file info
		from, to := file.Files()

		// Declare vars (to use named return err)
		var fromPath, toPath string
		var SkipFromFile, SkipToFile bool
		var commitFileToType string

		// Validate the from File object
		fromPath, _, SkipFromFile, err = validateCommittedFiles(&commitHostNames, DeployerEndpoints, from)
		if err != nil {
			return
		}

		// Validate the to File object
		toPath, commitFileToType, SkipToFile, err = validateCommittedFiles(&commitHostNames, DeployerEndpoints, to)
		if err != nil {
			return
		}

		// Skip if either from or to file is not valid
		if SkipFromFile || SkipToFile {
			continue
		}

		// Skip file if not user requested file (if requested)
		if len(fileOverrides) > 0 {
			var fileNotRequested bool
			for _, overrideFile := range fileOverrides {
				if fromPath == overrideFile || toPath == overrideFile {
					continue
				}
				fileNotRequested = true
			}
			if fileNotRequested {
				continue
			}
		}

		// Add file to map depending on how it changed in this commit
		if from == nil {
			// Newly created files
			//   like `touch etc/file.txt`
			commitFiles[toPath] = "create"
		} else if to == nil {
			// Deleted Files
			//   like `rm etc/file.txt`
			commitFiles[fromPath] = "delete"
		} else if fromPath != toPath {
			// Copied or renamed files
			//   like `cp etc/file.txt etc/file2.txt` or `mv etc/file.txt etc/file2.txt`
			_, err := os.Stat(fromPath)
			if os.IsNotExist(err) {
				// Mark for deletion if no longer present in repo
				commitFiles[fromPath] = "delete"
			}
			commitFiles[toPath] = "create"
		} else if fromPath == toPath {
			// Editted in place
			//   like `nano etc/file.txt`
			commitFiles[toPath] = "create"
		} else {
			// Anything else - no idea why this would happen
			commitFiles[fromPath] = "unsupported"
		}

		// Check for new symbolic links and add target file in actions for creation on remote hosts
		if commitFileToType == "symlink" && commitFiles[toPath] == "create" {
			// Get the target path of the sym link target and ensure it is valid
			var targetPath string
			targetPath, err = ResolveLinkToTarget(toPath)
			if err != nil {
				err = fmt.Errorf("failed resolving symbolic link")
				return
			}

			// Add new action to this file that includes the expected target path for the link
			commitFiles[toPath] = "symlinkcreate to target " + targetPath
		}
	}

	// Error if output of processed data is not present
	if len(commitFiles) == 0 {
		err = fmt.Errorf("no valid files in commit")
		return
	}

	// Show progres to user
	fmt.Print("Complete.\n")
	return
}

func getHostsAndFiles(commitFiles map[string]string, commitHostNames []string, repoHostsandFiles map[string][]string, DeployerEndpoints map[string][]DeployerEndpoints, hostOverride string, preDeploymentHosts *int) (hostsAndFilePaths map[string][]string, hostsAndEndpointInfo map[string][]string, targetEndpoints []string, allLocalFiles []string, err error) {
	// Show progress to user
	fmt.Print("Filtering deployment hosts... ")

	// Initialize maps
	hostsAndFilePaths = make(map[string][]string)    // Map of hosts and their arrays of file paths
	hostsAndEndpointInfo = make(map[string][]string) // Map of hosts and their associated endpoint information ([0]=Socket, [1]=User)

	// Loop hosts in config and prepare endpoint information and relevant configs for deployment
	for endpointName, endpointInfo := range DeployerEndpoints {
		// Used for fail tracker manual deployments - skip this host if not in override (if override was requested)
		SkipHost := checkForHostOverride(hostOverride, endpointName)
		if SkipHost {
			continue
		}

		// Ensure processing is only done for hosts which might have a config deployed - as identified by parse git commit section
		var noHostMatchFound bool
		for _, targetHost := range commitHostNames {
			if endpointName == targetHost || targetHost == UniversalDirectory {
				noHostMatchFound = false
				break
			}
			noHostMatchFound = true
		}
		if noHostMatchFound {
			continue
		}

		// Extract var for endpoint information
		endpointUser := endpointInfo[2].EndpointUser

		// Network Pre-Checks and Parsing
		var endpoint string
		endpoint, err = ParseEndpointAddress(endpointInfo[0].Endpoint, endpointInfo[1].EndpointPort)
		if err != nil {
			err = fmt.Errorf("failed parsing host '%s' network address: %v", endpointName, err)
			return
		}

		// Add endpoint info to The Maps
		hostsAndEndpointInfo[endpointName] = append(hostsAndEndpointInfo[endpointName], endpoint, endpointUser)

		// If the ignore index of the host is present, read in bool - for use in deduping
		var ignoreUniversalConfs bool
		if len(endpointInfo) == 4 {
			ignoreUniversalConfs = endpointInfo[3].IgnoreUniversalConfs
		}

		// Record universal files that are NOT to be used for this host (host has an override file)
		var deniedUniversalFiles []string
		for _, hostFile := range repoHostsandFiles[endpointName] {
			for _, universalFile := range repoHostsandFiles[UniversalDirectory] {
				// Deny the universal file when it has the same name as a file in the host directory
				if hostFile == universalFile {
					deniedFilePath := filepath.Join(UniversalDirectory, universalFile)
					deniedUniversalFiles = append(deniedUniversalFiles, deniedFilePath)
				}
			}
		}

		// Filter committed files to their specific host and deduplicate against universal directory
		for commitFile := range commitFiles {
			// Split out the host part of the committed file path
			HostAndPath := strings.SplitN(commitFile, OSPathSeparator, 2)
			commitHost := HostAndPath[0]

			// Format a commitFilePath with the expected remote host path separators
			filePath := strings.ReplaceAll(commitFile, OSPathSeparator, "/")

			// Skip files not relevant to this host
			if commitHost != endpointName && commitHost != UniversalDirectory {
				continue
			}

			// Skip Universal files if this host ignores universal configs
			if ignoreUniversalConfs && commitHost == UniversalDirectory {
				continue
			}

			// Skip if commitFile is a universal file that is not allowed for this host
			var SkipFile bool
			for _, deniedUniversalFile := range deniedUniversalFiles {
				if commitFile == deniedUniversalFile {
					SkipFile = true
					break
				}
			}
			if SkipFile {
				continue
			}

			// Adding file to deployment lists - commitFile is either a host specific file or an allowed Universal file
			allLocalFiles = append(allLocalFiles, commitFile)
			hostsAndFilePaths[endpointName] = append(hostsAndFilePaths[endpointName], filePath)
		}

		// Add filtered endpoints to The Maps (array) - this is the main reference for which hosts will have a routine spawned
		targetEndpoints = append(targetEndpoints, endpointName)

		// Increment count of hosts to be deployed for metrics
		*preDeploymentHosts++
	}

	// Error if theres nothing left after filtering
	if len(targetEndpoints) == 0 {
		err = fmt.Errorf("all hosts filtered out, nothing left to deploy")
		return
	}
	if len(allLocalFiles) == 0 {
		err = fmt.Errorf("all files filtered out, nothing left to deploy")
		return
	}

	// Show progress to user
	fmt.Print("Complete.\n")
	return
}

func loadFiles(allLocalFiles []string, commitFiles map[string]string, tree *object.Tree, preDeployedConfigs *int) (commitFileData map[string]string, commitFileMetadata map[string]map[string]interface{}, commitFileDataHashes map[string]string, commitFileActions map[string]string, err error) {
	// Show progress to user
	fmt.Print("Loading files for deployment... ")

	// Initialize maps
	commitFileData = make(map[string]string)                     // Map of target file paths and their associated content
	commitFileMetadata = make(map[string]map[string]interface{}) // Map of target file paths and their associated extracted metadata
	commitFileDataHashes = make(map[string]string)               // Map of target file paths and their associated content hashes
	commitFileActions = make(map[string]string)                  // Map of target file paths and their associated file actions

	// Load file contents, metadata, hashes, and actions into their own maps
	for _, commitFilePath := range allLocalFiles {
		// Ensure paths for deployment have correct separate for linux
		filePath := strings.ReplaceAll(commitFilePath, OSPathSeparator, "/")
		// As a reminder
		// filePath		should be identical to the full path of files in the repo except hard coded to forward slash path separators
		// commitFilePath	should be identical to the full path of files in the repo (meaning following the build OS file path separators)

		// Skip loading if file will be deleted
		if commitFiles[commitFilePath] == "delete" {
			// But, add it to the deploy target files so it can be deleted during ssh
			commitFileActions[filePath] = commitFiles[commitFilePath]
			continue
		}

		// Skip loading if file is sym link
		if strings.Contains(commitFiles[commitFilePath], "symlinkcreate") {
			// But, add it to the deploy target files so it can be ln'd during ssh
			commitFileActions[filePath] = commitFiles[commitFilePath]
			continue
		}

		// Get file from git tree
		var file *object.File
		file, err = tree.File(commitFilePath)
		if err != nil {
			err = fmt.Errorf("failed retrieving file from git tree: %v", err)
			return
		}

		// Open reader for file contents
		var reader io.ReadCloser
		reader, err = file.Reader()
		if err != nil {
			err = fmt.Errorf("failed retrieving file reader: %v", err)
			return
		}
		defer reader.Close()

		// Read file contents (as bytes)
		var content []byte
		content, err = io.ReadAll(reader)
		if err != nil {
			err = fmt.Errorf("failed reading file content: %v", err)
			return
		}

		// Grab metadata out of contents
		var metadata, configContent string
		metadata, configContent, err = extractMetadata(string(content))
		if err != nil {
			err = fmt.Errorf("failed to extract metadata header: %v", err)
			return
		}

		// SHA256 Hash the metadata-less contents
		contentBytes := []byte(configContent)
		hash := sha256.New()
		hash.Write(contentBytes)
		hashedBytes := hash.Sum(nil)

		// Parse JSON into a generic map
		var jsonMetadata MetaHeader
		err = json.Unmarshal([]byte(metadata), &jsonMetadata)
		if err != nil {
			err = fmt.Errorf("failed parsing JSON metadata header for %s: %v", commitFilePath, err)
			return
		}

		// Initialize inner map for metadata
		commitFileMetadata[filePath] = make(map[string]interface{})

		// Save metadata into its own map
		commitFileMetadata[filePath]["FileOwnerGroup"] = jsonMetadata.TargetFileOwnerGroup
		commitFileMetadata[filePath]["FilePermissions"] = jsonMetadata.TargetFilePermissions
		commitFileMetadata[filePath]["ReloadRequired"] = jsonMetadata.ReloadRequired
		commitFileMetadata[filePath]["Reload"] = jsonMetadata.ReloadCommands

		// Save Hashes into its own map
		commitFileDataHashes[filePath] = hex.EncodeToString(hashedBytes)

		// Save content into its own map
		commitFileData[filePath] = configContent

		// Save file paths and actions into its own map
		commitFileActions[filePath] = commitFiles[commitFilePath]

		// Increment config metric counter
		*preDeployedConfigs++
	}

	// Error if theres nothing left after filtering
	if len(commitFileActions) == 0 {
		err = fmt.Errorf("no files to load")
		return
	}

	// Show progress to user
	fmt.Print("Complete.\n")
	return
}

func getCommitFailures(lastFailTracker string) (commitFiles map[string]string, commitHostNames []string, err error) {
	// Initialize commitFiles map
	commitFiles = make(map[string]string)

	// Temporary map to not clobber repeating files
	failedFiles := make(map[string]string)

	// Retrieve failed hosts and files from failtracker json by line
	FailLines := strings.Split(lastFailTracker, "\n")
	for _, fail := range FailLines {
		// Skip any empty lines
		if fail == "" {
			continue
		}

		// Use global struct for errors json format
		var errorInfo ErrorInfo

		// Unmarshal the line into vars
		err = json.Unmarshal([]byte(fail), &errorInfo)
		if err != nil {
			err = fmt.Errorf("issue unmarshaling json: %v", err)
			return
		}

		// error if no hostname
		if errorInfo.EndpointName == "" {
			err = fmt.Errorf("hostname is empty: failtracker line: %s", fail)
			return
		}

		// Add failed hosts to isolate host deployment loop to only those hosts
		commitHostNames = append(commitHostNames, errorInfo.EndpointName)

		// error if no files
		if len(errorInfo.Files) == 0 {
			err = fmt.Errorf("no files in failtracker line: %s", fail)
			return
		}

		// Add failed files to array (Only create, deleted/symlinks dont get added to failtracker)
		for _, failedFile := range errorInfo.Files {
			failedFiles[failedFile] = "create"
		}
	}

	// Add complete map to return map
	commitFiles = failedFiles
	return
}

func mapAllRepoFiles(tree *object.Tree) (repoHostsandFiles map[string][]string, err error) {
	// Get list of all files in repo tree
	allFiles := tree.Files()

	// Record all files in repo to the map
	repoHostsandFiles = make(map[string][]string)
	for {
		// Go to next file in list
		var repoFile *object.File
		repoFile, err = allFiles.Next()
		if err != nil {
			// Break at end of list
			if err == io.EOF {
				err = nil
				return
			}

			// Fail if next file doesnt work
			err = fmt.Errorf("failed retrieving commit file: %v", err)
			return
		}

		// Get file path
		repoFilePath := repoFile.Name

		// Always ignore files in root of repository
		if !strings.ContainsRune(repoFilePath, []rune(OSPathSeparator)[0]) {
			continue
		}

		// Split host and path
		commitSplit := strings.SplitN(repoFilePath, OSPathSeparator, 2)
		commitHost := commitSplit[0]
		commitPath := commitSplit[1]

		// Create map for each host dir in repo
		repoHostsandFiles[commitHost] = append(repoHostsandFiles[commitHost], commitPath)
	}

	return
}

func getRepoFiles(tree *object.Tree, fileOverride string) (commitFiles map[string]string, repoHostsandFiles map[string][]string, err error) {
	// Initialize the commitFiles map
	commitFiles = make(map[string]string)
	fileOverrides := strings.Split(fileOverride, ",")

	// Get list of all files in repo tree
	allFiles := tree.Files()

	// Record all files in repo to the map
	repoHostsandFiles = make(map[string][]string)
	for {
		// Go to next file in list
		var repoFile *object.File
		repoFile, err = allFiles.Next()
		if err != nil {
			// Break at end of list
			if err == io.EOF {
				err = nil
				return
			}

			// Fail if next file doesnt work
			err = fmt.Errorf("failed retrieving commit file: %v", err)
			return
		}

		// Get file path
		repoFilePath := repoFile.Name

		// Always ignore files in root of repository
		if !strings.ContainsRune(repoFilePath, []rune(OSPathSeparator)[0]) {
			continue
		}

		// Split host and path
		commitSplit := strings.SplitN(repoFilePath, OSPathSeparator, 2)
		commitHost := commitSplit[0]
		commitPath := commitSplit[1]

		// Create map for each host dir in repo
		repoHostsandFiles[commitHost] = append(repoHostsandFiles[commitHost], commitPath)

		// Skip file if not user requested file (if requested)
		if len(fileOverrides) > 0 {
			var fileNotRequested bool
			for _, overrideFile := range fileOverrides {
				if repoFilePath == overrideFile {
					continue
				}
				fileNotRequested = true
			}
			if fileNotRequested {
				continue
			}
		}

		// Add repo file to the commit map with always create action
		commitFiles[repoFilePath] = "create"

		// If its a symlink - find target and add
		fileMode := fmt.Sprintf("%v", repoFile.Mode)
		fileType := determineFileType(fileMode)
		if fileType == "symlink" {
			// Get the target path of the sym link target and ensure it is valid
			var targetPath string
			targetPath, err = ResolveLinkToTarget(repoFilePath)
			if err != nil {
				err = fmt.Errorf("failed to parse symbolic link in commit: %v", err)
				return
			}

			// Add new action to this file that includes the expected target path for the link
			commitFiles[repoFilePath] = "symlinkcreate to target " + targetPath
		}
	}

	return
}
