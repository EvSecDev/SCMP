// controller
package main

import (
	"fmt"
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
			err := fmt.Errorf("%v", fatalError)
			errDescription := fmt.Sprintf("Controller panic during deployment to host '%s'", endpointInfo.endpointName)
			logError(errDescription, err, false) // Log and Exit
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
		err = fmt.Errorf("failed connect to SSH server %v", err)
		recordDeploymentFailure(host.name, endpointInfo.deploymentFiles, -1, err)
		return
	}
	defer host.sshClient.Close()

	// Create the backup directory - Error here is fatal to entire host deployment
	err = initBackupDirectory(host)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during creation of backup directory: %v", err)
		recordDeploymentFailure(host.name, endpointInfo.deploymentFiles, -1, err)
		return
	}

	// Deploy files that need reload commands to be run
	bytesTransferred, deployedFiles := DeployWithReload(host, commitFileByCommand, allFileInfo, allFileData)
	postDeployMetrics.updateCount(deployedFiles, bytesTransferred, 0)

	// Deploy files that dont need any reload commands run
	bytesTransferred, deployedFiles = DeployWithoutReload(host, commitFilesNoReload, allFileInfo, allFileData)
	postDeployMetrics.updateCount(deployedFiles, bytesTransferred, 0)

	// Update metric for entire host
	if postDeployMetrics.files > 0 {
		postDeployMetrics.updateCount(0, 0, 1)
	}

	// Do any remote cleanups are required (non-fatal)
	cleanupRemote(host)
}

// ###################################
//      RELOAD DEPLOYMENT HANDLING
// ###################################

func DeployWithReload(host HostMeta, commitFileByCommand map[string][]string, allFileInfo map[string]FileInfo, allFileData map[string][]byte) (deployedBytes int, deployedConfigs int) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			err := fmt.Errorf("%v", fatalError)
			errDescription := fmt.Sprintf("Controller panic  during reload deployments to host '%s'", host.name)
			logError(errDescription, err, false) // Log and Exit
		}
	}()

	printMessage(verbosityProgress, "Host %s: Starting deployment for configs with reload commands\n", host.name)
	// Loop over command groups and deploy files that need reload commands
	var err error
	for reloadID, repoFilePaths := range commitFileByCommand {
		printMessage(verbosityData, "Host %s: Starting deployment for configs with reload command ID %s\n", host.name, reloadID)

		// For metrics
		var fileModifiedCounter int

		// Deploy all files for this specific reload command set
		backupFileHashes := make(map[string]string)
		var reloadCmdsRequired bool
		for commitIndex, repoFilePath := range repoFilePaths {
			printMessage(verbosityData, "Host %s:   Starting deployment for config %s\n", host.name, repoFilePath)

			// Split repository host dir and config file path for obtaining the absolute target file path
			// Reminder:
			// targetFilePath   should be the file path as expected on the remote system
			// repoFilePath     should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)
			_, targetFilePath := translateLocalPathtoRemotePath(repoFilePath)

			// What to do - Create/Delete/symlink the config
			targetFileAction := allFileInfo[repoFilePath].action

			// Run Check commands first
			err = runCheckCommands(host, allFileInfo, repoFilePath)
			if err != nil {
				err = fmt.Errorf("failed SSH Command on host during check command: %v", err)
				recordDeploymentFailure(host.name, repoFilePaths, commitIndex, err)
				continue
			}

			// Run installation commands before deployments
			err = runInstallationCommands(host, allFileInfo, repoFilePath)
			if err != nil {
				err = fmt.Errorf("failed SSH Command on host during installation command: %v", err)
				recordDeploymentFailure(host.name, repoFilePaths, commitIndex, err)
				continue
			}

			// Create/Modify file if requested
			if targetFileAction == "create" {
				fileModified, transferredBytes, remoteOldMetadata, err := deployFile(host, targetFilePath, repoFilePath, allFileInfo[repoFilePath], allFileData)
				if err != nil {
					if err.Error() != "hash matches local and metadata up-to-date" {
						// Only record deployment errors that are real
						err = fmt.Errorf("failed deployment of file: %v", err)
						recordDeploymentFailure(host.name, repoFilePaths, commitIndex, err)
					}
					continue
				}

				// Increment metric for dir modification
				if fileModified {
					fileModifiedCounter++
					// Set flag to allow reload commands to run for this group
					reloadCmdsRequired = true
				}

				deployedBytes += transferredBytes

				// Record backup file hashes to map in case reload fails and restoration needs to occur
				backupFileHashes[targetFilePath] = remoteOldMetadata.hash
			}
		}

		// Since all the files use the same command array, just pick out one file to get the reload command array from
		commandReloadArray := allFileInfo[repoFilePaths[0]].reload

		// Do not run reloads if file operations encountered error
		if !reloadCmdsRequired {
			printMessage(verbosityProgress, "Host %s:   Refusing to run reloads - no remote changes made for reload group\n", host.name)
			continue
		}

		printMessage(verbosityProgress, "Host %s: Starting execution of reload commands\n", host.name)

		// Run all the commands required by this config file group
		var reloadFailed bool
		for _, command := range commandReloadArray {
			// Skip reloads if globally disabled
			if config.disableReloads {
				printMessage(verbosityProgress, "Host %s:   Skipping reload command '%s'\n", host.name, command)
				continue
			}

			printMessage(verbosityProgress, "Host %s:   Running reload command '%s'\n", host.name, command)

			rawCmd := RemoteCommand{command}
			_, err = rawCmd.SSHexec(host.sshClient, "root", config.disableSudo, host.password, 90)
			if err != nil {
				// Record this failed command - first failure always stops reloads
				// Record failures using the arry of all files for this command group and signal to record all the files using index "-1"
				err = fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err)
				recordDeploymentFailure(host.name, repoFilePaths, -1, err)
				reloadFailed = true
				break
			}
		}

		printMessage(verbosityProgress, "Host %s: Finished execution of reload commands\n", host.name)

		// Restore configs and skip to next reload group if reload failed
		if reloadFailed {
			printMessage(verbosityProgress, "Host %s:   Starting restoration of backup configs after reload failure\n", host.name)

			// Restore all config files for this group
			for commitIndex, repoFilePath := range repoFilePaths {
				// Separate path back into target format
				_, targetFilePath := translateLocalPathtoRemotePath(repoFilePath)

				printMessage(verbosityData, "Host %s:   Restoring config file %s due to failed reload command\n", host.name, targetFilePath)

				// Put backup file into origina location
				err = restoreOldFile(host, targetFilePath, backupFileHashes[targetFilePath])
				if err != nil {
					err = fmt.Errorf("failed old config restoration: %v", err)
					recordDeploymentFailure(host.name, repoFilePaths, commitIndex, err)
				}
			}
			continue
		}

		// Increment local metric for configs by number of files that required reloads
		deployedConfigs += fileModifiedCounter
	}
	return
}

// #####################################
//      NON- RELOAD DEPLOYMENT HANDLING
// #####################################

func DeployWithoutReload(host HostMeta, commitFilesNoReload []string, allFileInfo map[string]FileInfo, allFileData map[string][]byte) (deployedBytes int, deployedConfigs int) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			err := fmt.Errorf("%v", fatalError)
			errDescription := fmt.Sprintf("Controller panic during non-reload deployments to host '%s'", host.name)
			logError(errDescription, err, false) // Log and Exit
		}
	}()

	printMessage(verbosityProgress, "Host %s: Starting deployment for configs without reload commands\n", host.name)

	// Loop through target files and deploy (non-reload required configs)
	for commitIndex, repoFilePath := range commitFilesNoReload {
		printMessage(verbosityData, "Host %s:   Starting deployment for %s\n", host.name, repoFilePath)

		// Split repository host dir and config file path for obtaining the absolute target file path
		_, targetFilePath := translateLocalPathtoRemotePath(repoFilePath)
		// Reminder:
		// targetFilePath   should be the file path as expected on the remote system
		// repoFilePath     should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)

		// Run Check commands first
		err := runCheckCommands(host, allFileInfo, repoFilePath)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during check command: %v", err)
			recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
			continue
		}

		// Run installation commands before deployments
		err = runInstallationCommands(host, allFileInfo, repoFilePath)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during installation command: %v", err)
			recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
			continue
		}

		// What to do - Create/Delete/symlink the config
		targetFileAction := allFileInfo[repoFilePath].action

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

			linkModified, err := createSymLink(host, targetFilePath, targetFileAction)
			if err != nil {
				recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
				continue
			}

			// Increment metric for link creation
			if linkModified {
				deployedConfigs++
			}

			continue
		}

		// Create/Modify directory if requested
		if targetFileAction == "dirCreate" || targetFileAction == "dirModify" {
			dirModified, err := deployDirectory(host, targetFilePath, allFileInfo[repoFilePath])
			if err != nil {
				err = fmt.Errorf("failed deployment of directory: %v", err)
				recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
				continue
			}

			// Increment metric for dir modification
			if dirModified {
				deployedConfigs++
			}

			continue
		}

		// Create/Modify file if requested
		if targetFileAction == "create" {
			fileModified, transferredBytes, _, err := deployFile(host, targetFilePath, repoFilePath, allFileInfo[repoFilePath], allFileData)
			if err != nil {
				if err.Error() != "hash matches local and metadata up-to-date" {
					// Only record deployment errors that are real
					err = fmt.Errorf("failed deployment of file: %v", err)
					recordDeploymentFailure(host.name, commitFilesNoReload, commitIndex, err)
				}
				continue
			}

			// Increment metric for dir modification
			if fileModified {
				deployedConfigs++
			}

			// Add any tracked transferred bytes to metric counter
			deployedBytes += transferredBytes
		}
	}
	return
}
