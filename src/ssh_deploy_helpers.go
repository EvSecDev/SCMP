// controller
package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ###########################################
//      DEPLOYMENT HANDLING FUNCTIONS
// ###########################################

func (metric *DeploymentMetrics) addHostBytes(host string, deployedBytes int) {
	// Lock and write to metric var - increment total transferred bytes
	if deployedBytes > 0 {
		metric.hostBytesMutex.Lock()
		metric.hostBytes[host] += deployedBytes
		metric.hostBytesMutex.Unlock()
	}
}

func (metric *DeploymentMetrics) addFile(host string, files ...string) {
	metric.hostFilesMutex.Lock()
	metric.hostFiles[host] = append(metric.hostFiles[host], files...)
	metric.hostFilesMutex.Unlock()
}

func (metric *DeploymentMetrics) addFileFailure(file string, err error) {
	metric.fileErrMutex.Lock()
	metric.fileErr[file] = fmt.Sprintf("%v", err)
	metric.fileErrMutex.Unlock()
}

func (metric *DeploymentMetrics) addHostFailure(host string, err error) {
	metric.hostErrMutex.Lock()
	metric.hostErr[host] = fmt.Sprintf("%v", err)
	metric.hostErrMutex.Unlock()
}

// Assigns unique groups for file reload commands
// Returns a map keyed on repo file path and its reload group ID
// Returns a total count of files per reload group ID
func groupFilesByReloads(allFileMeta map[string]FileInfo, repoFilePaths []string) (reloadIDtoRepoFile map[string][]string, repoFileToReloadID map[string]string, reloadIDfileCount map[string]int) {
	// Initialize maps
	reloadIDtoRepoFile = make(map[string][]string)
	repoFileToReloadID = make(map[string]string)
	reloadIDfileCount = make(map[string]int)

	// Loop deployment files
	for _, repoFilePath := range repoFilePaths {
		if allFileMeta[repoFilePath].reloadRequired {
			// Create an ID based on the command array to uniquely identify the group that files will belong to
			reloadCommands := fmt.Sprintf("%v", allFileMeta[repoFilePath].reload)
			reloadCmdID := base64.StdEncoding.EncodeToString([]byte(reloadCommands))

			// Add file to reload ID map
			reloadIDtoRepoFile[reloadCmdID] = append(reloadIDtoRepoFile[reloadCmdID], repoFilePath)

			// Add reload ID to file map
			repoFileToReloadID[repoFilePath] = reloadCmdID

			// Increment count of files in reload group
			reloadIDfileCount[reloadCmdID]++
		}
	}
	return
}

// Compares compiled metadata from local and remote file and compares them and reports what is different
// Only compares hashes, owner+group, and permission bits
func checkForDiff(remoteMetadata RemoteFileInfo, localMetadata FileInfo) (contentDiffers bool, metadataDiffers bool) {
	// If user requested force, return early, as deployment will be atomic
	if config.options.forceEnabled {
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

// #################################
//      REMOTE ACTION HANDLING
// #################################

func initBackupDirectory(host HostMeta) (err error) {
	printMessage(verbosityProgress, "Host %s: Preparing remote config backup directory\n", host.name)

	// Create backup directory
	command := buildMkdir(host.backupPath)
	_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 10)
	if err != nil {
		// Since we blindly try to create the directory, ignore errors about it already existing
		if strings.Contains(strings.ToLower(err.Error()), "file exists") {
			err = nil // reset err so caller doesnt think function failed
			return
		}
	}
	return
}

func runCheckCommands(host HostMeta, localMetadata FileInfo) (err error) {
	if localMetadata.checksRequired {
		for _, command := range localMetadata.checks {
			printMessage(verbosityData, "Host %s:   Running check command '%s'\n", host.name, command)

			command := RemoteCommand{command}
			_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
			if err != nil {
				return
			}
		}
	}
	return
}

func runInstallationCommands(host HostMeta, localMetadata FileInfo) (err error) {
	if localMetadata.installOptional && config.options.runInstallCommands {
		for _, command := range localMetadata.install {
			printMessage(verbosityData, "Host %s:   Running install command '%s'\n", host.name, command)

			command := RemoteCommand{command}
			_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 180)
			if err != nil {
				return
			}
		}
	}

	return
}

func runReloadCommands(host HostMeta, reloadCommands []string) (err error) {
	printMessage(verbosityProgress, "Host %s:   Starting execution of reload commands\n", host.name)

	for _, command := range reloadCommands {
		printMessage(verbosityProgress, "Host %s:     Running reload command '%s'\n", host.name, command)

		rawCmd := RemoteCommand{command}
		_, err = rawCmd.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err)
			return
		}
	}

	printMessage(verbosityProgress, "Host %s:   Finished execution of reload commands\n", host.name)
	return
}

// Run full deployment of a new file to remote host
func createRemoteFile(host HostMeta, targetFilePath string, fileContents []byte, fileContentHash string, fileOwnerGroup string, filePermissions int) (err error) {
	// Transfer local file to remote
	err = transferFile(host.sshClient, fileContents, targetFilePath, host.password, host.transferBufferFile, fileOwnerGroup, filePermissions)
	if err != nil {
		err = fmt.Errorf("failed SCP file transfer to remote host: %v", err)
		return
	}

	// Check if deployed file is present on disk
	newFileExists, _, err := checkRemoteFileDirExistence(host.sshClient, targetFilePath, host.password)
	if err != nil {
		err = fmt.Errorf("error checking deployed file presence on remote host: %v", err)
		return
	}
	// Failed transfer
	if !newFileExists {
		err = fmt.Errorf("deployed file on remote host is not present after file transfer")
		return
	}

	// Get Hash of new deployed conf file
	command := buildHashCmd(targetFilePath)
	commandOutput, err := command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of deployed file: %v", err)
		return
	}

	// Parse hash command output to get just the hex
	newRemoteFileHash := SHA256RegEx.FindString(commandOutput)

	// Compare hashes and restore old conf if they dont match
	if newRemoteFileHash != fileContentHash {
		err = fmt.Errorf("hash of config file post deployment does not match hash of pre deployment")
		return
	}

	return
}

// Retrieves metadata about file/dir from ls
func getOldRemoteInfo(host HostMeta, targetPath string) (remoteMetadata RemoteFileInfo, err error) {
	// Find if target file exists on remote
	exists, statOutput, err := checkRemoteFileDirExistence(host.sshClient, targetPath, host.password)
	if err != nil {
		err = fmt.Errorf("failed checking file presence on remote host: %v", err)
		return
	}

	// Return early if not present
	remoteMetadata.exists = exists
	if !exists {
		return
	}

	// Get metadata from the output of the remote stat command
	remoteMetadata, err = extractMetadataFromStat(statOutput)
	if err != nil {
		return
	}
	if remoteMetadata.fsType != fileType && remoteMetadata.fsType != dirType && remoteMetadata.fsType != fileEmptyType {
		err = fmt.Errorf("expected remote path to be file or directory, but got type '%s' instead", remoteMetadata.fsType)
		return
	}

	// Ensure name in metadata is the path we received
	remoteMetadata.name = targetPath

	// Only hash if its a file
	if remoteMetadata.fsType == fileType || remoteMetadata.fsType == fileEmptyType {
		// Get the SHA256 hash of the remote old conf file
		command := buildHashCmd(targetPath)
		var commandOutput string
		commandOutput, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err)
			return
		}

		// Parse hash command output to get just the hex
		remoteMetadata.hash = SHA256RegEx.FindString(commandOutput)
	}

	return
}

// Create a copy of an existing config file into the temporary backup file path (only if targetFilePath exists)
// Also returns the hash of the file before being touched for verification of restore if needed
func backupOldFile(host HostMeta, remoteMetadata RemoteFileInfo) (err error) {
	// If remote file doesn't exist, return early
	if !remoteMetadata.exists {
		return
	}

	printMessage(verbosityData, "Host %s:   Backing up file %s\n", host.name, remoteMetadata.name)

	// Unique ID for this backup - base64 the target file path - can be later decoded for restoration
	backupFileName := base64.StdEncoding.EncodeToString([]byte(remoteMetadata.name))

	// Absolute path to backup file
	tmpBackupFilePath := host.backupPath + "/" + backupFileName

	// Backup old config
	command := buildCp(remoteMetadata.name, tmpBackupFilePath)
	_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("error making backup of old config file: %v", err)
		return
	}

	return
}

// Moves backup config file into original location after file deployment failure
// Assumes backup file is located in the directory at backupFilePath
// Ensures restoration worked by hashing and comparing to pre-deployment file hash
func restoreOldFile(host HostMeta, targetFilePath string, remoteMetadata RemoteFileInfo) (err error) {
	// Empty oldRemoteFileHash indicates there was nothing to backup, therefore restore should not occur
	if remoteMetadata.hash == "" {
		return
	}

	// Get the unique id for the backup for the given targetFilePath
	backupFileName := base64.StdEncoding.EncodeToString([]byte(targetFilePath))
	backupFilePath := host.backupPath + "/" + backupFileName

	// Move backup conf into place
	command := buildMv(backupFilePath, targetFilePath)
	_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}
	command = buildChmod(targetFilePath, remoteMetadata.permissions)
	_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}
	targetRemoteOwnerGroup := remoteMetadata.owner + ":" + remoteMetadata.group
	command = buildChown(targetFilePath, targetRemoteOwnerGroup)
	_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}

	// Check to make sure restore worked with hash
	command = buildHashCmd(targetFilePath)
	commandOutput, err := command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err)
		return
	}

	// Parse hash command output to get just the hex
	remoteFileHash := SHA256RegEx.FindString(commandOutput)

	// Ensure restoration succeeded
	if remoteMetadata.hash != remoteFileHash {
		err = fmt.Errorf("restored file hash is different than its original hash")
		return
	}

	return
}

// Checks if file/dir is already present on remote host
// Also retrieve metadata for file/dir
func checkRemoteFileDirExistence(sshClient *ssh.Client, remotePath string, SudoPassword string) (exists bool, statOutput string, err error) {
	command := buildStat(remotePath)
	statOutput, err = command.SSHexec(sshClient, "root", config.options.disableSudo, SudoPassword, 10)
	if err != nil {
		exists = false
		if strings.Contains(err.Error(), "No such file or directory") {
			err = nil
			return
		}
		return
	}
	exists = true
	return
}

// Transfers file content in variable to remote temp buffer, then moves into remote file path location
// Uses global var for remote temp buffer file path location
func transferFile(sshClient *ssh.Client, localFileContent []byte, remoteFilePath string, SudoPassword string, tmpRemoteFilePath string, fileOwnerGroup string, filePermissions int) (err error) {
	// Check if remote dir exists, if not create
	directoryPath := filepath.Dir(remoteFilePath)
	directoryExists, _, err := checkRemoteFileDirExistence(sshClient, directoryPath, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed checking directory existence: %v", err)
		return
	}
	if !directoryExists {
		command := buildMkdir(directoryPath)
		_, err = command.SSHexec(sshClient, "root", config.options.disableSudo, SudoPassword, 10)
		if err != nil {
			err = fmt.Errorf("failed to create directory: %v", err)
			return
		}
	}

	// SCP to temp file
	err = SCPUpload(sshClient, localFileContent, tmpRemoteFilePath)
	if err != nil {
		return
	}

	// Ensure owner/group are correct
	command := buildChown(tmpRemoteFilePath, fileOwnerGroup)
	_, err = command.SSHexec(sshClient, "root", config.options.disableSudo, SudoPassword, 10)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during owner/group change: %v", err)
		return
	}

	// Ensure permissions are correct
	command = buildChmod(tmpRemoteFilePath, filePermissions)
	_, err = command.SSHexec(sshClient, "root", config.options.disableSudo, SudoPassword, 10)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during permissions change: %v", err)
		return
	}

	// Move file from tmp dir to actual deployment path
	command = buildMv(tmpRemoteFilePath, remoteFilePath)
	_, err = command.SSHexec(sshClient, "root", config.options.disableSudo, SudoPassword, 30)
	if err != nil {
		err = fmt.Errorf("failed to move new file into place: %v", err)
		return
	}
	return
}

// Deletes given file from remote and parent directory if empty
func deleteFile(host HostMeta, targetFilePath string) (fileDeleted bool, err error) {
	// Note: technically inefficient; if a file is moved within same directory, this will delete the file and parent dir(maybe)
	//                                then when deploying the moved file, it will recreate folder that was just deleted.

	printMessage(verbosityData, "Host %s:   Deleting file '%s'\n", host.name, targetFilePath)

	// Attempt remove file
	command := buildRm(targetFilePath)
	_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 30)
	if err != nil {
		// Real errors only if file was present to begin with
		if !strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
			err = fmt.Errorf("failed to remove file '%s': %v", targetFilePath, err)
			return
		}

		// Reset err var
		err = nil
	}

	// Deletion occured, signal as such
	fileDeleted = true

	printMessage(verbosityData, "Host %s:   Checking for empty directories to delete\n", host.name)

	// Danger Zone: Remove empty parent dirs
	targetPath := filepath.Dir(targetFilePath)
	for range maxDirectoryLoopCount {
		// Check for presence of anything in dir
		command = buildLs(targetPath)
		commandOutput, _ := command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 10)

		// Empty stdout means empty dir
		if commandOutput == "" {
			printMessage(verbosityData, "Host %s:   Removing empty directory '%s'\n", host.name, targetPath)

			// Safe remove directory
			command = buildRmdir(targetPath)
			_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 30)
			if err != nil {
				// Error breaks loop
				err = fmt.Errorf("failed to remove empty parent directory '%s' for file '%s': %v", targetPath, targetFilePath, err)
				break
			}

			// Set the next loop dir to be one above
			targetPath = filepath.Dir(targetPath)
			continue
		}

		// Leave loop when a parent dir has something in it
		break
	}

	return
}

// Create symbolic link to specific target file (as present in file action string)
func deploySymLink(host HostMeta, linkName string, linkTarget string) (linkModified bool, err error) {
	printMessage(verbosityData, "Host %s:   Creating symlink %s\n", host.name, linkName)

	// Check if a file is already there
	oldSymLinkExists, statOutput, err := checkRemoteFileDirExistence(host.sshClient, linkName, host.password)
	if err != nil {
		err = fmt.Errorf("failed checking file existence before creating symbolic link: %v", err)
		return
	}

	if oldSymLinkExists {
		// Retrieve existing file information
		var oldMetadata RemoteFileInfo
		oldMetadata, err = extractMetadataFromStat(statOutput)
		if err != nil {
			return
		}

		// Error if the remote file is not a link
		if oldMetadata.fsType != symlinkType {
			err = fmt.Errorf("file already exists where symbolic link is supposed to be created")
			return
		}

		// Nothing to update, return
		if oldMetadata.linkTarget == linkTarget {
			return
		}
	}

	// Create symbolic link
	command := buildLink(linkName, linkTarget)
	_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 10)
	if err != nil {
		err = fmt.Errorf("failed to create symbolic link: %v", err)
		return
	}

	return
}

func deployFile(host HostMeta, repoFilePath string, localMetadata FileInfo, allFileData map[string][]byte) (fileModified bool, deployedBytes int, remoteMetadata RemoteFileInfo, err error) {
	targetFilePath := localMetadata.targetFilePath

	// Retrieve metadata of remote file if it exists
	remoteMetadata, err = getOldRemoteInfo(host, targetFilePath)
	if err != nil {
		return
	}

	// Create a backup config on remote host if remote file already exists
	err = backupOldFile(host, remoteMetadata)
	if err != nil {
		return
	}

	// Get remote vs local status
	contentDiffers, metadataDiffers := checkForDiff(remoteMetadata, localMetadata)

	// Next file if this one does not need updating
	if !contentDiffers && !metadataDiffers {
		printMessage(verbosityProgress, "Host %s:   File '%s' hash matches local and metadata up-to-date... skipping this file\n", host.name, targetFilePath)
		return
	}

	printMessage(verbosityData, "Host %s:   File '%s': remote hash: '%s' - local hash: '%s'\n", host.name, targetFilePath, remoteMetadata.hash, localMetadata.hash)

	// Create file if local is empty
	if localMetadata.fileSize == 0 && !remoteMetadata.exists {
		printMessage(verbosityData, "Host %s:   File '%s' is empty and does not exist on remote, creating\n", host.name, targetFilePath)

		command := buildTouch(localMetadata.targetFilePath)
		_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 10)
		if err != nil {
			err = fmt.Errorf("unable to create empty file: %v", err)
			return
		}
	}

	// Update file content
	if contentDiffers && localMetadata.fileSize > 0 {
		printMessage(verbosityData, "Host %s:   Transferring config '%s' to remote\n", host.name, repoFilePath)

		// Use hash to retrieve file data from map
		hashIndex := localMetadata.hash

		// Transfer config file to remote with correct ownership and permissions
		err = createRemoteFile(host, targetFilePath, allFileData[hashIndex], localMetadata.hash, localMetadata.ownerGroup, localMetadata.permissions)
		if err != nil {
			lerr := restoreOldFile(host, targetFilePath, remoteMetadata)
			if lerr != nil {
				err = fmt.Errorf("%v: restoration failed: %v", err, lerr)
			}
			return
		}

		// Increment byte metric always after a file was uploaded to remote
		deployedBytes += localMetadata.fileSize

		// For metrics
		fileModified = true
	}

	// Update file metadata
	if metadataDiffers {
		printMessage(verbosityData, "Host %s:   Checking if file '%s' needs its metadata updated\n", host.name, targetFilePath)

		err = modifyMetadata(host, remoteMetadata, localMetadata)
		if err != nil {
			lerr := restoreOldFile(host, targetFilePath, remoteMetadata)
			if lerr != nil {
				err = fmt.Errorf("%v: restoration failed: %v", err, lerr)
			}
			return
		}
		printMessage(verbosityData, "Host %s:   File '%s': updated metadata\n", host.name, targetFilePath)

		// For  metrics
		fileModified = true
	}

	return
}

func deployDirectory(host HostMeta, dirInfo FileInfo) (dirModified bool, remoteMetadata RemoteFileInfo, err error) {
	targetDirPath := dirInfo.targetFilePath
	printMessage(verbosityData, "Host %s:   Checking directory '%s'\n", host.name, targetDirPath)

	// Retrieve metadata of remote file if it exists
	remoteMetadata, err = getOldRemoteInfo(host, targetDirPath)
	if err != nil {
		return
	}

	// Create directory if it does not exist
	if !remoteMetadata.exists {
		printMessage(verbosityData, "Host %s:   Directory '%s' is missing, creating...\n", host.name, targetDirPath)

		command := buildMkdir(targetDirPath)
		_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 10)
		if err != nil {
			return
		}

		// Update metadata var with existence
		remoteMetadata.exists = true

		// For metrics
		dirModified = true
	}

	// Check if metadata on directory is up-to-date
	_, metadataDiffers := checkForDiff(remoteMetadata, dirInfo)
	if !metadataDiffers {
		printMessage(verbosityProgress, "Host %s:   Directory '%s' metadata is up-to-date... skipping changes\n", host.name, targetDirPath)
		return
	}

	// Correct metadata of directory
	printMessage(verbosityData, "Host %s:   Updating metdata for directory %s\n", host.name, targetDirPath)

	err = modifyMetadata(host, remoteMetadata, dirInfo)
	if err != nil {
		return
	}

	printMessage(verbosityData, "Host %s:   Modified Directory %s\n", host.name, targetDirPath)

	// For metrics
	dirModified = true

	return
}

// Modifies metadata if supplied remote file/dir metadata does not match supplied metadata
func modifyMetadata(host HostMeta, remoteMetadata RemoteFileInfo, localMetadata FileInfo) (err error) {
	// Change permissions if different
	if remoteMetadata.permissions != localMetadata.permissions {
		printMessage(verbosityFullData, "Host %s:    File '%s': changing permissions\n", host.name, localMetadata.targetFilePath)

		command := buildChmod(localMetadata.targetFilePath, localMetadata.permissions)
		_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 10)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during permissions change: %v", err)
			return
		}
	}

	// Change ownership if different
	remoteOwnerGroup := remoteMetadata.owner + ":" + remoteMetadata.group
	if remoteOwnerGroup != localMetadata.ownerGroup {
		printMessage(verbosityFullData, "Host %s:    File '%s': changing ownership\n", host.name, localMetadata.targetFilePath)

		command := buildChown(localMetadata.targetFilePath, localMetadata.ownerGroup)
		_, err = command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 10)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during owner/group change: %v", err)
			return
		}
	}

	return
}

// Determines if reload should occur for files with reloads
func checkForReload(endpointName string, totalDeployedReloadFiles map[string]int, reloadIDfileCount map[string]int, reloadID string, remoteModified bool) (clearedToReload bool) {
	// Increment deployment success for files reload group (if it has one)
	totalDeployedReloadFiles[reloadID]++

	// Internal bool to track if we are going to run reloads are not - default to true if remote modification happened
	var executeReloads bool
	if remoteModified {
		executeReloads = true
	} else {
		printMessage(verbosityProgress, "Host %s:   Refusing to run reloads - no remote changes made for reload group\n", endpointName)
	}

	// User requested force disable of reloads
	if config.options.disableReloads {
		printMessage(verbosityProgress, "Host %s:   Force disabling reloads by user request\n", endpointName)
		executeReloads = false
	}

	// User requested force enable reloads
	if config.options.forceEnabled {
		printMessage(verbosityProgress, "Host %s:   Force enabling reloads by user request\n", endpointName)
		executeReloads = true
	}

	// Run reloads when all files in reload group deployed without error
	if executeReloads && totalDeployedReloadFiles[reloadID] == reloadIDfileCount[reloadID] {
		clearedToReload = true
	}

	return
}

// Cleans up any temporarily items on the remote host
// Errors are non-fatal, but will be printed to the user
func cleanupRemote(host HostMeta) {
	printMessage(verbosityProgress, "Host %s: Cleaning up remote temporary directories\n", host.name)

	// Cleanup temporary files
	command := buildRmAll(host.transferBufferFile, host.backupPath)
	_, err := command.SSHexec(host.sshClient, "root", config.options.disableSudo, host.password, 30)
	if err != nil {
		// Only print error if there was a file to remove in the first place
		if !strings.Contains(err.Error(), "No such file or directory") {
			// Failures to remove the tmp files are not critical, but notify the user regardless
			printMessage(verbosityStandard, " Warning! Failed to cleanup temporary buffer files: %v\n", err)
		}
	}
}
