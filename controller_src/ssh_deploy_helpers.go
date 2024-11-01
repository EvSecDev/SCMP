// controller
package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ###########################################
//      DEPLOYMENT HANDLING FUNCTIONS
// ###########################################

func restoreOldConfig(client *ssh.Client, targetFilePath string, OldRemoteFileHash string, SHA256RegEx *regexp.Regexp, SudoPassword string, backupConfCreated bool) (err error) {
	var command string
	var CommandOutput string
	oldFilePath := targetFilePath + ".old"

	// Check if there is no backup to restore, return early
	if !backupConfCreated {
		return
	}

	// Move backup conf into place
	command = "mv " + oldFilePath + " " + targetFilePath
	_, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
		return
	}

	// Check to make sure restore worked with hash
	command = "sha256sum " + targetFilePath
	CommandOutput, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err)
		return
	}

	RemoteFileHash := SHA256RegEx.FindString(CommandOutput)

	if OldRemoteFileHash != RemoteFileHash {
		err = fmt.Errorf("restored file hash is different than its original hash")
		return
	}
	return
}

func CheckRemoteFileExistence(client *ssh.Client, remoteFilePath string, SudoPassword string) (fileExists bool, err error) {
	command := "ls " + remoteFilePath
	_, err = RunSSHCommand(client, command, SudoPassword)
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

func TransferFile(client *ssh.Client, localFileContent string, remoteFilePath string, SudoPassword string) (err error) {
	var command string

	// Check if remote dir exists, if not create
	dir := filepath.Dir(remoteFilePath)
	command = "ls -d " + dir
	_, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			command = "mkdir -p " + dir
			_, err = RunSSHCommand(client, command, SudoPassword)
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
	err = RunSFTP(client, []byte(localFileContent))
	if err != nil {
		return
	}

	// Move file from tmp dir to actual deployment path
	command = "mv " + tmpRemoteFilePath + " " + remoteFilePath
	_, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("failed to move new file into place: %v", err)
		return
	}
	return
}
