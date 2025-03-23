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

	// Deploy files that dont need any reload commands run
	bytesTransferred, deployedFiles := DeployFiles(host, endpointInfo.deploymentFiles, allFileInfo, allFileData)
	postDeployMetrics.updateCount(deployedFiles, bytesTransferred, 0)

	// Update metric for entire host
	if postDeployMetrics.files > 0 {
		postDeployMetrics.updateCount(0, 0, 1)
	}

	// Do any remote cleanups are required (non-fatal)
	cleanupRemote(host)
}

// #####################################
//     FILE DEPLOYMENT HANDLING
// #####################################

func DeployFiles(host HostMeta, deploymentFiles []string, allFileInfo map[string]FileInfo, allFileData map[string][]byte) (deployedBytes int, deployedConfigs int) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			err := fmt.Errorf("%v", fatalError)
			errDescription := fmt.Sprintf("Controller panic during non-reload deployments to host '%s'", host.name)
			logError(errDescription, err, false) // Log and Exit
		}
	}()

	// Separate files with and without reload commands
	printMessage(verbosityProgress, "Host %s: Grouping config files by reload commands\n", host.name)
	reloadIDtoRepoFile, repoFileToReloadID, reloadIDfileCount := groupFilesByReloads(allFileInfo, deploymentFiles)

	// Count of successfully deployed files by their reloadID
	totalDeployedReloadFiles := make(map[string]int)

	// Track remote file metadata (mainly for reload failure restoration)
	remoteFileMetadatas := make(map[string]RemoteFileInfo)

	// Loop through target files and deploy (non-reload required configs)
	for commitIndex, repoFilePath := range deploymentFiles {
		printMessage(verbosityData, "Host %s: Starting deployment for '%s'\n", host.name, repoFilePath)

		// Split repository host dir and config file path for obtaining the absolute target file path
		// Reminder:
		// targetFilePath   should be the file path as expected on the remote system
		// repoFilePath     should be the local file path within the commit repository - is REQUIRED to reference keys in the big config information maps (commitFileData, commitFileActions, ect.)
		_, targetFilePath := translateLocalPathtoRemotePath(repoFilePath)

		// Run Check commands first
		err := runCheckCommands(host, allFileInfo, repoFilePath)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during check command: %v", err)
			recordDeploymentFailure(host.name, deploymentFiles, commitIndex, err)
			continue
		}

		// Run installation commands before deployments
		err = runInstallationCommands(host, allFileInfo, repoFilePath)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during installation command: %v", err)
			recordDeploymentFailure(host.name, deploymentFiles, commitIndex, err)
			continue
		}

		// For metrics
		var remoteModified bool
		var transferredBytes int

		// Deploy based on action (fs type)
		switch allFileInfo[repoFilePath].action {
		case "delete":
			remoteModified, err = deleteFile(host, targetFilePath)
			if err != nil {
				if strings.Contains(err.Error(), "failed to remove file") {
					// Record errors where removal of the specific file failed
					recordDeploymentFailure(host.name, deploymentFiles, commitIndex, err)
				} else {
					// Show warning to user for other errors (removing empty parent dirs)
					printMessage(verbosityStandard, "Warning: Host %s: %v\n", host.name, err)
				}
				continue
			}
		case "symlinkCreate":
			remoteModified, err = deploySymLink(host, targetFilePath, allFileInfo[repoFilePath].linkTarget)
			if err != nil {
				err = fmt.Errorf("failed deployment of symbolic link: %v", err)
				recordDeploymentFailure(host.name, deploymentFiles, commitIndex, err)
				continue
			}
		case "dirCreate", "dirModify":
			remoteModified, remoteFileMetadatas[repoFilePath], err = deployDirectory(host, targetFilePath, allFileInfo[repoFilePath])
			if err != nil {
				err = fmt.Errorf("failed deployment of directory: %v", err)
				recordDeploymentFailure(host.name, deploymentFiles, commitIndex, err)
				continue
			}
		case "create":
			remoteModified, transferredBytes, remoteFileMetadatas[repoFilePath], err = deployFile(host, targetFilePath, repoFilePath, allFileInfo[repoFilePath], allFileData)
			if err != nil {
				err = fmt.Errorf("failed deployment of file: %v", err)
				recordDeploymentFailure(host.name, deploymentFiles, commitIndex, err)
				continue
			}
		}

		// Increment byte counter
		deployedBytes += transferredBytes

		// Handle reloads
		reloadID, fileHasReloadGroup := repoFileToReloadID[repoFilePath]
		if fileHasReloadGroup {
			// Run reloads when all files in reload group deployed without error
			clearedToReload := checkForReload(host.name, totalDeployedReloadFiles, reloadIDfileCount, reloadID, remoteModified)
			if clearedToReload {
				err = runReloadCommands(host, allFileInfo[repoFilePath].reload)
				if err != nil {
					failedFiles := reloadIDtoRepoFile[reloadID]
					for _, file := range failedFiles {
						// Record which specific files failed
						failedFiles = append(failedFiles, file)

						// Restore the failed files
						printMessage(verbosityData, "Host %s:   Restoring config file %s due to failed reload command\n", host.name, targetFilePath)
						lerr := restoreOldFile(host, targetFilePath, remoteFileMetadatas[file])
						if lerr != nil {
							// Only warning for restoration failures
							printMessage(verbosityStandard, "Warning: Host %s:   File restoration failed: %v\n", host.name, lerr)
						}
					}

					// Record the failed files and skip to next file deployment
					recordDeploymentFailure(host.name, failedFiles, -1, err)
					continue
				}
			} else if totalDeployedReloadFiles[reloadID] != reloadIDfileCount[reloadID] {
				printMessage(verbosityProgress, "Host %s:   Reload group not fully deployed yet (or disabled), not running reloads\n", host.name)
			}
		}

		// Increment metric for modification
		if remoteModified {
			deployedConfigs++
		}
	}
	return
}
