// controller
package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// ###################################
//      HOST DEPLOYMENT HANDLING
// ###################################

// SSH's into a remote host to deploy files and run reload commands
func sshDeploy(wg *sync.WaitGroup, semaphore chan struct{}, endpointInfo EndpointInfo, allFileInfo map[string]FileInfo, allFileData map[string][]byte, postDeployMetrics *PostDeploymentMetrics) {
	// Signal routine is done after return
	defer wg.Done()

	// Concurrency Limit Signaler
	semaphore <- struct{}{}
	defer func() { <-semaphore }()

	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError(fmt.Sprintf("Controller panic during deployment to host '%s'", endpointInfo.endpointName), fmt.Errorf("%v", fatalError), false)
		}
	}()

	// Separate files with and without reload commands
	printMessage(verbosityProgress, "Host %s: Grouping config files by reload commands\n", endpointInfo.endpointName)
	commitFileByCommand, commitFilesNoReload := groupFilesByReloads(allFileInfo, endpointInfo.deploymentFiles)

	// Save meta info for this host in a structure to easily pass around required pieces
	var host HostMeta
	host.name = endpointInfo.endpointName
	host.password = endpointInfo.password
	host.transferBufferFile = endpointInfo.remoteTransferBuffer
	host.backupPath = endpointInfo.remoteBackupDir

	// Bail before initiating outbound connections if in dry-run mode
	if dryRunRequested {
		return
	}

	// Connect to the SSH server
	var err error
	host.sshClient, err = connectToSSH(endpointInfo.endpointName, endpointInfo.endpoint, endpointInfo.endpointUser, endpointInfo.password, endpointInfo.privateKey, endpointInfo.keyAlgo)
	if err != nil {
		recordDeploymentFailure(host.name, endpointInfo.deploymentFiles, 0, fmt.Errorf("failed connect to SSH server %v", err))
		return
	}
	defer host.sshClient.Close()

	// Create the backup directory - Error here is fatal to entire host deployment
	err = initBackupDirectory(host)
	if err != nil {
		recordDeploymentFailure(host.name, endpointInfo.deploymentFiles, 0, fmt.Errorf("failed SSH Command on host during creation of backup directory: %v", err))
		return
	}

	// Deploy files that need reload commands to be run
	bytesTransferred, deployedFiles := DeployWithReload(host, commitFileByCommand, allFileInfo, allFileData)
	updateMetricCounters(host.name, deployedFiles, bytesTransferred, postDeployMetrics)

	// Deploy files that dont need any reload commands run
	bytesTransferred, deployedFiles = DeployWithoutReload(host, commitFilesNoReload, allFileInfo, allFileData)
	updateMetricCounters(host.name, deployedFiles, bytesTransferred, postDeployMetrics)

	// Do any remote cleanups are required (non-fatal)
	cleanupRemote(host)
}

// ###################################
//      RELOAD DEPLOYMENT HANDLING
// ###################################

func DeployWithReload(host HostMeta, commitFileByCommand map[string][]string, allFileInfo map[string]FileInfo, allFileData map[string][]byte) (deployedBytes int, deployedConfigs int) {
	printMessage(verbosityProgress, "Host %s: Starting deployment for configs with reload commands\n", host.name)
	// Loop over command groups and deploy files that need reload commands
	var err error
	for reloadID, commitFilePaths := range commitFileByCommand {
		printMessage(verbosityData, "Host %s: Starting deployment for configs with reload command ID %s\n", host.name, reloadID)

		// For metrics - get length of this groups file array
		filesRequiringReload := len(commitFilePaths)

		// For metrics - track file size transferred (only for successful reload groups)
		var bytesTransferredLocal int

		// Deploy all files for this specific reload command set
		backupFileHashes := make(map[string]string)
		var dontRunReloads bool
		for index, commitFilePath := range commitFilePaths {
			printMessage(verbosityData, "Host %s:   Starting deployment for config %s\n", host.name, commitFilePath)

			// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
			commitIndex := index + 1

			// Split repository host dir and config file path for obtaining the absolute target file path
			// Reminder:
			// targetFilePath   should be the file path as expected on the remote system
			// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)
			_, targetFilePath := translateLocalPathtoRemotePath(commitFilePath)

			// Run Check commands first
			err = runCheckCommands(host, allFileInfo, commitFilePath)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during check command: %v", err))
				// Failures in checks for any single file in a reload group means reloads should not occur
				dontRunReloads = true
				continue
			}

			// Run installation commands before deployments
			err = runInstallationCommands(host, allFileInfo, commitFilePath)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during installation command: %v", err))
				// Failures in installation for any single file in a reload group means reloads should not occur
				dontRunReloads = true
				continue
			}

			printMessage(verbosityData, "Host %s:   Backing up config %s\n", host.name, targetFilePath)

			// Create a backup config on remote host if remote file already exists
			oldRemoteFileHash, oldRemoteFileMeta, err := backupOldConfig(host, targetFilePath)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during backup of existing config"))
				dontRunReloads = true
				continue
			}

			printMessage(verbosityData, "Host %s: File '%s': remote hash: '%s' - local hash: '%s'\n", host.name, targetFilePath, oldRemoteFileHash, allFileInfo[commitFilePath].hash)

			// Compare hashes and skip to next file deployment if remote is same as local
			if oldRemoteFileHash == allFileInfo[commitFilePath].hash {
				printMessage(verbosityData, "Host %s:   Checking if file '%s' needs its metadata updated\n", host.name, targetFilePath)

				// Modify metadata of file if required
				var fileModified bool
				fileModified, err = modifyMetadata(host, targetFilePath, oldRemoteFileMeta, allFileInfo[commitFilePath].fileOwnerGroup, allFileInfo[commitFilePath].filePermissions)
				if err != nil {
					recordDeploymentFailure(host.name, commitFilePaths, commitIndex, fmt.Errorf("failed SSH Command on host during file metadata check: %v", err))
					dontRunReloads = true
					continue
				}

				// If file was modified, continue to reloads, otherwise skip
				if !fileModified {
					printMessage(verbosityProgress, "Host %s: File '%s' hash matches local and metadata up-to-date... skipping this file\n", host.name, targetFilePath)
					filesRequiringReload-- // Decrement counter when one file is found to be identical
					continue
				} else {
					printMessage(verbosityProgress, "Host %s: File '%s' metadata modified, but content hash matches local.\n", host.name, targetFilePath)
				}
			} else {

				printMessage(verbosityData, "Host %s:   Transferring config %s to remote\n", host.name, commitFilePath)

				// Use hash to retrieve file data from map
				hashIndex := allFileInfo[commitFilePath].hash

				// Transfer config file to remote with correct ownership and permissions
				err = createFile(host, targetFilePath, allFileData[hashIndex], allFileInfo[commitFilePath].hash, allFileInfo[commitFilePath].fileOwnerGroup, allFileInfo[commitFilePath].filePermissions)
				if err != nil {
					recordDeploymentFailure(host.name, commitFilePaths, commitIndex, err)
					err = restoreOldConfig(host, targetFilePath, oldRemoteFileHash)
					if err != nil {
						recordDeploymentFailure(host.name, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
					}
					dontRunReloads = true
					continue
				}

				// Metrics for total bytes transferred for this reload group
				bytesTransferredLocal += allFileInfo[commitFilePath].fileSize

				// Record backup file hashes to map in case reload fails and restoration needs to occur
				backupFileHashes[targetFilePath] = oldRemoteFileHash
			}
		}

		// Since all the files use the same command array, just pick out one file to get the reload command array from
		commandReloadArray := allFileInfo[commitFilePaths[0]].reload

		printMessage(verbosityProgress, "Host %s: Starting execution of reload commands\n", host.name)

		// Do not run reloads if file operations encountered error
		if dontRunReloads {
			printMessage(verbosityProgress, "Host %s:   Refusing to run reloads - file encountered error\n", host.name)
			continue
		}
		// Do not run reloads if all files are identical local and remote
		if filesRequiringReload == 0 {
			printMessage(verbosityProgress, "Host %s:   Refusing to run reloads - all files in reload group are unchanged\n", host.name)
			continue
		}

		// Run all the commands required by this config file group
		var reloadFailed bool
		for _, command := range commandReloadArray {
			// Skip reloads if globally disabled
			if config.disableReloads {
				printMessage(verbosityProgress, "Host %s:   Skipping reload command '%s'\n", host.name, command)
				continue
			}

			printMessage(verbosityProgress, "Host %s:   Running reload command '%s'\n", host.name, command)

			_, err = runSSHCommand(host.sshClient, command, "root", config.disableSudo, host.password, 90)
			if err != nil {
				// Record this failed command - first failure always stops reloads
				// Record failures using the arry of all files for this command group and signal to record all the files using index "0"
				recordDeploymentFailure(host.name, commitFilePaths, 0, fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err))
				reloadFailed = true
				break
			}
		}

		printMessage(verbosityProgress, "Host %s: Finished execution of reload commands\n", host.name)

		// Restore configs and skip to next reload group if reload failed
		if reloadFailed {
			printMessage(verbosityProgress, "Host %s:   Starting restoration of backup configs after reload failure\n", host.name)

			// Restore all config files for this group
			for index, commitFilePath := range commitFilePaths {
				// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
				commitIndex := index + 1

				// Separate path back into target format
				_, targetFilePath := translateLocalPathtoRemotePath(commitFilePath)

				printMessage(verbosityData, "Host %s:   Restoring config file %s due to failed reload command\n", host.name, targetFilePath)

				// Put backup file into origina location
				err = restoreOldConfig(host, targetFilePath, backupFileHashes[targetFilePath])
				if err != nil {
					recordDeploymentFailure(host.name, commitFilePaths, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
				}
			}
			continue
		}

		// Increment local metric for configs by number of files that required reloads
		deployedBytes += bytesTransferredLocal
		deployedConfigs += filesRequiringReload
	}
	return
}

// #####################################
//      NON- RELOAD DEPLOYMENT HANDLING
// #####################################

func DeployWithoutReload(host HostMeta, commitFilesNoReload []string, allFileInfo map[string]FileInfo, allFileData map[string][]byte) (deployedBytes int, deployedConfigs int) {
	printMessage(verbosityProgress, "Host %s: Starting deployment for configs without reload commands\n", host.name)

	// Loop through target files and deploy (non-reload required configs)
	for index, commitFilePath := range commitFilesNoReload {
		printMessage(verbosityData, "Host %s:   Starting deployment for %s\n", host.name, commitFilePath)

		// Move index up one to differentiate between first array item and entire host failure - offset is tracked in record failure function
		commitIndex := index + 1

		// Split repository host dir and config file path for obtaining the absolute target file path
		_, targetFilePath := translateLocalPathtoRemotePath(commitFilePath)
		// Reminder:
		// targetFilePath   should be the file path as expected on the remote system
		// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)

		// Run Check commands first
		err := runCheckCommands(host, allFileInfo, commitFilePath)
		if err != nil {
			recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, fmt.Errorf("failed SSH Command on host during check command: %v", err))
			continue
		}

		// Run installation commands before deployments
		err = runInstallationCommands(host, allFileInfo, commitFilePath)
		if err != nil {
			recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, fmt.Errorf("failed SSH Command on host during installation command: %v", err))
			continue
		}

		// What to do - Create/Delete/symlink the config
		targetFileAction := allFileInfo[commitFilePath].action

		// Delete file on remote if deleted in repo
		if targetFileAction == "delete" {
			printMessage(verbosityData, "Host %s:   Deleting config %s\n", host.name, targetFilePath)

			err = deleteFile(host, targetFilePath)
			if err != nil {
				// Only record errors where removal of the specific file failed
				if strings.Contains(err.Error(), "failed to remove file") {
					recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
					continue
				}

				// Other errors (removing empty parent dirs) are not recorded
				printMessage(verbosityStandard, "Warning: Host %s: %v\n", host.name, err)
			}

			// Done deleting (or recording error) - Next deployment file
			deployedConfigs++
			continue
		}

		// Create symbolic link if requested
		if strings.Contains(targetFileAction, "symlinkcreate") {
			printMessage(verbosityData, "Host %s:   Creating symlink %s\n", host.name, targetFilePath)

			err = createSymLink(host, targetFilePath, targetFileAction)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
				continue
			}

			// Done creating link (or recording error) - Next deployment file
			deployedConfigs++
			continue
		}

		// Create/Modify directory if requested
		if targetFileAction == "dirCreate" || targetFileAction == "dirModify" {
			// Trim directory metadata file name from path
			targetFilePath = filepath.Dir(targetFilePath)

			printMessage(verbosityData, "Host %s:   Checking directory %s\n", host.name, targetFilePath)

			// Check if directory exists, if not create
			directoryExists, lsOutput, err := checkRemoteFileDirExistence(host.sshClient, targetFilePath, host.password, true)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
				continue
			}
			if !directoryExists {
				command := "mkdir -p " + targetFilePath
				_, err = runSSHCommand(host.sshClient, command, "root", config.disableSudo, host.password, 10)
				if err != nil {
					recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
					continue
				}
			}

			// Check if dir needs to be created/modified, and do so if required
			var dirModified bool
			dirModified, err = modifyMetadata(host, targetFilePath, lsOutput, allFileInfo[commitFilePath].fileOwnerGroup, allFileInfo[commitFilePath].filePermissions)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
				continue
			}

			// Only increment metrics for modifications
			if dirModified {
				printMessage(verbosityData, "Host %s:   Modified Directory %s\n", host.name, targetFilePath)
				// Done modifying directory (or recording error) - Next deployment file
				deployedConfigs++
			}
			continue
		}

		printMessage(verbosityData, "Host %s:   Backing up config %s\n", host.name, targetFilePath)

		// Create a backup config on remote host if remote file already exists
		oldRemoteFileHash, oldRemoteFileMeta, err := backupOldConfig(host, targetFilePath)
		if err != nil {
			recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
			continue
		}

		printMessage(verbosityData, "Host %s: File '%s': remote hash: '%s' - local hash: '%s'\n", host.name, targetFilePath, oldRemoteFileHash, allFileInfo[commitFilePath].hash)

		// Compare hashes and skip to next file deployment if remote is same as local
		if oldRemoteFileHash == allFileInfo[commitFilePath].hash {
			printMessage(verbosityData, "Host %s:   Checking if file '%s' needs its metadata updated\n", host.name, targetFilePath)

			// Modify metadata of file if required
			var fileModified bool
			fileModified, err = modifyMetadata(host, targetFilePath, oldRemoteFileMeta, allFileInfo[commitFilePath].fileOwnerGroup, allFileInfo[commitFilePath].filePermissions)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, fmt.Errorf("failed SSH Command on host during file metadata check: %v", err))
				continue
			}

			// If file was modified, continue to metrics and next file, otherwise skip immediately to next file
			if !fileModified {
				printMessage(verbosityProgress, "Host %s: File '%s' hash matches local and metadata up-to-date... skipping this file\n", host.name, targetFilePath)
				continue
			} else {
				printMessage(verbosityProgress, "Host %s: File '%s' metadata modified, but content hash matches local.\n", host.name, targetFilePath)
			}
		} else {
			printMessage(verbosityData, "Host %s:   Transferring config '%s' to remote\n", host.name, commitFilePath)

			// Use hash to retrieve file data from map
			hashIndex := allFileInfo[commitFilePath].hash

			// Transfer config file to remote with correct ownership and permissions
			err = createFile(host, targetFilePath, allFileData[hashIndex], allFileInfo[commitFilePath].hash, allFileInfo[commitFilePath].fileOwnerGroup, allFileInfo[commitFilePath].filePermissions)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
				err = restoreOldConfig(host, targetFilePath, oldRemoteFileHash)
				if err != nil {
					recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, fmt.Errorf("failed old config restoration: %v", err))
				}
				continue
			}
		}

		// Increment local metric for file
		deployedBytes += allFileInfo[commitFilePath].fileSize
		deployedConfigs++
	}
	return
}
