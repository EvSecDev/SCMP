// controller
package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
)

func remoteDeploymentPreparation(host *HostMeta) (err error) {
	printMessage(verbosityProgress, "Host %s: Determining remote OS\n", host.name)

	command := buildUnameKernel()
	unameOutput, err := command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 5)
	if err != nil {
		err = fmt.Errorf("failed to determine OS, cannot deploy: %v", err)
		return
	}
	osName := strings.ToLower(unameOutput)
	if strings.Contains(osName, "bsd") {
		host.osFamily = "bsd"
	} else if strings.Contains(osName, "linux") {
		host.osFamily = "linux"
	} else {
		err = fmt.Errorf("received unknown os type: %s", unameOutput)
		host.osFamily = "unknown"
		return
	}

	printMessage(verbosityProgress, "Host %s: Preparing remote config backup directory\n", host.name)

	// Create transfer directory
	command = buildMkdir(host.transferBufferDir)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, true, host.password, 10)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			err = fmt.Errorf("failed to setup remote transfer directory: %v", err)
			return
		}
		err = nil // reset err so caller doesnt think function failed
	}

	// Create backup directory
	command = buildMkdir(host.backupPath)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, true, host.password, 10)
	if err != nil {
		err = fmt.Errorf("failed to setup remote temporary backup directory: %v", err)
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
			_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
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

			if config.options.wetRunEnabled {
				printMessage(verbosityData, "Host %s:    Wet-run enabled, skipping command", host.name)
				continue
			}

			command := RemoteCommand{command}
			_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 180)
			if err != nil {
				return
			}
		}
	}

	return
}

func runReloadCommands(host HostMeta, reloadCommands []string) (warning string, err error) {
	printMessage(verbosityProgress, "Host %s:   Starting execution of reload commands\n", host.name)

	for index, command := range reloadCommands {
		printMessage(verbosityProgress, "Host %s:     Running reload command '%s'\n", host.name, command)

		if config.options.wetRunEnabled {
			printMessage(verbosityProgress, "Host %s:      Wet-run enabled, skipping command", host.name)
			continue
		}

		rawCmd := RemoteCommand{command}
		_, err = rawCmd.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
		if err != nil {
			if index > 1 {
				warning = "first reload command succeeded, but a later command failed. This might mean the service is currently running a bad configuration."
			}
			err = fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err)
			return
		}
	}

	printMessage(verbosityProgress, "Host %s:   Finished execution of reload commands\n", host.name)
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
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}
	command = buildChmod(targetFilePath, remoteMetadata.permissions)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}
	targetRemoteOwnerGroup := remoteMetadata.owner + ":" + remoteMetadata.group
	command = buildChown(targetFilePath, targetRemoteOwnerGroup)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}

	// Check to make sure restore worked with hash
	command = buildHashCmd(targetFilePath)
	commandOutput, err := command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
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

// Deletes given file from remote and parent directory if empty
func deleteFile(host HostMeta, targetFilePath string) (fileDeleted bool, err error) {
	// Note: technically inefficient; if a file is moved within same directory, this will delete the file and parent dir(maybe)
	//                                then when deploying the moved file, it will recreate folder that was just deleted.

	printMessage(verbosityData, "Host %s:   Deleting file '%s'\n", host.name, targetFilePath)

	if config.options.wetRunEnabled {
		fileDeleted = true // implied that file will always (try) to be deleted
		return
	}

	// Attempt remove file
	command := buildRm(targetFilePath)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 30)
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
		commandOutput, _ := command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)

		// Empty stdout means empty dir
		if commandOutput == "" {
			printMessage(verbosityData, "Host %s:   Removing empty directory '%s'\n", host.name, targetPath)

			// Safe remove directory
			command = buildRmdir(targetPath)
			_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 30)
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
	oldSymLinkExists, statOutput, err := checkRemoteFileDirExistence(host, linkName)
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
			printMessage(verbosityData, "Host %s:    link target is up-to-date\n", host.name)
			return
		}
	}

	if config.options.wetRunEnabled {
		linkModified = true // would have been modified
		return
	}

	// Create symbolic link
	command := buildLink(linkTarget, linkName)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
	if err != nil {
		err = fmt.Errorf("failed to create symbolic link: %v", err)
		return
	}

	linkModified = true
	return
}

func deployFile(host HostMeta, repoFilePath string, localMetadata FileInfo, allFileData map[string][]byte) (fileModified bool, deployedBytes int, remoteMetadata RemoteFileInfo, err error) {
	targetFilePath := localMetadata.targetFilePath

	// Retrieve metadata of remote file if it exists
	remoteMetadata, err = getOldRemoteInfo(host, targetFilePath)
	if err != nil {
		return
	}

	if remoteMetadata.exists {
		printMessage(verbosityData, "Host %s:   Backing up file %s\n", host.name, remoteMetadata.name)

		backupFileName := base64.StdEncoding.EncodeToString([]byte(remoteMetadata.name))
		tmpBackupFilePath := host.backupPath + "/" + backupFileName

		command := buildCp(remoteMetadata.name, tmpBackupFilePath)
		_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
		if err != nil {
			err = fmt.Errorf("error making backup of old config file: %v", err)
			return
		}
	}

	// Get remote vs local status
	contentDiffers, metadataDiffers := checkForDiff(remoteMetadata, localMetadata)

	// Next file if this one does not need updating
	if !contentDiffers && !metadataDiffers {
		printMessage(verbosityProgress, "Host %s:   File '%s' hash matches local and metadata up-to-date... skipping this file\n", host.name, targetFilePath)
		return
	}

	printMessage(verbosityData, "Host %s:   File '%s': remote hash: '%s' - local hash: '%s'\n", host.name, targetFilePath, remoteMetadata.hash, localMetadata.hash)

	if config.options.wetRunEnabled {
		fileModified = true // would have been modified
		return
	}

	// Create file if local is empty
	if localMetadata.fileSize == 0 && !remoteMetadata.exists {
		printMessage(verbosityData, "Host %s:   File '%s' is empty and does not exist on remote, creating\n", host.name, targetFilePath)

		command := buildTouch(localMetadata.targetFilePath)
		_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
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

		if config.options.wetRunEnabled {
			return
		}

		command := buildMkdir(targetDirPath)
		_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
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

	if config.options.wetRunEnabled {
		dirModified = true // would have been modified
		return
	}

	err = modifyMetadata(host, remoteMetadata, dirInfo)
	if err != nil {
		return
	}

	printMessage(verbosityData, "Host %s:   Modified Directory %s\n", host.name, targetDirPath)

	// For metrics
	dirModified = true

	return
}

func checkForReload(endpointName string, deploymentList DeploymentList, totalDeployedReloadFiles map[string]int, reloadIDreadyToReload map[string]bool, repoFilePath string, remoteModified bool) (clearedToReload bool, reloadGroup string) {
	reloadID, fileHasReloadGroup := deploymentList.fileToReloadID[repoFilePath]

	// Nothing to do for this file, early return
	if !fileHasReloadGroup {
		return
	}

	// Increment deployment success for files reload group
	totalDeployedReloadFiles[reloadID]++

	// Any single file modification triggers reload OR user manually requests it
	if remoteModified || config.options.forceEnabled {
		reloadIDreadyToReload[reloadID] = true
	}

	// First, catch not-fully-deployed groups
	if totalDeployedReloadFiles[reloadID] != deploymentList.reloadIDfileCount[reloadID] {
		printMessage(verbosityProgress, "Host %s:   Reload group not fully deployed yet, not running reloads\n", endpointName)
		return
	}

	// Second, catch groups with no remote modifications
	if !reloadIDreadyToReload[reloadID] {
		printMessage(verbosityProgress, "Host %s:   Refusing to run reloads - no remote changes made for reload group\n", endpointName)
		return
	}

	// Third, catch user disabling all reloads
	if config.options.disableReloads && !config.options.forceEnabled {
		printMessage(verbosityProgress, "Host %s:   Force disabling reloads by user request\n", endpointName)
		return
	}

	// Reload commands will be immediately run
	reloadGroup = reloadID
	clearedToReload = true
	return
}

// Cleans up any temporarily items on the remote host
// Errors are non-fatal, but will be printed to the user
func cleanupRemote(host HostMeta) {
	printMessage(verbosityProgress, "Host %s: Cleaning up remote temporary directories\n", host.name)

	// Cleanup temporary files
	command := buildRmAll(host.transferBufferDir, host.backupPath)
	_, err := command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 30)
	if err != nil {
		// Only print error if there was a file to remove in the first place
		if !strings.Contains(err.Error(), "No such file or directory") {
			// Failures to remove the tmp files are not critical, but notify the user regardless
			printMessage(verbosityStandard, " Warning! Failed to cleanup temporary buffer files: %v\n", err)
		}
	}
}
