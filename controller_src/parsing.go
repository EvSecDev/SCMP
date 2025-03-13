// controller
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Retrieves file names and associated host names for given commit
// Returns the changed files (file paths) between commit and previous commit
// Marks files with create/delete action for deployment and also handles marking symbolic links
func getCommitFiles(commit *object.Commit, fileOverride string) (commitFiles map[string]string, err error) {
	// Show progress to user
	printMessage(VerbosityStandard, "Retrieving files from commit... \n")

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

	printMessage(VerbosityProgress, "Parsing commit files\n")

	// Initialize maps
	commitFiles = make(map[string]string)

	// Determine what to do with each file in the commit
	for _, file := range patch.FilePatches() {
		// Get the old file and new file info
		from, to := file.Files()

		// Declare vars
		var fromPath, toPath, commitFileToType string
		var SkipToFile bool

		// Validate the from File object
		fromPath, _, _, err = validateCommittedFiles(from, fileOverride)
		if err != nil {
			return
		}

		// Validate the to File object
		toPath, commitFileToType, SkipToFile, err = validateCommittedFiles(to, fileOverride)
		if err != nil {
			return
		}

		// Skip if not valid - only check tofile (valid deployment with tofile could include invalid fromfile)
		if SkipToFile {
			printMessage(VerbosityFullData, "  Skipping Invalid File '%s'\n", toPath)
			continue
		}

		// Add file to map depending on how it changed in this commit
		if from == nil {
			// Decide if file is dir metadata or actual config
			if strings.HasSuffix(toPath, directoryMetadataFileName) {
				printMessage(VerbosityFullData, "  Dir Metadata '%s' is brand new and will affect parent\n", toPath)
				commitFiles[toPath] = "dirCreate"
			} else {
				printMessage(VerbosityFullData, "  File '%s' is brand new and to be created\n", toPath)
				// Newly created files
				//   like `touch etc/file.txt`
				commitFiles[toPath] = "create"
			}
		} else if to == nil {
			printMessage(VerbosityFullData, "  File '%s' is to be deleted\n", fromPath)
			// Deleted Files
			//   like `rm etc/file.txt`
			if config.AllowDeletions {
				commitFiles[fromPath] = "delete"
			} else {
				printMessage(VerbosityProgress, "  Skipping deletion of file '%s'\n", fromPath)
			}
		} else if fromPath != toPath {
			printMessage(VerbosityFullData, "  File '%s' has been changed to file '%s'\n", fromPath, toPath)
			// Copied or renamed files
			//   like `cp etc/file.txt etc/file2.txt` or `mv etc/file.txt etc/file2.txt`
			_, err = os.Stat(fromPath)
			if err != nil {
				// Any error other than file is not present, return
				if !strings.Contains(err.Error(), "no such file or directory") {
					return
				}

				// Reset err var to prevent false-positive error return on prompt function
				err = nil

				// If file was moved within same host, don't prompt
				fromDirs := strings.Split(fromPath, "/")
				topLevelDirFrom := fromDirs[0]
				toDirs := strings.Split(toPath, "/")
				topLevelDirTo := toDirs[0]

				// Only prompt for file moves outside of current host
				if topLevelDirFrom != topLevelDirTo {
					if config.AllowDeletions {
						// Mark for deletion if no longer present in repo
						commitFiles[fromPath] = "delete"
						printMessage(VerbosityFullData, "  File '%s' is to be deleted\n", fromPath)
					} else {
						printMessage(VerbosityProgress, "  Skipping deletion of file '%s'\n", fromPath)
					}
				} else if config.AllowDeletions {
					commitFiles[fromPath] = "delete"
					printMessage(VerbosityFullData, "  File '%s' is to be deleted\n", fromPath)
				}
			}

			// Decide if file is dir metadata or actual config
			if strings.HasSuffix(toPath, directoryMetadataFileName) {
				printMessage(VerbosityFullData, "  Dir Metadata '%s' is modified and will modify target directory\n", toPath)
				commitFiles[toPath] = "dirModify"
			} else {
				// To path should always be present (otherwise it would be nil and caught earlier)
				printMessage(VerbosityProgress, "  File '%s' is modified and to be created\n", toPath)
				commitFiles[toPath] = "create"
			}
		} else if fromPath == toPath {
			// Decide if file is dir metadata or actual config
			if strings.HasSuffix(toPath, directoryMetadataFileName) {
				printMessage(VerbosityFullData, "  Dir Metadata '%s' is modified in place and will modify target directory\n", toPath)
				commitFiles[toPath] = "dirModify"
			} else {
				printMessage(VerbosityFullData, "  File '%s' is modified in place and to be created\n", toPath)
				// Editted in place
				//   like `nano etc/file.txt`
				commitFiles[toPath] = "create"
			}
		} else {
			printMessage(VerbosityFullData, "  File '%s' unknown and unsupported\n", fromPath)
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

	return
}

// Retrieves all files for current commit (regardless if changed)
// This is used to also get all files in commit for deployment of unchanged files when requested
func getRepoFiles(tree *object.Tree, fileOverride string) (commitFiles map[string]string, err error) {
	// Initialize maps
	commitFiles = make(map[string]string)

	// Get list of all files in repo tree
	allFiles := tree.Files()

	printMessage(VerbosityProgress, "Retrieving all files in repository\n")

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

		printMessage(VerbosityData, "  Filtering file %s\n", repoFilePath)

		// Ensure file is valid against config
		if repoFileIsValid(repoFilePath) {
			// Not valid, skip
			continue
		}

		// Skip file if not user requested file (if requested)
		skipFile := checkForOverride(fileOverride, repoFilePath)
		if skipFile {
			printMessage(VerbosityFullData, "    File not desired\n")
			continue
		}

		printMessage(VerbosityData, "    File available\n")

		// Decide if file is dir metadata or actual config
		if strings.HasSuffix(repoFilePath, directoryMetadataFileName) {
			commitFiles[repoFilePath] = "dirCreate"
		} else {
			// Add repo file to the commit map with always create action
			commitFiles[repoFilePath] = "create"
		}

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

// Uses global config and deployment files to create list of files and hosts specific to deployment
// Also deduplicates host and universal to ensure host override files don't get clobbered
func filterHostsAndFiles(deniedUniversalFiles map[string]map[string]struct{}, commitFiles map[string]string, hostOverride string) (allDeploymentHosts []string, allDeploymentFiles map[string]string) {
	// Show progress to user
	printMessage(VerbosityStandard, "Filtering deployment hosts... \n")

	// Initialize maps for deployment info
	allDeploymentFiles = make(map[string]string) // Map of all (filtered) deployment files and their associated actions

	printMessage(VerbosityProgress, "Creating files per host and all deployment files maps\n")

	// Loop hosts in config and prepare endpoint information and relevant configs for deployment
	for endpointName, hostInfo := range config.HostInfo {
		printMessage(VerbosityData, "  Host %s: Filtering files...\n", endpointName)
		// Skip this host if not in override (if override was requested)
		skipHost := checkForOverride(hostOverride, endpointName)
		if skipHost {
			printMessage(VerbosityFullData, "    Host not desired\n")
			continue
		}

		// Get Denied universal files for this host
		hostsDeniedUniversalFiles := deniedUniversalFiles[endpointName]

		// Filter committed files to their specific host and deduplicate against universal directory
		var filteredCommitFiles []string
		for commitFile, commitFileAction := range commitFiles {
			printMessage(VerbosityData, "    Filtering file %s\n", commitFile)

			// Split out the host part of the committed file path
			HostAndPath := strings.SplitN(commitFile, config.OSPathSeparator, 2)
			commitHost := HostAndPath[0]

			// Skip files not relevant to this host (either file is local to host, in global universal dir, or in host group universal)
			_, hostIsInFilesUniversalGroup := hostInfo.UniversalGroups[commitHost]
			if commitHost != endpointName && !hostIsInFilesUniversalGroup {
				printMessage(VerbosityFullData, "        File not for this host/host's universal group and not universal \n")
				continue
			}

			// Skip if commitFile is a universal file that is not allowed for this host
			_, fileIsDenied := hostsDeniedUniversalFiles[commitFile]
			if fileIsDenied {
				printMessage(VerbosityFullData, "        File is universal and host has non-universal identical file\n")
				continue
			}

			printMessage(VerbosityData, "        Selected\n")

			// Add file to the host-specific file list and the global deployment file map
			allDeploymentFiles[commitFile] = commitFileAction
			filteredCommitFiles = append(filteredCommitFiles, commitFile)
		}

		// Skip this host if no files to deploy
		if len(filteredCommitFiles) == 0 {
			continue
		}

		// Write all deployment info for this host into the global map
		hostInfo.DeploymentFiles = filteredCommitFiles
		config.HostInfo[endpointName] = hostInfo

		// Track hosts for deployment
		allDeploymentHosts = append(allDeploymentHosts, endpointName)
	}

	return
}

// Writes hosts secrest (key, password) into received map
func retrieveHostSecrets(endpointName string) (err error) {
	// Copy current global config for this host to local
	hostInfo := config.HostInfo[endpointName]

	printMessage(VerbosityData, "    Retrieving endpoint key\n")

	// Get SSH Private Key from the supplied identity file
	hostInfo.PrivateKey, hostInfo.KeyAlgo, err = SSHIdentityToKey(hostInfo.IdentityFile)
	if err != nil {
		err = fmt.Errorf("failed to retrieve private key: %v", err)
		return
	}
	printMessage(VerbosityFullData, "      Key: %d\n", hostInfo.PrivateKey)

	// Retrieve password if required
	if hostInfo.RequiresVault {
		hostInfo.Password, err = unlockVault(config.HostInfo[endpointName].EndpointName)
		if err != nil {
			err = fmt.Errorf("error retrieving host password from vault: %v", err)
			return
		}

		printMessage(VerbosityFullData, "      Password: %s\n", hostInfo.Password)
	} else {
		printMessage(VerbosityFullData, "      Host does not require password\n")
	}

	// Write host info back into global config
	config.HostInfo[endpointName] = hostInfo
	return
}

// Retrieves all file content for this deployment
// Return vales provide the content keyed on local file path for the file data, metadata, hashes, and actions
func loadFiles(allDeploymentFiles map[string]string, tree *object.Tree) (allFileInfo map[string]FileInfo, allFileData map[string][]byte, err error) {
	// Show progress to user
	printMessage(VerbosityStandard, "Loading files for deployment... \n")

	// Initialize map of all local file paths and their associated info (metadata, hashes, and actions)
	allFileInfo = make(map[string]FileInfo)

	// Initialize map of all local file content mapped by their hash
	allFileData = make(map[string][]byte)

	// Load file contents, metadata, hashes, and actions into their own maps
	for commitFilePath, commitFileAction := range allDeploymentFiles {
		printMessage(VerbosityData, "  Loading repository file %s\n", commitFilePath)

		printMessage(VerbosityData, "    Marked as 'to be %s'\n", commitFileAction)

		// Skip loading if file will be deleted
		if commitFileAction == "delete" {
			// But, add it to the deploy target files so it can be deleted during ssh
			allFileInfo[commitFilePath] = FileInfo{Action: commitFileAction}
			continue
		}

		// Skip loading if file is sym link
		if strings.Contains(commitFileAction, "symlinkcreate") {
			// But, add it to the deploy target files so it can be ln'd during ssh
			allFileInfo[commitFilePath] = FileInfo{Action: commitFileAction}
			continue
		}

		// Skip loading unsupported file types - safety blocker
		if commitFileAction != "create" && commitFileAction != "dirCreate" && commitFileAction != "dirModify" {
			continue
		}

		printMessage(VerbosityData, "    Retrieving file contents\n")

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

		printMessage(VerbosityData, "    Extracting file metadata\n")

		// Directory metadata
		if strings.HasSuffix(commitFilePath, directoryMetadataFileName) {
			// Get just directory name
			directoryName := filepath.Dir(commitFilePath)

			// Extract metadata
			var jsonDirMetadata MetaHeader
			err = json.Unmarshal(content, &jsonDirMetadata)
			if err != nil {
				err = fmt.Errorf("failed parsing directory JSON metadata for '%s': %v", directoryName, err)
				return
			}

			// Save Directory metadata to map
			var info FileInfo
			info.FileOwnerGroup = jsonDirMetadata.TargetFileOwnerGroup
			info.FilePermissions = jsonDirMetadata.TargetFilePermissions
			info.ReloadRequired = false
			info.Action = commitFileAction
			allFileInfo[commitFilePath] = info

			// Skip to next file
			continue
		}

		// Grab metadata out of contents
		var metadata string
		var fileContent []byte
		metadata, fileContent, err = extractMetadata(string(content))
		if err != nil {
			err = fmt.Errorf("failed to extract metadata header from '%s': %v", commitFilePath, err)
			return
		}

		printMessage(VerbosityData, "    Parsing metadata header JSON\n")

		// Parse JSON into a generic map
		var jsonMetadata MetaHeader
		err = json.Unmarshal([]byte(metadata), &jsonMetadata)
		if err != nil {
			err = fmt.Errorf("failed parsing JSON metadata header for %s: %v", commitFilePath, err)
			return
		}

		// If file is an artifact pointer, retrieve real file contents, else hash content itself
		var contentHash string
		if len(jsonMetadata.ExternalContentLocation) > 0 {
			// Only allow file URIs for now
			if !strings.HasPrefix(jsonMetadata.ExternalContentLocation, fileURIPrefix) {
				err = fmt.Errorf("remote-artifact file '%s': must use '%s' before file paths in 'ExternalContentLocationput' field", commitFilePath, fileURIPrefix)
				return
			}

			// Use hash already in pointer file as hash of actual artifact file contents
			contentHash = SHA256RegEx.FindString(string(fileContent))

			// Retrieve artifact file data if not already loaded
			_, artifactDataAlreadyLoaded := allFileData[contentHash]
			if !artifactDataAlreadyLoaded {
				// Not adhering to actual URI standards -- I just want file paths
				artifactFileName := strings.TrimPrefix(jsonMetadata.ExternalContentLocation, fileURIPrefix)

				// Check for ~/ and expand if required
				artifactFileName = expandHomeDirectory(artifactFileName)

				// Retrieve artifact file contents
				var artifactFileContents []byte
				artifactFileContents, err = os.ReadFile(artifactFileName)
				if err != nil {
					err = fmt.Errorf("failed to load artifact file data: %v", err)
					return
				}

				// Overwrite pointer file contents with actual file data
				fileContent = artifactFileContents
			}
		} else {
			printMessage(VerbosityData, "    Hashing file content\n")

			// SHA256 Hash the metadata-less contents
			contentHash = SHA256Sum(fileContent)
		}

		// Put all information gathered into struct
		var info FileInfo
		info.FileOwnerGroup = jsonMetadata.TargetFileOwnerGroup
		info.FilePermissions = jsonMetadata.TargetFilePermissions
		info.FileSize = len(fileContent)
		info.Reload = jsonMetadata.ReloadCommands
		if len(info.Reload) > 0 {
			// Reload commands are present, set bool to true
			info.ReloadRequired = true
		} else {
			// Reload commands are not present, set to false
			info.ReloadRequired = false
		}
		info.Checks = jsonMetadata.CheckCommands
		if len(info.Checks) > 0 {
			// Check commands are present, set bool to true
			info.ChecksRequired = true
		} else {
			// Check commands are not present, set to false
			info.ChecksRequired = false
		}
		info.Install = jsonMetadata.InstallCommands
		if len(info.Install) > 0 {
			// Install commands are present, set bool to true
			info.InstallOptional = true
		} else {
			// Install commands are not present, set to false
			info.InstallOptional = false
		}
		info.Hash = contentHash
		info.Action = commitFileAction

		// Save info struct into map for this file
		allFileInfo[commitFilePath] = info

		// Save data into map
		_, fileContentAlreadyStored := allFileData[contentHash]
		if !fileContentAlreadyStored {
			allFileData[contentHash] = fileContent
		}

		// Print verbose file metadata information
		printMessage(VerbosityFullData, "      Owner and Group:  %s\n", info.FileOwnerGroup)
		printMessage(VerbosityFullData, "      Permissions:      %d\n", info.FilePermissions)
		printMessage(VerbosityFullData, "      Content Hash:     %s\n", info.Hash)
		printMessage(VerbosityFullData, "      Install Required? %t\n", info.InstallOptional)
		if info.InstallOptional {
			printMessage(VerbosityFullData, "      Install Commands  %s\n", info.Install)
		}
		printMessage(VerbosityFullData, "      Checks Required?  %t\n", info.ChecksRequired)
		if info.ChecksRequired {
			printMessage(VerbosityFullData, "      Check Commands    %s\n", info.Checks)
		}
		printMessage(VerbosityFullData, "      Reload Required?  %t\n", info.ReloadRequired)
		if info.ReloadRequired {
			printMessage(VerbosityFullData, "      Reload Commands   %s\n", info.Reload)
		}
	}

	// Guard against empty return value
	if len(allFileInfo) == 0 {
		err = fmt.Errorf("something went wrong, no files available to load")
		return
	}

	return
}

func getFailTrackerCommit() (commitID string, failures []string, err error) {
	printMessage(VerbosityProgress, "Retrieving commit ID from failtracker file\n")

	// Regex to match commitid line from fail tracker
	failCommitRegEx := regexp.MustCompile(`commitid:([0-9a-fA-F]+)\n`)

	// Read in contents of fail tracker file
	lastFailTrackerBytes, err := os.ReadFile(config.FailTrackerFilePath)
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

	printMessage(VerbosityProgress, "Parsing failtracker lines\n")

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

		printMessage(VerbosityData, "Parsing failure for host %v\n", errorInfo.EndpointName)

		// Add host to override to isolate deployment to just the failed hosts
		hostOverrideArray = append(hostOverrideArray, errorInfo.EndpointName)

		// error if no files
		if len(errorInfo.Files) == 0 {
			err = fmt.Errorf("no files in failtracker line: %s", fail)
			return
		}

		// Add failed files to array (Only create, deleted/symlinks dont get added to failtracker)
		for _, failedFile := range errorInfo.Files {
			printMessage(VerbosityData, "Parsing failure for file %s\n", failedFile)

			// Skip this file if not in override (if override was requested)
			skipFile := checkForOverride(fileOverride, failedFile)
			if skipFile {
				continue
			}

			printMessage(VerbosityData, "Marked host %s - file %s for redeployment\n", errorInfo.EndpointName, failedFile)

			commitFiles[failedFile] = "create"
		}
	}
	// Convert to standard format for override
	hostOverride = strings.Join(hostOverrideArray, ",")

	return
}
