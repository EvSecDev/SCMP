// controller
package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

// ###################################
//      HOST DEPLOYMENT HANDLING
// ###################################

// SSH's into a remote host to deploy files and run reload commands
func deployConfigs(wg *sync.WaitGroup, semaphore chan struct{}, endpointName string, commitFilePaths []string, endpointInfo EndpointInfo, commitFileInfo map[string]CommitFileInfo) {
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

	// Separate files with and without reload commands
	commitFileByCommand := make(map[string][]string)
	var commitFilesNoReload []string
	for _, commitFilePath := range commitFilePaths {
		// New files with reload commands
		if commitFileInfo[commitFilePath].ReloadRequired && len(commitFileInfo[commitFilePath].Reload) > 0 {
			// Create an ID based on the command array to uniquely identify the group that files will belong to
			// The data represented in cmdArrayID does not matter and it is not used outside this loop, it only needs to be unique
			reloadCommands := fmt.Sprintf("%v", commitFileInfo[commitFilePath].Reload)
			cmdArrayID := base64.StdEncoding.EncodeToString([]byte(reloadCommands))

			// Add file to array based on its unique set of reload commands
			commitFileByCommand[cmdArrayID] = append(commitFileByCommand[cmdArrayID], commitFilePath)
		} else {
			// All other files - no reloads
			commitFilesNoReload = append(commitFilesNoReload, commitFilePath)
		}
	}

	// Connect to the SSH server
	sshClient, err := connectToSSH(endpointInfo.Endpoint, endpointInfo.EndpointUser, endpointInfo.PrivateKey, endpointInfo.KeyAlgo)
	if err != nil {
		recordDeploymentFailure(endpointName, commitFilePaths, 0, fmt.Errorf("failed connect to SSH server %v", err))
		return
	}
	defer sshClient.Close()

	// Get sudo password from info map
	SudoPassword := endpointInfo.SudoPassword

	// Get this hosts remote transfer buffer file path
	tmpRemoteFilePath := endpointInfo.RemoteTransferBuffer
	tmpBackupPath := endpointInfo.RemoteBackupDir

	// Need local metric in order to determine what number of configs for this specific host succeeded (to increment global host metric counter)
	var postDeployedConfigsLocal int

	// Create backup directory
	command := "mkdir " + tmpBackupPath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		// Since we blindly try to create the directory, ignore errors about it already existing
		if !strings.Contains(err.Error(), "File exists") {
			recordDeploymentFailure(endpointName, commitFilePaths, 0, fmt.Errorf("failed SSH Command on host during creation of backup directory: %v", err))
			return
		}
	}

	// Loop over command groups and deploy files that need reload commands
	for _, commitFilePaths := range commitFileByCommand {
		// Deploy all files for this specific reload command set
		var targetFilePaths []string
		backupFileHashes := make(map[string]string)
		for index, commitFilePath := range commitFilePaths {
			// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
			commitIndex := index + 1

			// Split repository host dir and config file path for obtaining the absolute target file path
			_, targetFilePath := separateHostDirFromPath(commitFilePath)
			// Reminder:
			// targetFilePath   should be the file path as expected on the remote system
			// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)

			// Create a backup config on remote host if remote file already exists
			oldRemoteFileHash, err := backupOldConfig(sshClient, SudoPassword, targetFilePath, tmpBackupPath)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, err)
				continue
			}

			// Compare hashes and skip to next file deployment if remote is same as local
			if oldRemoteFileHash == commitFileInfo[commitFilePath].Hash {
				fmt.Printf("\rHost '%s': file '%s' hash matches local... skipping this file\n", endpointName, targetFilePath)
				continue
			}

			// Transfer config file to remote with correct ownership and permissions
			err = createFile(sshClient, SudoPassword, targetFilePath, tmpRemoteFilePath, commitFileInfo[commitFilePath].Data, commitFileInfo[commitFilePath].Hash, commitFileInfo[commitFilePath].FileOwnerGroup, commitFileInfo[commitFilePath].FilePermissions)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, err)
				err = restoreOldConfig(sshClient, targetFilePath, tmpBackupPath, oldRemoteFileHash, SudoPassword)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
				}
				continue
			}

			// Record completed target file paths in case reload fails and restoration needs to occur
			targetFilePaths = append(targetFilePaths, targetFilePath)

			// Record backup file hashes to map in case reload fails and restoration needs to occur
			backupFileHashes[targetFilePath] = oldRemoteFileHash
		}

		// Since all the files use the same command array, just pick out one file to get the reload command array from
		commandReloadArray := commitFileInfo[commitFilePaths[0]].Reload

		// Run all the commands required by this config file group
		var ReloadFailed bool
		for _, command := range commandReloadArray {
			_, err = RunSSHCommand(sshClient, command, SudoPassword)
			if err != nil {
				// Record this failed command - first failure always stops reloads
				// Record failures using the arry of all files for this command group and signal to record all the files using index "0"
				recordDeploymentFailure(endpointName, targetFilePaths, 0, fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err))
				ReloadFailed = true
				break
			}
		}
		// Restore configs and skip to next reload group if reload failed
		if ReloadFailed {
			// Restore all config files for this group
			for index, targetFilePath := range targetFilePaths {
				// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
				commitIndex := index + 1

				err = restoreOldConfig(sshClient, targetFilePath, tmpBackupPath, backupFileHashes[targetFilePath], SudoPassword)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
				}
			}
			continue
		}

		// Increment local metric for configs by number of files under this command group (deployment by command group is all files or none)
		postDeployedConfigsLocal += len(commitFilePaths)
	}

	// Loop through target files and deploy (non-reload required configs)
	for index, commitFilePath := range commitFilesNoReload {
		// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
		commitIndex := index + 1

		// Split repository host dir and config file path for obtaining the absolute target file path
		_, targetFilePath := separateHostDirFromPath(commitFilePath)
		// Reminder:
		// targetFilePath   should be the file path as expected on the remote system
		// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)

		// What to do - Create/Delete/symlink the config
		targetFileAction := commitFileInfo[commitFilePath].Action

		// Delete file on remote if deleted in repo
		if targetFileAction == "delete" {
			err = deleteFile(sshClient, SudoPassword, targetFilePath)
			if err != nil {
				// Only record errors where removal of the specific file failed
				if strings.Contains(err.Error(), "failed to remove file") {
					recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
				}

				// Other errors (removing empty parent dirs) are not recorded
				fmt.Printf("Warning: Host %s: %v\n", endpointName, err)
			}

			// Done deleting (or recording error) - Next deployment file
			continue
		}

		// Create symbolic link if requested
		if strings.Contains(targetFileAction, "symlinkcreate") {
			err = createSymLink(sshClient, SudoPassword, targetFilePath, targetFileAction)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
			}

			// Done creating link (or recording error) - Next deployment file
			continue
		}

		// Create a backup config on remote host if remote file already exists
		oldRemoteFileHash, err := backupOldConfig(sshClient, SudoPassword, targetFilePath, tmpBackupPath)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
			continue
		}

		// Compare hashes and skip to next file deployment if remote is same as local
		if oldRemoteFileHash == commitFileInfo[commitFilePath].Hash {
			fmt.Printf("\rHost '%s': file '%s' hash matches local... skipping this file\n", endpointName, targetFilePath)
			continue
		}

		// Transfer config file to remote with correct ownership and permissions
		err = createFile(sshClient, SudoPassword, targetFilePath, tmpRemoteFilePath, commitFileInfo[commitFilePath].Data, commitFileInfo[commitFilePath].Hash, commitFileInfo[commitFilePath].FileOwnerGroup, commitFileInfo[commitFilePath].FilePermissions)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
			err = restoreOldConfig(sshClient, targetFilePath, tmpBackupPath, oldRemoteFileHash, SudoPassword)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		// Increment local metric for config
		postDeployedConfigsLocal++
	}

	// Cleanup temporary files
	command = "rm -r " + tmpRemoteFilePath + " " + tmpBackupPath
	_, err = RunSSHCommand(sshClient, command, SudoPassword)
	if err != nil {
		// Only print error if there was a file to remove in the first place
		if !strings.Contains(err.Error(), "No such file or directory") {
			// Failures to remove the tmp files are not critical, but notify the user regardless
			fmt.Printf(" Warning! Failed to cleanup temporary buffer files: %v\n", err)
		}
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
