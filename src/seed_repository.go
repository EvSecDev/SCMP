// controller
package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ###################################
//  SEED REPO FILES FUNCTIONS
// ###################################

// Entry point for user to select remote files to download and format into local repository
func seedRepositoryFiles(hostOverride string, remoteFileOverride string) {
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while seeding repository files", fmt.Errorf("%v", fatalError), false)
		}
	}()

	err := localSystemChecks()
	logError("Error in system checks", err, false)

	err = retrieveGitRepoPath()
	logError("Repository Error", err, false)

	if dryRunRequested {
		printMessage(verbosityStandard, "Requested dry-run, aborting deployment\n")
		if globalVerbosityLevel < 2 {
			// If not running with higher verbosity, no need to collect deployment information
			return
		}
		printMessage(verbosityProgress, "Outputting information collected for deployment:\n")
	}

	// Loop hosts chosen by user and prepare relevant host information for deployment
	for endpointName, hostInfo := range config.hostInfo {
		skipHost := checkForOverride(hostOverride, endpointName)
		if skipHost {
			printMessage(verbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		config.hostInfo[endpointName], err = retrieveHostSecrets(config.hostInfo[endpointName])
		logError("Error retrieving host secrets", err, true)

		proxyName := config.hostInfo[endpointName].proxy
		if proxyName != "" {
			config.hostInfo[proxyName], err = retrieveHostSecrets(config.hostInfo[proxyName])
			logError("Error retrieving proxy secrets", err, true)
		}

		// Retrieve most current global host config
		hostInfo = config.hostInfo[endpointName]
		proxyInfo := config.hostInfo[config.hostInfo[endpointName].proxy]

		// If user requested dry run - print host information and abort connections
		if dryRunRequested {
			printHostInformation(hostInfo)
			continue
		}

		client, proxyClient, err := connectToSSH(hostInfo, proxyInfo)
		logError("Failed connect to SSH server", err, false)
		if proxyClient != nil {
			defer proxyClient.Close()
		}
		defer client.Close()

		var selectedFiles []string
		if remoteFileOverride == "" {
			selectedFiles, err = interactiveSelection(endpointName, client, hostInfo.password)
			logError("Error retrieving remote file list", err, false)
		} else {
			// Set user choices directly
			selectedFiles = strings.Split(remoteFileOverride, ",")
		}

		// Initialize buffer file (with random byte) - ensures ownership of buffer stays correct when retrieving remote files
		err = SCPUpload(client, []byte{12}, hostInfo.remoteTransferBuffer)
		logError(fmt.Sprintf("Failed to initialize buffer file on remote host %s", endpointName), err, false)

		for _, targetFilePath := range selectedFiles {
			err = handleSelectedFile(targetFilePath, endpointName, client, hostInfo.password, hostInfo.remoteTransferBuffer)
			logError("Error seeding repository", err, false)
		}
	}
}

// Runs the CLI-based menu that user will use to select which files to download
func interactiveSelection(endpointName string, client *ssh.Client, SudoPassword string) (selectedFiles []string, err error) {
	// Start selection at root of filesystem - '/'
	var directoryState DirectoryState
	directoryState.current = "/"
	directoryState.stack = []string{"/"}

	// Loop until user is done selecting
	for {
		// Get file names and info for the directory
		command := buildLsList(directoryState.current)
		var directoryList string
		directoryList, err = command.SSHexec(client, "", config.options.disableSudo, SudoPassword, 30)
		if err != nil {
			// All errors except permission denied exits selection menu
			if !strings.Contains(err.Error(), "Permission denied") {
				return
			}

			// Exit menu if it failed reading the first directory after ssh connection (i.e. "/")
			if directoryState.current == "/" {
				err = fmt.Errorf("permission denied when reading '/'")
				return
			}

			// Show progress to user
			printMessage(verbosityStandard, "Error: unable to read '%s'\n", directoryState.current)

			// Set next loop directory to parent directory
			directoryState.current = directoryState.stack[len(directoryState.stack)-2]

			// Remove current directory from the stack
			directoryState.stack = directoryState.stack[:len(directoryState.stack)-1]
			continue
		}

		// Extract info from ls directory listing
		dirList, maxNameLenght := parseDirEntries(directoryList)

		// Show Menu - Print the directory contents in columns
		userSelections := dirListMenu(endpointName, maxNameLenght, dirList, directoryState.current)

		// Parse users selections
		var userRequestedExit bool
		var dirSelectedFiles []string
		userRequestedExit, dirSelectedFiles, directoryState = parseUserSelections(userSelections, dirList, directoryState, client, SudoPassword)
		selectedFiles = append(selectedFiles, dirSelectedFiles...)
		if userRequestedExit {
			// Stop selecting
			break
		}
	}

	return
}

// Downloads user selected files/directories and metadata and writes information to repository
func handleSelectedFile(remoteFilePath string, endpointName string, client *ssh.Client, SudoPassword string, tmpRemoteFilePath string) (err error) {
	// Ensure decorators from ls do not get fed into repo
	remoteFilePath = strings.TrimSuffix(remoteFilePath, "*")
	remoteFilePath = strings.TrimSuffix(remoteFilePath, "@")

	// Use target file path and hosts name for repo file location
	localFilePath := filepath.Join(endpointName, strings.ReplaceAll(remoteFilePath, "/", config.osPathSeparator))

	command := buildStat(remoteFilePath)
	statOutput, err := command.SSHexec(client, "root", config.options.disableSudo, SudoPassword, 10)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}
	printMessage(verbosityProgress, "  Selection '%s': Parsing metadata...\n", remoteFilePath)
	selectionMetadata, err := extractMetadataFromStat(statOutput)
	if err != nil {
		err = fmt.Errorf("failed parsing stat output: %v", err)
		return
	}

	if selectionMetadata.fsType == dirType {
		err = writeNewDirectoryMetadata(localFilePath, selectionMetadata)
		return
	}

	if selectionMetadata.fsType == symlinkType {
		err = writeSymbolicLinkToRepo(localFilePath, selectionMetadata)
		return
	}

	printMessage(verbosityProgress, "  File '%s': Downloading file\n", remoteFilePath)

	command = RemoteCommand{"cp '" + remoteFilePath + "' '" + tmpRemoteFilePath + "'"}
	_, err = command.SSHexec(client, "", config.options.disableSudo, SudoPassword, 20)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	command = buildChmod(tmpRemoteFilePath, 666)
	_, err = command.SSHexec(client, "", config.options.disableSudo, SudoPassword, 10)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	fileContents, err := SCPDownload(client, tmpRemoteFilePath)
	if err != nil {
		return
	}

	// Retrieve and write to repo parent directory permissions that are unique
	err = writeNewDirectoryTreeMetadata(endpointName, remoteFilePath, client, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed to walk directory tree metadata for file %s: %v", remoteFilePath, err)
		return
	}

	printMessage(verbosityProgress, "  File '%s': Parsing metadata information\n", remoteFilePath)

	// Metadata header
	var fileMetadata MetaHeader
	fileMetadata.TargetFileOwnerGroup = selectionMetadata.owner + ":" + selectionMetadata.group
	fileMetadata.TargetFilePermissions = selectionMetadata.permissions

	// Get reload commands from user
	fileMetadata.ReloadCommands, err = handleNewReloadCommands(remoteFilePath, localFilePath)
	if err != nil {
		return
	}

	// Check for binary files and handle them separately from text files
	fileMetadata.ExternalContentLocation, err = handleArtifactFiles(&localFilePath, &fileContents)
	if err != nil {
		return
	}

	// Write metadata and content to repository file
	err = writeNewFileFull(localFilePath, fileMetadata, &fileContents)
	if err != nil {
		err = fmt.Errorf("failed to add file to repository: %v", err)
		return
	}

	return
}
