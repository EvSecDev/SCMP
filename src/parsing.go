// controller
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"slices"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Retrieves file paths and file mode for a given commit
func getChangedFiles(commit *object.Commit) (changedFiles []GitChangedFileMetadata, err error) {
	printMessage(verbosityProgress, "Retrieving changed files from commit... \n")

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

	for _, file := range patch.FilePatches() {
		var changedFile GitChangedFileMetadata

		from, to := file.Files()

		// Must safely retrieve file information to avoid panic
		if from != nil {
			_, err = os.Stat(changedFile.fromPath)
			if err != nil {
				// Any error other than file is not present, return
				if !strings.Contains(err.Error(), "no such file or directory") {
					return
				}
				err = nil

				// Actual on-disk file is missing
				changedFile.fromNotOnFS = true
			}

			changedFile.fromPath = from.Path()
			changedFile.fromMode = from.Mode().String()
		}
		if to != nil {
			_, err = os.Stat(changedFile.fromPath)
			if err != nil {
				// Any error other than file is not present, return
				if !strings.Contains(err.Error(), "no such file or directory") {
					return
				}
				err = nil

				// Actual on-disk file is missing
				changedFile.toNotOnFS = true
			}

			changedFile.toPath = to.Path()
			changedFile.toMode = to.Mode().String()
		}

		changedFiles = append(changedFiles, changedFile)
	}
	return
}

// Parses changed files according to presence, path, and mode validity
// Marks files with create/delete/modify action for deployment
func parseChangedFiles(changedFiles []GitChangedFileMetadata, fileOverride string) (commitFiles map[string]string, err error) {
	printMessage(verbosityProgress, "Parsing commit files\n")

	commitFiles = make(map[string]string)

	for _, changedFile := range changedFiles {
		// If either from/to path matches user request, continue parsing
		// TODO:
		//  If user provided override for a deleted file
		//   (that was moved, so source was deleted, destination is still present)
		//   both the deletion and the moved file will be added to deployment
		//  So user specifying only delete will actually get delete and create
		skipFromFile := checkForOverride(fileOverride, changedFile.fromPath)
		skipToFile := checkForOverride(fileOverride, changedFile.toPath)
		if skipToFile && skipFromFile {
			printMessage(verbosityFullData, "  File not desired\n")
			continue
		}

		fromFileIsValid := fileIsValid(changedFile.fromPath, changedFile.fromMode)
		toFileIsValid := fileIsValid(changedFile.toPath, changedFile.toMode)

		if changedFile.fromPath == "" && changedFile.toPath == "" {
			continue
		} else if changedFile.fromPath == "" && toFileIsValid {
			// Newly created files
			//   like `touch etc/file.txt`
			if strings.HasSuffix(changedFile.toPath, directoryMetadataFileName) {
				printMessage(verbosityFullData, "  Dir Metadata '%s' is brand new and will affect parent\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = "dirCreate"
			} else {
				printMessage(verbosityFullData, "  File '%s' is brand new and to be created\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = "create"
			}
		} else if changedFile.toPath == "" && fromFileIsValid {
			// Deleted Files
			//   like `rm etc/file.txt`
			if config.options.allowDeletions {
				printMessage(verbosityFullData, "  File '%s' is to be deleted\n", changedFile.fromPath)
				commitFiles[changedFile.fromPath] = "delete"
			} else {
				printMessage(verbosityProgress, "  Skipping deletion of file '%s'\n", changedFile.fromPath)
			}
		} else if changedFile.fromPath != changedFile.toPath && fromFileIsValid && toFileIsValid {
			// Copied or renamed files
			//   like `cp etc/file.txt etc/file2.txt` or `mv etc/file.txt etc/file2.txt`

			if changedFile.fromNotOnFS {
				fromDirs := strings.Split(changedFile.fromPath, "/")
				topLevelDirFrom := fromDirs[0]
				toDirs := strings.Split(changedFile.toPath, "/")
				topLevelDirTo := toDirs[0]

				if topLevelDirFrom != topLevelDirTo {
					// File was moved between hosts - must remove source
					if config.options.allowDeletions {
						printMessage(verbosityFullData, "  File '%s' is to be deleted\n", changedFile.fromPath)
						commitFiles[changedFile.fromPath] = "delete"
					} else {
						printMessage(verbosityProgress, "  Skipping deletion of file '%s'\n", changedFile.fromPath)
					}
				} else if config.options.allowDeletions {
					printMessage(verbosityFullData, "  File '%s' is to be deleted\n", changedFile.fromPath)
					commitFiles[changedFile.fromPath] = "delete"
				}
			}

			if strings.HasSuffix(changedFile.toPath, directoryMetadataFileName) {
				printMessage(verbosityFullData, "  Dir Metadata '%s' is modified and will modify target directory\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = "dirModify"
			} else {
				printMessage(verbosityProgress, "  File '%s' is modified and to be created\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = "create"
			}
		} else if changedFile.fromPath == changedFile.toPath && fromFileIsValid && toFileIsValid {
			// Editted in place
			//   like `nano etc/file.txt`
			if strings.HasSuffix(changedFile.toPath, directoryMetadataFileName) {
				printMessage(verbosityFullData, "  Dir Metadata '%s' is modified in place and will modify target directory\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = "dirModify"
			} else {
				printMessage(verbosityFullData, "  File '%s' is modified in place and to be created\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = "create"
			}
		} else {
			printMessage(verbosityFullData, "  File '%s' unsupported\n", changedFile.fromPath)
		}
	}

	if len(commitFiles) == 0 {
		err = fmt.Errorf("something went wrong, no changed files found in commit")
		return
	}

	return
}

// Retrieves all files for current commit (regardless if changed)
// This is used to also get all files in commit for deployment of unchanged files when requested
func getRepoFiles(tree *object.Tree, fileOverride string) (commitFiles map[string]string, err error) {
	// Initialize maps
	commitFiles = make(map[string]string)

	// Get list of all files in repo tree
	allFiles := tree.Files()

	printMessage(verbosityProgress, "Retrieving all files in repository\n")

	// Use all repository files to create map and array of files/hosts
	for {
		// Go to next file in list
		var repoFile *object.File
		repoFile, err = allFiles.Next()
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

		// Get file path
		repoFilePath := repoFile.Name

		printMessage(verbosityData, "  Filtering file %s\n", repoFilePath)

		if !fileIsValid(repoFilePath, repoFile.Mode.String()) {
			continue
		}

		// Skip file if not user requested file (if requested)
		skipFile := checkForOverride(fileOverride, repoFilePath)
		if skipFile {
			printMessage(verbosityFullData, "    File not desired\n")
			continue
		}

		printMessage(verbosityData, "    File available\n")

		// Decide if file is dir metadata or actual config
		if strings.HasSuffix(repoFilePath, directoryMetadataFileName) {
			commitFiles[repoFilePath] = "dirCreate"
		} else {
			// Add repo file to the commit map with always create action
			commitFiles[repoFilePath] = "create"
		}
	}

	return
}

// Uses global config and deployment files to create list of files and hosts specific to deployment
// Also deduplicates host and universal to ensure host override files don't get clobbered
func filterHostsAndFiles(deniedUniversalFiles map[string]map[string]struct{}, commitFiles map[string]string, hostOverride string) (allDeploymentHosts []string, allDeploymentFiles map[string]string) {
	// Show progress to user
	printMessage(verbosityProgress, "Filtering deployment hosts... \n")

	// Initialize maps for deployment info
	allDeploymentFiles = make(map[string]string) // Map of all (filtered) deployment files and their associated actions

	printMessage(verbosityProgress, "Creating files per host and all deployment files maps\n")

	// Loop hosts in config and prepare endpoint information and relevant configs for deployment
	for endpointName, hostInfo := range config.hostInfo {
		printMessage(verbosityData, "  Host %s: Filtering files...\n", endpointName)
		// Skip this host if not in override (if override was requested)
		skipHost := checkForOverride(hostOverride, endpointName)
		if skipHost {
			printMessage(verbosityFullData, "    Host not desired\n")
			continue
		}

		// Get Denied universal files for this host
		hostsDeniedUniversalFiles := deniedUniversalFiles[endpointName]

		// Filter committed files to their specific host and deduplicate against universal directory
		var filteredCommitFiles []string // Order of items in this array is directly linked to the order in which they are deployed per host
		for commitFile, commitFileAction := range commitFiles {
			printMessage(verbosityData, "    Filtering file %s\n", commitFile)

			// Split out the host part of the committed file path
			hostAndPath := strings.SplitN(commitFile, config.osPathSeparator, 2)
			commitHost := hostAndPath[0]

			// Skip files not relevant to this host (either file is local to host, in global universal dir, or in host group universal)
			_, hostIsInFilesUniversalGroup := hostInfo.universalGroups[commitHost]
			if commitHost != endpointName && !hostIsInFilesUniversalGroup {
				printMessage(verbosityFullData, "        File not for this host/host's universal group and not universal \n")
				continue
			}

			// Skip if commitFile is a universal file that is not allowed for this host
			_, fileIsDenied := hostsDeniedUniversalFiles[commitFile]
			if fileIsDenied {
				printMessage(verbosityFullData, "        File is universal and host has non-universal identical file\n")
				continue
			}

			printMessage(verbosityData, "        Selected\n")

			// Add file to the host-specific file list and the global deployment file map
			allDeploymentFiles[commitFile] = commitFileAction
			filteredCommitFiles = append(filteredCommitFiles, commitFile)
		}

		// Skip this host if no files to deploy
		if len(filteredCommitFiles) == 0 {
			continue
		}

		// Write all deployment info for this host into the global map
		hostInfo.deploymentFiles = filteredCommitFiles
		config.hostInfo[endpointName] = hostInfo

		// Track hosts for deployment
		allDeploymentHosts = append(allDeploymentHosts, endpointName)
	}

	return
}

// Writes hosts secrest (key, password) into received map
func retrieveHostSecrets(oldHostInfo EndpointInfo) (newHostInfo EndpointInfo, err error) {
	// Copy current global config for this host to local
	newHostInfo = oldHostInfo

	printMessage(verbosityData, "    Retrieving endpoint key\n")

	// Get SSH Private Key from the supplied identity file
	newHostInfo.privateKey, newHostInfo.keyAlgo, err = SSHIdentityToKey(newHostInfo.identityFile)
	if err != nil {
		err = fmt.Errorf("failed to retrieve private key: %v", err)
		return
	}

	printMessage(verbosityFullData, "      Key: %d\n", newHostInfo.privateKey)

	// Retrieve password if required
	if newHostInfo.requiresVault {
		newHostInfo.password, err = unlockVault(newHostInfo.endpointName)
		if err != nil {
			err = fmt.Errorf("error retrieving host password from vault: %v", err)
			return
		}

		printMessage(verbosityFullData, "      Password: %s\n", newHostInfo.password)
	} else {
		printMessage(verbosityFullData, "      Host does not require password\n")
	}

	return
}

// Retrieves all file content for this deployment
func loadGitFileContent(allDeploymentFiles map[string]string, tree *object.Tree) (rawFileContent map[string][]byte, err error) {
	printMessage(verbosityProgress, "Loading files for deployment... \n")

	rawFileContent = make(map[string][]byte)

	for repoFilePath, commitFileAction := range allDeploymentFiles {
		if commitFileAction == "delete" {
			continue
		}

		printMessage(verbosityData, "  Loading repository file %s\n", repoFilePath)

		// Get file from git tree
		file, lerr := tree.File(repoFilePath)
		if lerr != nil {
			err = fmt.Errorf("failed retrieving file information from git tree: %v", lerr)
			return
		}

		reader, lerr := file.Reader()
		if lerr != nil {
			err = fmt.Errorf("failed retrieving file reader: %v", lerr)
			return
		}
		defer reader.Close()

		content, lerr := io.ReadAll(reader)
		if lerr != nil {
			err = fmt.Errorf("failed reading file content: %v", lerr)
			return
		}

		rawFileContent[repoFilePath] = content
	}

	return
}

// Parses loaded file content and retrieves needed metadata
// Return vales provide the content keyed on local file path for the file data, metadata, hashes, and actions
func parseFileContent(allDeploymentFiles map[string]string, rawFileContent map[string][]byte) (allFileMeta map[string]FileInfo, allFileData map[string][]byte, err error) {
	printMessage(verbosityProgress, "Parsing files for deployment... \n")

	// Initialize maps
	allFileMeta = make(map[string]FileInfo) // File metadata
	allFileData = make(map[string][]byte)   // File data

	// Load file contents, metadata, hashes, and actions into their own maps
	for repoFilePath, commitFileAction := range allDeploymentFiles {
		printMessage(verbosityData, "  Parsing repository file %s\n", repoFilePath)
		printMessage(verbosityData, "    Marked as '%s'\n", commitFileAction)

		// Actions that do not require content loading
		if commitFileAction == "delete" {
			// Add it to the deploy target files so it can be deleted during ssh
			_, deletedFilePath := translateLocalPathtoRemotePath(repoFilePath)
			allFileMeta[repoFilePath] = FileInfo{action: commitFileAction, targetFilePath: deletedFilePath}
			continue
		} else if commitFileAction != "create" && commitFileAction != "dirCreate" && commitFileAction != "dirModify" {
			// Skip unsupported file types - safety blocker
			continue
		}

		content := rawFileContent[repoFilePath]

		// Retrieve metadata depending on if this is a directory or a file
		fileContent, jsonMetadata, lerr := extractMetadataFromContents(repoFilePath, content)
		if lerr != nil {
			err = fmt.Errorf("failed to separate metadata from file content: %v", lerr)
			return
		}

		// Retrieve actual artifact contents and hash
		var contentHash string
		if len(jsonMetadata.ExternalContentLocation) > 0 {
			fileContent, contentHash, err = loadArtifactContent(jsonMetadata.ExternalContentLocation, repoFilePath, fileContent, allFileData)
			if err != nil {
				err = fmt.Errorf("failed to load artifact file content: %v", err)
				return
			}
		} else if len(fileContent) > 0 {
			printMessage(verbosityData, "    Hashing file content\n")

			// Hash the metadata-less contents
			contentHash = SHA256Sum(fileContent)
		}

		// Put all metadata gathered into map
		allFileMeta[repoFilePath] = jsonToFileInfo(repoFilePath, jsonMetadata, len(fileContent), commitFileAction, contentHash)

		// Put file content into map (do not load sym link contents)
		_, fileContentAlreadyStored := allFileData[contentHash]
		if !fileContentAlreadyStored && allFileMeta[repoFilePath].action != "symlinkCreate" {
			allFileData[contentHash] = fileContent
		}
	}

	// Guard against empty return value
	if len(allFileMeta) == 0 {
		err = fmt.Errorf("something went wrong, no files available to load")
		return
	}

	return
}

func getFailTrackerCommit() (commitID string, failures []string, err error) {
	printMessage(verbosityProgress, "Retrieving commit ID from failtracker file\n")

	// Regex to match commitid line from fail tracker
	failCommitRegEx := regexp.MustCompile(`commitid:([0-9a-fA-F]+)\n`)

	// Read in contents of fail tracker file
	lastFailTrackerBytes, err := os.ReadFile(config.failTrackerFilePath)
	if err != nil {
		return
	}

	// Convert tracker to string
	lastFailTracker := string(lastFailTrackerBytes)

	// Use regex to extract commit hash from line in fail tracker (should be the first line)
	commitRegexMatches := failCommitRegEx.FindStringSubmatch(lastFailTracker)

	// Extract the commit hash hex from the failtracker
	if len(commitRegexMatches) < 2 {
		err = fmt.Errorf("commitid missing from failtracker file")
		return
	}
	commitID = commitRegexMatches[1]

	// Remove commit line from the failtracker contents using the commit regex
	lastFailTracker = failCommitRegEx.ReplaceAllString(lastFailTracker, "")

	// Put failtracker failures into array
	failures = strings.Split(lastFailTracker, "\n")

	return
}

// Reads in last failtracker file and retrieves individual failures and the commitHash of the failure
func getFailedFiles(failures []string, fileOverride string) (commitFiles map[string]string, hostOverride string, err error) {
	// Initialize maps
	commitFiles = make(map[string]string)

	printMessage(verbosityProgress, "Parsing failtracker lines\n")

	// Retrieve failed hosts and files from failtracker json by line
	var hostOverrideArray []string
	for _, fail := range failures {
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

		printMessage(verbosityData, "Parsing failure for host %v\n", errorInfo.EndpointName)

		// Add host to override to isolate deployment to just the failed hosts
		hostOverrideArray = append(hostOverrideArray, errorInfo.EndpointName)

		// error if no files
		if len(errorInfo.Files) == 0 {
			err = fmt.Errorf("no files in failtracker line: %s", fail)
			return
		}

		// Add failed files to array (Only create, deleted/symlinks dont get added to failtracker)
		for _, failedFile := range errorInfo.Files {
			printMessage(verbosityData, "Parsing failure for file %s\n", failedFile)

			// Skip this file if not in override (if override was requested)
			skipFile := checkForOverride(fileOverride, failedFile)
			if skipFile {
				continue
			}

			printMessage(verbosityData, "Marked host %s - file %s for redeployment\n", errorInfo.EndpointName, failedFile)

			commitFiles[failedFile] = "create"
		}
	}
	// Convert to standard format for override
	hostOverride = strings.Join(hostOverrideArray, ",")

	return
}

// Correct the order of deployment based on any present dependencies
func handleFileDependencies(rawDeploymentFiles []string, allFileMeta map[string]FileInfo) (orderedDeploymentFiles []string, err error) {
	depCount := make(map[string]int)
	graph := make(map[string][]string)
	fileSet := make(map[string]bool)

	// Make map of files for this host for easy lookups of file existence
	for _, file := range rawDeploymentFiles {
		fileSet[file] = true
	}

	// Create dependency graph
	for file, info := range allFileMeta {
		for _, dep := range info.dependencies {
			// If dependency is not part of this deployment, skip adding to graph
			if !fileSet[dep] {
				continue
			}
			graph[dep] = append(graph[dep], file)
			depCount[file]++
		}
	}

	// Separate list of files with no dependencies
	noDeps := []string{}
	for _, file := range rawDeploymentFiles {
		if depCount[file] == 0 {
			noDeps = append(noDeps, file)
		}
	}
	sort.Strings(noDeps) // Ensure no_dependency files are in lexiconographical order

	queue := noDeps // Start queue from files with no dependents
	result := []string{}
	count := 0

	for len(queue) > 0 {
		file := queue[0]              // Get lead item in queue
		queue = queue[1:]             // Remove lead item in queue
		result = append(result, file) // Add lead item to result
		count++                       // Count of processed files for circular dep detection

		// Add dependents to result when immediately "attached" to parent
		for _, neighbor := range graph[file] {
			depCount[neighbor]-- // Decrease dependent count for processed dependent

			// When file has no more parents, add to queue to get added to result
			if depCount[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Return immediately if circular dependency was encountered
	if count != len(rawDeploymentFiles) {
		err = fmt.Errorf("circular dependency detected, unable to continue: deployment files: '%v'", rawDeploymentFiles)
		return
	}

	// Retrieve solo slice of dependents
	depFiles := []string{}
	for _, file := range result {
		found := slices.Contains(noDeps, file)

		if !found {
			depFiles = append(depFiles, file)
		}
	}

	// Always append dependencies after files with no dependencies
	orderedDeploymentFiles = append(noDeps, depFiles...)
	return
}
