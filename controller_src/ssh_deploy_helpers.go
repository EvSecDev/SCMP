// controller
package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ###########################################
//      DEPLOYMENT HANDLING FUNCTIONS
// ###########################################

// Run full deployment of a new file to remote host
func createFile(sshClient *ssh.Client, SudoPassword string, targetFilePath string, tmpRemoteFilePath string, fileContents string, fileContentHash string, fileOwnerGroup string, filePermissions int) (err error) {
	// Transfer local file to remote
	err = TransferFile(sshClient, fileContents, targetFilePath, SudoPassword, tmpRemoteFilePath)
	if err != nil {
		err = fmt.Errorf("failed SFTP config file transfer to remote host: %v", err)
		return
	}

	// Check if deployed file is present on disk
	NewFileExists, err := CheckRemoteFileExistence(sshClient, targetFilePath, SudoPassword)
	if err != nil {
		err = fmt.Errorf("error checking deployed file presence on remote host: %v", err)
		return
	}
	// Failed transfer
	if !NewFileExists {
		err = fmt.Errorf("deployed file on remote host is not present after file transfer")
		return
	}

	// Get Hash of new deployed conf file
	command := "sha256sum " + targetFilePath
	CommandOutput, err := RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of deployed file: %v", err)
		return
	}

	// Parse hash command output to get just the hex
	NewRemoteFileHash := SHA256RegEx.FindString(CommandOutput)

	// Compare hashes and restore old conf if they dont match
	if NewRemoteFileHash != fileContentHash {
		err = fmt.Errorf("hash of config file post deployment does not match hash of pre deployment")
		return
	}

	// Ensure owner/group are correct
	command = "chown " + fileOwnerGroup + " " + targetFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during owner/group change: %v", err)
		return
	}

	// Ensure permissions are correct
	command = "chmod " + strconv.Itoa(filePermissions) + " " + targetFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during permissions change: %v", err)
		return
	}

	return
}

// Create a copy of an existing config file into the temporary backup file path (only if targetFilePath exists)
// Also returns the hash of the file before being touched for verification of restore if needed
func backupOldConfig(sshClient *ssh.Client, SudoPassword string, targetFilePath string, tmpBackupPath string) (oldRemoteFileHash string, err error) {
	// Find if target file exists on remote
	oldFileExists, err := CheckRemoteFileExistence(sshClient, targetFilePath, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed checking file presence on remote host: %v", err)
		return
	}

	// If remote file doesn't exist, return early
	if !oldFileExists {
		return
	}

	// Get the SHA256 hash of the remote old conf file
	command := "sha256sum " + targetFilePath
	CommandOutput, err := RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err)
		return
	}

	// Parse hash command output to get just the hex
	oldRemoteFileHash = SHA256RegEx.FindString(CommandOutput)

	// Unique ID for this backup - base64 the target file path - can be later decoded for restoration
	backupFileName := base64.StdEncoding.EncodeToString([]byte(targetFilePath))

	// Absolute path to backup file
	tmpBackupFilePath := tmpBackupPath + "/" + backupFileName

	// Backup old config
	command = "cp -p " + targetFilePath + " " + tmpBackupFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("error making backup of old config file: %v", err)
		return
	}

	return
}

// Moves backup config file into original location after file deployment failure
// Assumes backup file is located in the directory at backupFilePath
// Ensures restoration worked by hashing and comparing to pre-deployment file hash
func restoreOldConfig(sshClient *ssh.Client, targetFilePath string, tmpBackupPath string, oldRemoteFileHash string, SudoPassword string) (err error) {
	// Empty oldRemoteFileHash indicates there was nothing to backup, therefore restore should not occur
	if oldRemoteFileHash == "" {
		return
	}

	// Get the unique id for the backup for the given targetFilePath
	backupFileName := base64.StdEncoding.EncodeToString([]byte(targetFilePath))
	backupFilePath := tmpBackupPath + "/" + backupFileName

	// Move backup conf into place
	command := "mv " + backupFilePath + " " + targetFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}

	// Check to make sure restore worked with hash
	command = "sha256sum " + targetFilePath
	CommandOutput, err := RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err)
		return
	}

	// Parse hash command output to get just the hex
	RemoteFileHash := SHA256RegEx.FindString(CommandOutput)

	// Ensure restoration succeeded
	if oldRemoteFileHash != RemoteFileHash {
		err = fmt.Errorf("restored file hash is different than its original hash")
		return
	}

	return
}

// Checks if file is already present on remote host
func CheckRemoteFileExistence(sshClient *ssh.Client, remoteFilePath string, SudoPassword string) (fileExists bool, err error) {
	command := "ls " + remoteFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		fileExists = false
		if strings.Contains(err.Error(), "No such file or directory") {
			err = nil
			return
		}
		return
	}
	fileExists = true
	return
}

// Transfers file content in variable to remote temp buffer, then moves into remote file path location
// Uses global var for remote temp buffer file path location
func TransferFile(sshClient *ssh.Client, localFileContent string, remoteFilePath string, SudoPassword string, tmpRemoteFilePath string) (err error) {
	var command string

	// Check if remote dir exists, if not create
	dir := filepath.Dir(remoteFilePath)
	command = "ls -d " + dir
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			command = "mkdir -p " + dir
			_, err = RunSSHCommand(sshClient, command, SudoPassword)
			if err != nil {
				err = fmt.Errorf("failed to create directory: %v", err)
				return
			}
		} else {
			err = fmt.Errorf("error checking directory: %v", err)
			return
		}
	}

	// SFTP to temp file
	err = RunSFTP(sshClient, []byte(localFileContent), tmpRemoteFilePath)
	if err != nil {
		return
	}

	// Move file from tmp dir to actual deployment path
	command = "mv " + tmpRemoteFilePath + " " + remoteFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed to move new file into place: %v", err)
		return
	}
	return
}

// Deletes given file from remote and parent directory if empty
func deleteFile(sshClient *ssh.Client, SudoPassword string, targetFilePath string) (err error) {
	// Note: technically inefficient; if a file is moved within same directory, this will delete the file and parent dir(maybe)
	//                                then when deploying the moved file, it will recreate folder that was just deleted.

	// Attempt remove file
	command := "rm " + targetFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		// Ignore specific error if one one isnt there but the other is
		if !strings.Contains(err.Error(), "No such file or directory") {
			err = fmt.Errorf("failed to remove file '%s': %v\n", targetFilePath, err)
			return
		}
	}

	// Danger Zone: Remove empty parent dirs
	targetPath := filepath.Dir(targetFilePath)
	maxLoopCount := 1000 // for safety - sane number to avoid endless dir loops
	for i := 0; i < maxLoopCount; i++ {
		// Check for presence of anything in dir
		command = "ls -A " + targetPath
		CommandOutput, _ := RunSSHCommand(sshClient, command, SudoPassword)

		// Empty stdout means empty dir
		if CommandOutput == "" {
			// Safe remove directory
			command = "rmdir " + targetPath
			_, err = RunSSHCommand(sshClient, command, SudoPassword)
			if err != nil {
				// Error breaks loop
				err = fmt.Errorf("failed to remove empty parent directory '%s' for file '%s': %v\n", targetPath, targetFilePath, err)
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
func createSymLink(sshClient *ssh.Client, SudoPassword string, targetFilePath string, targetFileAction string) (err error) {
	// Check if a file is already there - if so, error
	OldSymLinkExists, err := CheckRemoteFileExistence(sshClient, targetFilePath, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed checking file existence before creating symbolic link: %v", err)
		return
	}
	if OldSymLinkExists {
		err = fmt.Errorf("file already exists where symbolic link is supposed to be created")
		return
	}

	// Extract target path
	tgtActionSplitReady := strings.ReplaceAll(targetFileAction, " to target ", "?")
	targetActionArray := strings.SplitN(tgtActionSplitReady, "?", 2)
	symLinkTarget := targetActionArray[1]

	// Create symbolic link
	command := "ln -s " + symLinkTarget + " " + targetFilePath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed to create symbolic link: %v", err)
		return
	}

	return
}
