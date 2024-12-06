// controller
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Retrieves file names and associated host names for given commit
// Returns the changed files (file paths) between commit and previous commit
// Marks files with create/delete action for deployment and also handles marking symbolic links
func getCommitFiles(commit *object.Commit, DeployerEndpoints map[string]DeployerEndpoints, fileOverride string) (commitFiles map[string]string, commitHostNames []string, err error) {
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

	// If file override array ispresent, split into fields
	var fileOverrides []string
	if len(fileOverride) > 0 {
		fileOverrides = strings.Split(fileOverride, ",")
	}

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

	// Return specific error if no files - usually when user changes and commits files outside of host directories
	// Usually occurs when all files are validated as "unsupported" - see above
	// Don't change error text - used to return program back to main
	if len(commitFiles) == 0 {
		err = fmt.Errorf("no valid files in commit")
		return
	}

	// Show progres to user
	fmt.Print("Complete.\n")
	return
}

// Filters files down to their associated host
// Also deduplicates and creates array of all relevant file paths for the deployment
func getHostsAndFiles(commitFiles map[string]string, commitHostNames []string, repoHostsandFiles map[string][]string, DeployerEndpoints map[string]DeployerEndpoints, hostOverride string, SSHClientDefault SSHClientDefaults, preDeploymentHosts *int) (hostsAndFilePaths map[string][]string, hostsAndEndpointInfo map[string]EndpointInfo, targetEndpoints []string, allLocalFiles []string, err error) {
	// Show progress to user
	fmt.Print("Filtering deployment hosts... ")

	// Initialize maps
	hostsAndFilePaths = make(map[string][]string)        // Map of hosts and their arrays of file paths
	hostsAndEndpointInfo = make(map[string]EndpointInfo) // Map of hosts and their associated endpoint information

	// Loop hosts in config and prepare endpoint information and relevant configs for deployment
	for endpointName, endpointInfo := range DeployerEndpoints {
		// Skip this host if not in override (if override was requested)
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

		// Get ignore universal bool from endpoint - No defaults, empty implies false
		ignoreUniversalConfs := endpointInfo.IgnoreUniversalConfs

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

		// Skip this host if no files to deploy
		if len(hostsAndFilePaths[endpointName]) == 0 {
			continue
		}

		// Parse out endpoint info and/or default SSH options
		var newInfo EndpointInfo
		newInfo, err = retrieveEndpointInfo(endpointInfo, SSHClientDefault)
		if err != nil {
			err = fmt.Errorf("failed to retrieve endpoint information: %v", err)
			return
		}
		hostsAndEndpointInfo[endpointName] = newInfo

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

// Retrieves all file content for this deployment
// Return vales provide the content keyed on local file path for the file data, metadata, hashes, and actions
func loadFiles(allLocalFiles []string, commitFiles map[string]string, tree *object.Tree, preDeployedConfigs *int) (commitFileInfo map[string]CommitFileInfo, err error) {
	// Show progress to user
	fmt.Print("Loading files for deployment... ")

	// Initialize map of all local file paths and their associated info (content, metadata, hashes, and actions)
	commitFileInfo = make(map[string]CommitFileInfo)

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
			commitFileInfo[filePath] = CommitFileInfo{Action: commitFiles[commitFilePath]}
			continue
		}

		// Skip loading if file is sym link
		if strings.Contains(commitFiles[commitFilePath], "symlinkcreate") {
			// But, add it to the deploy target files so it can be ln'd during ssh
			commitFileInfo[filePath] = CommitFileInfo{Action: commitFiles[commitFilePath]}
			continue
		}

		// Skip loading other file types - safety blocker
		if commitFiles[commitFilePath] != "create" {
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
		contentHash := SHA256Sum(configContent)

		// Parse JSON into a generic map
		var jsonMetadata MetaHeader
		err = json.Unmarshal([]byte(metadata), &jsonMetadata)
		if err != nil {
			err = fmt.Errorf("failed parsing JSON metadata header for %s: %v", commitFilePath, err)
			return
		}

		// Put all information gathered into struct
		var info CommitFileInfo
		info.FileOwnerGroup = jsonMetadata.TargetFileOwnerGroup
		info.FilePermissions = jsonMetadata.TargetFilePermissions
		info.ReloadRequired = jsonMetadata.ReloadRequired
		info.Reload = jsonMetadata.ReloadCommands
		info.Hash = contentHash
		info.Data = configContent
		info.Action = commitFiles[commitFilePath]

		// Save info struct into map for this file
		commitFileInfo[filePath] = info

		// Increment config metric counter
		*preDeployedConfigs++
	}

	// Error if theres nothing left after filtering
	if len(commitFileInfo) == 0 {
		err = fmt.Errorf("no files to load")
		return
	}

	// Show progress to user
	fmt.Print("Complete.\n")
	return
}

// Reads fail tracker file and retrieves the files and host names to be redeployed
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

// Retrieves all files for current commit (regardless if changed)
// Used to deduplicate files per host when deploying
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

// Retrieves all files for current commit (regardless if changed)
// Used to deduplicate files per host when deploying
// This variant is used to also get all files in commit for deployment of unchanged files when requested
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
		if len(fileOverride) > 0 {
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
