// controller
package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// ###################################
//      HOST DEPLOYMENT HANDLING
// ###################################

func deployConfigs(wg *sync.WaitGroup, semaphore chan struct{}, endpointName string, commitFilePaths []string, endpointSocket string, endpointUser string, commitFileData map[string]string, commitFileMetadata map[string]map[string]interface{}, commitFileDataHashes map[string]string, commitFileActions map[string]string, PrivateKey ssh.Signer, SudoPassword string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError(fmt.Sprintf("Controller panic during deployment to host '%s'", endpointName), fmt.Errorf("%v", fatalError), false)
		}
	}()

	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	// Connect to the SSH server
	client, err := connectToSSH(endpointSocket, endpointUser, PrivateKey)
	if err != nil {
		recordDeploymentFailure(endpointName, commitFilePaths, 0, fmt.Errorf("failed connect to SSH server %v", err))
		return
	}
	defer client.Close()

	// Loop through target files and deploy
	var postDeployedConfigsLocal int
	backupConfCreated := false
	for index, commitFilePath := range commitFilePaths {
		// Move index up one to differentiate between first array item and entire host failure
		commitIndex := index + 1

		// Split repository host dir and config file path for obtaining the absolute target file path
		commitSplit := strings.SplitN(commitFilePath, "/", 2)
		commitPath := commitSplit[1]
		targetFilePath := "/" + commitPath
		// Reminder:
		// targetFilePath   should be the file path as expected on the remote system
		// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)

		var command string
		var CommandOutput string
		targetFileAction := commitFileActions[commitFilePath]

		// If git file was deleted, attempt to delete file any empty folders above - failures here should not stop deployment to this host
		// Note: technically inefficient; if a file is moved within same directory, this will delete the file and parent dir(maybe)
		//                                then when deploying the moved file, it will recreate folder that was just deleted.
		if targetFileAction == "delete" {
			// Attempt remove file and any backup for that file
			command = "rm " + targetFilePath + " " + targetFilePath + ".old"
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				// Ignore specific error if one one isnt there but the other is
				if !strings.Contains(err.Error(), "No such file or directory") {
					fmt.Printf("Warning: Host %s: failed to remove file '%s': %v\n", endpointName, targetFilePath, err)
				}
			}
			// Danger Zone: Remove empty parent dirs
			targetPath := filepath.Dir(targetFilePath)
			maxLoopCount := 64001 // for safety - max ext4 sub dirs (but its sane enough for other fs which have super high limits)
			for i := 0; i < maxLoopCount; i++ {
				// Check for presence of anything in dir
				command = "ls -A " + targetPath
				CommandOutput, _ = RunSSHCommand(client, command, SudoPassword)

				// Empty stdout means empty dir
				if CommandOutput == "" {
					// Safe remove directory
					command = "rmdir " + targetPath
					_, err = RunSSHCommand(client, command, SudoPassword)
					if err != nil {
						// Error breaks loop
						fmt.Printf("Warning: Host %s: failed to remove empty parent directory '%s' for file '%s': %v\n", endpointName, targetPath, targetFilePath, err)
						break
					}

					// Set the next loop dir to be one above
					targetPath = filepath.Dir(targetPath)
					continue
				}

				// Leave loop when a parent dir has something in it
				break
			}

			// Next target file to deploy for this host
			continue
		}

		// Create symbolic link if requested
		if strings.Contains(targetFileAction, "symlinkcreate") {
			// Check if a file is already there - if so, error
			OldSymLinkExists, err := CheckRemoteFileExistence(client, targetFilePath, SudoPassword)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("error checking file existence before creating symbolic link: %v", err))
				continue
			}
			if OldSymLinkExists {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("error file already exists where symbolic link is supposed to be created"))
				continue
			}

			// Extract target path
			tgtActionSplitReady := strings.ReplaceAll(targetFileAction, " to target ", "?")
			targetActionArray := strings.SplitN(tgtActionSplitReady, "?", 2)
			symLinkTarget := targetActionArray[1]

			// Create symbolic link
			command = "ln -s " + symLinkTarget + " " + targetFilePath
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("error creating symbolic link: %v", err))
				continue
			}
			continue
		}

		// Safety blocker for files that didn't get tagged 'create'
		if targetFileAction != "create" {
			// Skip non-create files
			continue
		}

		// Parse out Metadata Map into vars
		TargetFileOwnerGroup := commitFileMetadata[commitFilePath]["FileOwnerGroup"].(string)
		TargetFilePermissions := commitFileMetadata[commitFilePath]["FilePermissions"].(int)
		ReloadRequired := commitFileMetadata[commitFilePath]["ReloadRequired"].(bool)
		ReloadCommands := commitFileMetadata[commitFilePath]["Reload"].([]string)

		// Find if target file exists on remote
		oldFileExists, err := CheckRemoteFileExistence(client, targetFilePath, SudoPassword)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("error checking file presence on remote host: %v", err))
			continue
		}

		// If file exists, Hash remote file
		var oldRemoteFileHash string
		if oldFileExists {
			// Get the SHA256 hash of the remote old conf file
			command = "sha256sum " + targetFilePath
			CommandOutput, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err))
				continue
			}

			// Parse hash command output to get just the hex
			oldRemoteFileHash = SHA256RegEx.FindString(CommandOutput)

			// Compare hashes and go to next file deployment if remote is same as local
			if oldRemoteFileHash == commitFileDataHashes[commitFilePath] {
				fmt.Printf("\rHost '%s': file '%s' hash matches local... skipping this file\n", endpointName, targetFilePath)
				continue
			}

			// Backup old config
			command = "cp -p " + targetFilePath + " " + targetFilePath + ".old"
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("error making backup of old config file: %v", err))
				continue
			}

			// Ensure old restore only happens if a backup was created
			backupConfCreated = true
		}

		// Transfer local file to remote
		err = TransferFile(client, commitFileData[commitFilePath], targetFilePath, SudoPassword)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SFTP config file transfer to remote host: %v", err))
			err = restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed Old Config Restoration: %v", err))
			}
			continue
		}

		// Check if deployed file is present on disk
		NewFileExists, err := CheckRemoteFileExistence(client, targetFilePath, SudoPassword)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("error checking deployed file presence on remote host: %v", err))
			err = restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed Old Config Restoration: %v", err))
			}
			continue
		}
		if !NewFileExists {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("deployed file on remote host is not present after file transfer"))
			err = restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		// Get Hash of new deployed conf file
		command = "sha256sum " + targetFilePath
		CommandOutput, err = RunSSHCommand(client, command, SudoPassword)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during hash of deployed file: %v", err))
			err := restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		// Parse hash command output to get just the hex
		NewRemoteFileHash := SHA256RegEx.FindString(CommandOutput)

		// Compare hashes and restore old conf if they dont match
		if NewRemoteFileHash != commitFileDataHashes[commitFilePath] {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("error: hash of config file post deployment does not match hash of pre deployment"))
			err = restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		command = "chown " + TargetFileOwnerGroup + " " + targetFilePath
		_, err = RunSSHCommand(client, command, SudoPassword)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during owner/group change: %v", err))
			err = restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		command = "chmod " + strconv.Itoa(TargetFilePermissions) + " " + targetFilePath
		_, err = RunSSHCommand(client, command, SudoPassword)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during permissions change: %v", err))
			err = restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		// No reload required, early loop continue
		if !ReloadRequired {
			// Increment counter of configs by 1
			postDeployedConfigsLocal++
			continue
		}

		// Run all the commands required by new config file
		var ReloadFailed bool
		for _, command := range ReloadCommands {
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err))
				err = restoreOldConfig(client, targetFilePath, oldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
				}
				ReloadFailed = true
				break
			}
		}
		if ReloadFailed {
			continue
		}

		// Increment local metric for config
		postDeployedConfigsLocal++
	}

	// Lock and write to metric var - increment success configs by local file counter
	MetricCountMutex.Lock()
	postDeployedConfigs += postDeployedConfigsLocal
	MetricCountMutex.Unlock()

	// Lock and write to metric var - increment success hosts by 1 (only if any config was deployed)
	if postDeployedConfigsLocal > 0 {
		MetricCountMutex.Lock()
		postDeploymentHosts++
		MetricCountMutex.Unlock()
	}
}
