// controller
package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// ###################################
//      HOST DEPLOYMENT HANDLING
// ###################################

// SSH's into a remote host to deploy files and run reload commands
func deployConfigs(wg *sync.WaitGroup, semaphore chan struct{}, endpointInfo EndpointInfo, allFileInfo map[string]FileInfo, allFileData map[string]string) {
	// Grab endpoint name
	endpointName := endpointInfo.EndpointName

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

	// Grab files for this host
	commitFilePaths := endpointInfo.DeploymentFiles

	printMessage(VerbosityProgress, "Host %s: Grouping config files by reload commands\n", endpointName)

	// Separate files with and without reload commands
	commitFileByCommand := make(map[string][]string)
	var commitFilesNoReload []string
	for _, commitFilePath := range commitFilePaths {
		// New files with reload commands
		if allFileInfo[commitFilePath].ReloadRequired {
			// Create an ID based on the command array to uniquely identify the group that files will belong to
			// The data represented in cmdArrayID does not matter and it is not used outside this loop, it only needs to be unique
			reloadCommands := fmt.Sprintf("%v", allFileInfo[commitFilePath].Reload)
			cmdArrayID := base64.StdEncoding.EncodeToString([]byte(reloadCommands))

			// Add file to array based on its unique set of reload commands
			commitFileByCommand[cmdArrayID] = append(commitFileByCommand[cmdArrayID], commitFilePath)
		} else {
			// All other files - no reloads
			commitFilesNoReload = append(commitFilesNoReload, commitFilePath)
		}
	}

	printMessage(VerbosityProgress, "Host %s: Connecting to SSH server\n", endpointName)

	// Get sudo password from info map
	Password := endpointInfo.Password

	// Bail before initiating outbound connections if in dry-run mode
	if dryRunRequested {
		return
	}

	// Connect to the SSH server
	sshClient, err := connectToSSH(endpointInfo.Endpoint, endpointInfo.EndpointUser, endpointInfo.Password, endpointInfo.PrivateKey, endpointInfo.KeyAlgo)
	if err != nil {
		recordDeploymentFailure(endpointName, commitFilePaths, 0, fmt.Errorf("failed connect to SSH server %v", err))
		return
	}
	defer sshClient.Close()

	printMessage(VerbosityProgress, "Host %s: Connected to SSH server\n", endpointName)

	// Get this hosts remote transfer buffer file path
	tmpRemoteFilePath := endpointInfo.RemoteTransferBuffer
	tmpBackupPath := endpointInfo.RemoteBackupDir

	// Need local metric in order to determine what number of configs for this specific host succeeded (to increment global host metric counter)
	var postDeployedConfigsLocal int

	printMessage(VerbosityProgress, "Host %s: Preparing remote config backup directory\n", endpointName)

	// Create backup directory
	command := "mkdir " + tmpBackupPath
	_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 10)
	if err != nil {
		// Since we blindly try to create the directory, ignore errors about it already existing
		if !strings.Contains(err.Error(), "File exists") {
			recordDeploymentFailure(endpointName, commitFilePaths, 0, fmt.Errorf("failed SSH Command on host during creation of backup directory: %v", err))
			return
		}
	}

	printMessage(VerbosityProgress, "Host %s: Starting deployment for configs with reload commands\n", endpointName)

	// Loop over command groups and deploy files that need reload commands
	for reloadID, commitFilePaths := range commitFileByCommand {
		printMessage(VerbosityData, "Host %s: Starting deployment for configs with reload command ID %s\n", endpointName, reloadID)

		// For metrics - get length of this groups file array
		filesRequiringReload := len(commitFilePaths)

		// Deploy all files for this specific reload command set
		backupFileHashes := make(map[string]string)
		var dontRunReloads bool
		for index, commitFilePath := range commitFilePaths {
			printMessage(VerbosityData, "Host %s:   Starting deployment for config %s\n", endpointName, commitFilePath)

			// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
			commitIndex := index + 1

			// Split repository host dir and config file path for obtaining the absolute target file path
			_, targetFilePath := translateLocalPathtoRemotePath(commitFilePath)
			// Reminder:
			// targetFilePath   should be the file path as expected on the remote system
			// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)

			// Run installation commands first if requested
			if allFileInfo[commitFilePath].InstallOptional && config.RunInstallCommands {
				var InstallFailed bool
				for _, command := range allFileInfo[commitFilePath].Install {
					printMessage(VerbosityData, "Host %s:   Running install command '%s'\n", endpointName, command)

					_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 90)
					if err != nil {
						// Record this failed command - first failure always stops file deployment
						recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during installation command %s: %v", command, err))
						InstallFailed = true
						break
					}
				}

				// Skip to next file if install failed
				if InstallFailed {
					// Failures in installation for any single file in a reload group means reloads should not occur
					dontRunReloads = true
					continue
				}
			}

			// Run Check commands before beginning deployment of this file
			if allFileInfo[commitFilePath].ChecksRequired {
				var CheckFailed bool
				for _, command := range allFileInfo[commitFilePath].Checks {
					printMessage(VerbosityData, "Host %s:   Running check command '%s'\n", endpointName, command)

					_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 90)
					if err != nil {
						// Record this failed command - first failure always stops file deployment
						recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during check command %s: %v", command, err))
						CheckFailed = true
						break
					}
				}

				// Skip to next file if check failed
				if CheckFailed {
					// Failures in checks for any single file in a reload group means reloads should not occur
					dontRunReloads = true
					continue
				}
			}

			printMessage(VerbosityData, "Host %s:   Backing up config %s\n", endpointName, targetFilePath)

			// Create a backup config on remote host if remote file already exists
			oldRemoteFileHash, oldRemoteFileMeta, err := backupOldConfig(sshClient, Password, targetFilePath, tmpBackupPath)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during backup of existing config"))
				dontRunReloads = true
				continue
			}

			printMessage(VerbosityData, "Host %s: File '%s': remote hash: '%s' - local hash: '%s'\n", endpointName, targetFilePath, oldRemoteFileHash, allFileInfo[commitFilePath].Hash)

			// Compare hashes and skip to next file deployment if remote is same as local
			if oldRemoteFileHash == allFileInfo[commitFilePath].Hash {
				printMessage(VerbosityData, "Host %s:   Checking if file '%s' needs its metadata updated\n", endpointName, targetFilePath)

				// Modify metadata of file if required
				var fileModified bool
				fileModified, err = modifyMetadata(sshClient, Password, targetFilePath, oldRemoteFileMeta, allFileInfo[commitFilePath].FileOwnerGroup, allFileInfo[commitFilePath].FilePermissions)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during file metadata check: %v", err))
					dontRunReloads = true
					continue
				}

				// If file was modified, continue to reloads, otherwise skip
				if !fileModified {
					printMessage(VerbosityProgress, "Host %s: File '%s' hash matches local and metadata up-to-date... skipping this file\n", endpointName, targetFilePath)
					filesRequiringReload-- // Decrement counter when one file is found to be identical
					continue
				} else {
					printMessage(VerbosityProgress, "Host %s: File '%s' metadata modified, but content hash matches local.\n", endpointName, targetFilePath)
				}
			} else {

				printMessage(VerbosityData, "Host %s:   Transferring config %s to remote\n", endpointName, commitFilePath)

				// Use hash to retrieve file data from map
				hashIndex := allFileInfo[commitFilePath].Hash

				// Transfer config file to remote with correct ownership and permissions
				err = createFile(sshClient, Password, targetFilePath, tmpRemoteFilePath, allFileData[hashIndex], allFileInfo[commitFilePath].Hash, allFileInfo[commitFilePath].FileOwnerGroup, allFileInfo[commitFilePath].FilePermissions)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, err)
					err = restoreOldConfig(sshClient, targetFilePath, tmpBackupPath, oldRemoteFileHash, Password)
					if err != nil {
						recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
					}
					dontRunReloads = true
					continue
				}

				// Record backup file hashes to map in case reload fails and restoration needs to occur
				backupFileHashes[targetFilePath] = oldRemoteFileHash
			}
		}

		// Since all the files use the same command array, just pick out one file to get the reload command array from
		commandReloadArray := allFileInfo[commitFilePaths[0]].Reload

		printMessage(VerbosityProgress, "Host %s: Starting execution of reload commands\n", endpointName)

		// Do not run reloads if file operations encountered error
		if dontRunReloads {
			printMessage(VerbosityProgress, "Host %s:   Refusing to run reloads - file encountered error\n", endpointName)
			continue
		}
		// Do not run reloads if all files are identical local and remote
		if filesRequiringReload == 0 {
			printMessage(VerbosityProgress, "Host %s:   Refusing to run reloads - all files in reload group are unchanged\n", endpointName)
			continue
		}

		// Run all the commands required by this config file group
		var ReloadFailed bool
		for _, command := range commandReloadArray {
			printMessage(VerbosityData, "Host %s:   Running reload command '%s'\n", endpointName, command)

			_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 90)
			if err != nil {
				// Record this failed command - first failure always stops reloads
				// Record failures using the arry of all files for this command group and signal to record all the files using index "0"
				recordDeploymentFailure(endpointName, commitFilePaths, 0, fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err))
				ReloadFailed = true
				break
			}
		}

		printMessage(VerbosityProgress, "Host %s: Finished execution of reload commands\n", endpointName)

		// Restore configs and skip to next reload group if reload failed
		if ReloadFailed {
			printMessage(VerbosityProgress, "Host %s:   Starting restoration of backup configs after reload failure\n", endpointName)

			// Restore all config files for this group
			for index, commitFilePath := range commitFilePaths {
				// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
				commitIndex := index + 1

				// Separate path back into target format
				_, targetFilePath := translateLocalPathtoRemotePath(commitFilePath)

				printMessage(VerbosityData, "Host %s:   Restoring config file %s due to failed reload command\n", endpointName, targetFilePath)

				// Put backup file into origina location
				err = restoreOldConfig(sshClient, targetFilePath, tmpBackupPath, backupFileHashes[targetFilePath], Password)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
				}
			}
			continue
		}

		// Increment local metric for configs by number of files that required reloads
		postDeployedConfigsLocal += filesRequiringReload
	}

	printMessage(VerbosityProgress, "Host %s: Starting deployment for configs without reload commands\n", endpointName)

	// Loop through target files and deploy (non-reload required configs)
	for index, commitFilePath := range commitFilesNoReload {
		printMessage(VerbosityData, "Host %s:   Starting deployment for %s\n", endpointName, commitFilePath)

		// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
		commitIndex := index + 1

		// Split repository host dir and config file path for obtaining the absolute target file path
		_, targetFilePath := translateLocalPathtoRemotePath(commitFilePath)
		// Reminder:
		// targetFilePath   should be the file path as expected on the remote system
		// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)

		// What to do - Create/Delete/symlink the config
		targetFileAction := allFileInfo[commitFilePath].Action

		// Run installation commands first if requested
		if allFileInfo[commitFilePath].InstallOptional && config.RunInstallCommands {
			var InstallFailed bool
			for _, command := range allFileInfo[commitFilePath].Install {
				printMessage(VerbosityData, "Host %s:   Running install command '%s'\n", endpointName, command)

				_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 90)
				if err != nil {
					// Record this failed command - first failure always stops file deployment
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during installation command %s: %v", command, err))
					InstallFailed = true
					break
				}
			}

			// Skip to next file if install failed
			if InstallFailed {
				continue
			}
		}

		// Run Check commands before beginning deployment of this file
		if allFileInfo[commitFilePath].ChecksRequired {
			var CheckFailed bool
			for _, command := range allFileInfo[commitFilePath].Checks {
				printMessage(VerbosityData, "Host %s:   Running check command '%s'\n", endpointName, command)

				_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 90)
				if err != nil {
					// Record this failed command - first failure always stops file deployment
					recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during check command %s: %v", command, err))
					CheckFailed = true
					break
				}
			}

			// Skip to next file if check failed
			if CheckFailed {
				continue
			}
		}

		// Delete file on remote if deleted in repo
		if targetFileAction == "delete" {
			printMessage(VerbosityData, "Host %s:   Deleting config %s\n", endpointName, targetFilePath)

			err = deleteFile(sshClient, Password, targetFilePath)
			if err != nil {
				// Only record errors where removal of the specific file failed
				if strings.Contains(err.Error(), "failed to remove file") {
					recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
					continue
				}

				// Other errors (removing empty parent dirs) are not recorded
				printMessage(VerbosityStandard, "Warning: Host %s: %v\n", endpointName, err)
			}

			// Done deleting (or recording error) - Next deployment file
			postDeployedConfigsLocal++
			continue
		}

		// Create symbolic link if requested
		if strings.Contains(targetFileAction, "symlinkcreate") {
			printMessage(VerbosityData, "Host %s:   Creating symlink %s\n", endpointName, targetFilePath)

			err = createSymLink(sshClient, Password, targetFilePath, targetFileAction)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
				continue
			}

			// Done creating link (or recording error) - Next deployment file
			postDeployedConfigsLocal++
			continue
		}

		// Create/Modify directory if requested
		if targetFileAction == "dirCreate" || targetFileAction == "dirModify" {
			// Trim directory metadata file name from path
			targetFilePath = filepath.Dir(targetFilePath)

			printMessage(VerbosityData, "Host %s:   Checking directory %s\n", endpointName, targetFilePath)

			// Check if directory exists, if not create
			directoryExists, lsOutput, err := CheckRemoteFileDirExistence(sshClient, targetFilePath, Password, true)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
				continue
			}
			if !directoryExists {
				command := "mkdir -p " + targetFilePath
				_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 10)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
					continue
				}
			}

			// Check if dir needs to be created/modified, and do so if required
			var DirModified bool
			DirModified, err = modifyMetadata(sshClient, Password, targetFilePath, lsOutput, allFileInfo[commitFilePath].FileOwnerGroup, allFileInfo[commitFilePath].FilePermissions)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
				continue
			}

			// Only increment metrics for modifications
			if DirModified {
				printMessage(VerbosityData, "Host %s:   Modified Directory %s\n", endpointName, targetFilePath)
				// Done modifying directory (or recording error) - Next deployment file
				postDeployedConfigsLocal++
			}
			continue
		}

		printMessage(VerbosityData, "Host %s:   Backing up config %s\n", endpointName, targetFilePath)

		// Create a backup config on remote host if remote file already exists
		oldRemoteFileHash, oldRemoteFileMeta, err := backupOldConfig(sshClient, Password, targetFilePath, tmpBackupPath)
		if err != nil {
			recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
			continue
		}

		printMessage(VerbosityData, "Host %s: File '%s': remote hash: '%s' - local hash: '%s'\n", endpointName, targetFilePath, oldRemoteFileHash, allFileInfo[commitFilePath].Hash)

		// Compare hashes and skip to next file deployment if remote is same as local
		if oldRemoteFileHash == allFileInfo[commitFilePath].Hash {
			printMessage(VerbosityData, "Host %s:   Checking if file '%s' needs its metadata updated\n", endpointName, targetFilePath)

			// Modify metadata of file if required
			var fileModified bool
			fileModified, err = modifyMetadata(sshClient, Password, targetFilePath, oldRemoteFileMeta, allFileInfo[commitFilePath].FileOwnerGroup, allFileInfo[commitFilePath].FilePermissions)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during file metadata check: %v", err))
				continue
			}

			// If file was modified, continue to metrics and next file, otherwise skip immediately to next file
			if !fileModified {
				printMessage(VerbosityProgress, "Host %s: File '%s' hash matches local and metadata up-to-date... skipping this file\n", endpointName, targetFilePath)
				continue
			} else {
				printMessage(VerbosityProgress, "Host %s: File '%s' metadata modified, but content hash matches local.\n", endpointName, targetFilePath)
			}
		} else {
			printMessage(VerbosityData, "Host %s:   Transferring config '%s' to remote\n", endpointName, commitFilePath)

			// Use hash to retrieve file data from map
			hashIndex := allFileInfo[commitFilePath].Hash

			// Transfer config file to remote with correct ownership and permissions
			err = createFile(sshClient, Password, targetFilePath, tmpRemoteFilePath, allFileData[hashIndex], allFileInfo[commitFilePath].Hash, allFileInfo[commitFilePath].FileOwnerGroup, allFileInfo[commitFilePath].FilePermissions)
			if err != nil {
				recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, err)
				err = restoreOldConfig(sshClient, targetFilePath, tmpBackupPath, oldRemoteFileHash, Password)
				if err != nil {
					recordDeploymentFailure(endpointName, commitFilesNoReload, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
				}
				continue
			}
		}

		// Increment local metric for config
		postDeployedConfigsLocal++
	}

	printMessage(VerbosityProgress, "Host %s: Cleaning up remote temporary directories\n", endpointName)

	// Cleanup temporary files
	command = "rm -r " + tmpRemoteFilePath + " " + tmpBackupPath
	_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, Password, 30)
	if err != nil {
		// Only print error if there was a file to remove in the first place
		if !strings.Contains(err.Error(), "No such file or directory") {
			// Failures to remove the tmp files are not critical, but notify the user regardless
			printMessage(VerbosityStandard, " Warning! Failed to cleanup temporary buffer files: %v\n", err)
		}
	}

	printMessage(VerbosityProgress, "Host %s: Writing to global metric counters\n", endpointName)

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
