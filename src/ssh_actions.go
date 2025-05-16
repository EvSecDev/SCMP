// controller
package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
)

// Transfers file into place with correct permissions and ownership
func createRemoteFile(host HostMeta, targetFilePath string, fileContents []byte, fileContentHash string, fileOwnerGroup string, filePermissions int) (err error) {
	// Check if remote dir exists, if not create
	directoryPath := filepath.Dir(targetFilePath)
	directoryExists, _, err := checkRemoteFileDirExistence(host, directoryPath)
	if err != nil {
		err = fmt.Errorf("failed checking directory existence: %v", err)
		return
	}
	if !directoryExists {
		command := buildMkdir(directoryPath)
		_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
		if err != nil {
			err = fmt.Errorf("failed to create directory: %v", err)
			return
		}
	}

	// Unique file name for buffer file
	tempFileName := base64.StdEncoding.EncodeToString([]byte(targetFilePath))
	bufferFilePath := host.transferBufferDir + "/" + tempFileName

	// SCP to temp file
	err = SCPUpload(host.sshClient, fileContents, bufferFilePath)
	if err != nil {
		return
	}

	// Ensure owner/group are correct
	command := buildChown(bufferFilePath, fileOwnerGroup)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during owner/group change: %v", err)
		return
	}

	// Ensure permissions are correct
	command = buildChmod(bufferFilePath, filePermissions)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during permissions change: %v", err)
		return
	}

	// Move file from tmp dir to actual deployment path
	command = buildMv(bufferFilePath, targetFilePath)
	_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 30)
	if err != nil {
		err = fmt.Errorf("failed to move new file into place: %v", err)
		return
	}

	// Check if deployed file is present on disk
	newFileExists, _, err := checkRemoteFileDirExistence(host, targetFilePath)
	if err != nil {
		err = fmt.Errorf("error checking deployed file presence on remote host: %v", err)
		return
	}
	if !newFileExists {
		err = fmt.Errorf("deployed file on remote host is not present after file transfer")
		return
	}

	// Ensure final file is intact
	command = buildHashCmd(targetFilePath)
	commandOutput, err := command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of deployed file: %v", err)
		return
	}

	newRemoteFileHash := SHA256RegEx.FindString(commandOutput)
	if newRemoteFileHash != fileContentHash {
		err = fmt.Errorf("hash of config file post deployment does not match hash of pre deployment")
		return
	}

	return
}

// Retrieves metadata about file/dir from stat
func getOldRemoteInfo(host HostMeta, targetPath string) (remoteMetadata RemoteFileInfo, err error) {
	// Find if target file exists on remote
	exists, statOutput, err := checkRemoteFileDirExistence(host, targetPath)
	if err != nil {
		err = fmt.Errorf("failed checking file presence on remote host: %v", err)
		return
	}

	// Return early if not present
	remoteMetadata.exists = exists
	if !exists {
		printMessage(verbosityData, "Host %s:    File %s: remote does not exist, not extracting metadata\n", host.name, targetPath)
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
		commandOutput, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 90)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err)
			return
		}

		// Parse hash command output to get just the hex
		remoteMetadata.hash = SHA256RegEx.FindString(commandOutput)
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

// Checks if file/dir is already present on remote host
// Also retrieve metadata for file/dir
func checkRemoteFileDirExistence(host HostMeta, remotePath string) (exists bool, statOutput string, err error) {
	var command RemoteCommand
	if host.osFamily == "bsd" {
		command = buildBSDStat(remotePath)
	} else if host.osFamily == "linux" {
		command = buildStat(remotePath)
	}

	statOutput, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
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

// Modifies metadata if supplied remote file/dir metadata does not match supplied metadata
func modifyMetadata(host HostMeta, remoteMetadata RemoteFileInfo, localMetadata FileInfo) (err error) {
	// Change permissions if different
	if remoteMetadata.permissions != localMetadata.permissions {
		printMessage(verbosityFullData, "Host %s:    File '%s': changing permissions\n", host.name, localMetadata.targetFilePath)

		command := buildChmod(localMetadata.targetFilePath, localMetadata.permissions)
		_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
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
		_, err = command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 10)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during owner/group change: %v", err)
			return
		}
	}

	return
}
