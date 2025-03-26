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
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while seeding repository files", fmt.Errorf("%v", fatalError), false)
		}
	}()

	printMessage(verbosityStandard, "==== Secure Configuration Management Repository Seeding ====\n")

	// Check local system
	err := localSystemChecks()
	logError("Error in system checks", err, false)

	// Check working dir for git repo
	err = retrieveGitRepoPath()
	logError("Repository Error", err, false)

	if dryRunRequested {
		// Notify user that program is in dry run mode
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

		// Retrieve host secrests (keys,passwords)
		err = retrieveHostSecrets(endpointName)
		logError("Error retrieving host secrets", err, true)

		// Retrieve most current global host config
		hostInfo = config.hostInfo[endpointName]

		// If user requested dry run - print host information and abort connections
		if dryRunRequested {
			printHostInformation(hostInfo)
			continue
		}

		// Connect to the SSH server
		client, err := connectToSSH(hostInfo.endpointName, hostInfo.endpoint, hostInfo.endpointUser, hostInfo.password, hostInfo.privateKey, hostInfo.keyAlgo)
		logError("Failed connect to SSH server", err, false)
		defer client.Close()

		// Run menu for user to select desired files or direct download
		var selectedFiles []string
		if remoteFileOverride == "" {
			selectedFiles, err = runSelection(endpointName, client, hostInfo.password)
			logError("Error retrieving remote file list", err, false)
		} else {
			// Set user choices directly
			selectedFiles = strings.Split(remoteFileOverride, ",")
		}

		// Initialize buffer file (with random byte) - ensures ownership of buffer stays correct when retrieving remote files
		err = SCPUpload(client, []byte{12}, hostInfo.remoteTransferBuffer)
		logError(fmt.Sprintf("Failed to initialize buffer file on remote host %s", endpointName), err, false)

		// Download user file choices to local repo and format
		for _, targetFilePath := range selectedFiles {
			err = retrieveSelectedFile(targetFilePath, endpointName, client, hostInfo.password, hostInfo.remoteTransferBuffer)
			logError("Error seeding repository", err, false)
		}
	}

	printMessage(verbosityStandard, "============================================================\n")
}

// Runs the CLI-based menu that user will use to select which files to download
func runSelection(endpointName string, client *ssh.Client, SudoPassword string) (selectedFiles []string, err error) {
	// Start selection at root of filesystem - '/'
	var directoryState DirectoryState
	directoryState.current = "/"
	directoryState.stack = []string{"/"}

	// Loop until user is done selecting
	for {
		// Get file names and info for the directory
		command := buildLsList(directoryState.current)
		var directoryList string
		directoryList, err = command.SSHexec(client, "", config.disableSudo, SudoPassword, 30)
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
		userRequestedExit, selectedFiles, directoryState = parseUserSelections(userSelections, dirList, directoryState, client, SudoPassword)
		if userRequestedExit {
			// Stop selecting
			break
		}
	}

	return
}

// Downloads user selected files/directories and metadata and writes information to repository
func retrieveSelectedFile(remoteFilePath string, endpointName string, client *ssh.Client, SudoPassword string, tmpRemoteFilePath string) (err error) {
	// Ensure decorators from ls do not get fed into repo
	remoteFilePath = strings.TrimSuffix(remoteFilePath, "*")
	remoteFilePath = strings.TrimSuffix(remoteFilePath, "@")

	// Use target file path and hosts name for repo file location
	localFilePath := filepath.Join(endpointName, strings.ReplaceAll(remoteFilePath, "/", config.osPathSeparator))

	// Retrieve metadata for user selection
	command := buildStat(remoteFilePath)
	statOutput, err := command.SSHexec(client, "root", config.disableSudo, SudoPassword, 10)
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

	// Selected is directory, save just that and return
	if selectionMetadata.fsType == dir {
		err = writeNewDirectoryMetadata(localFilePath, selectionMetadata)
		return
	}

	// Copy desired file to buffer location
	printMessage(verbosityProgress, "  File '%s': Downloading file\n", remoteFilePath)
	command = RemoteCommand{"cp '" + remoteFilePath + "' '" + tmpRemoteFilePath + "'"}
	_, err = command.SSHexec(client, "", config.disableSudo, SudoPassword, 20)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	// Ensure buffer file can be read and then deleted later
	command = buildChmod(tmpRemoteFilePath, 666)
	_, err = command.SSHexec(client, "", config.disableSudo, SudoPassword, 10)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	// Download remote file contents
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
