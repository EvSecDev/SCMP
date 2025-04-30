// controller
package main

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// ###################################
//      HOST DEPLOYMENT HANDLING
// ###################################

// SSH's into a remote host to deploy files and run reload commands
func sshDeploy(wg *sync.WaitGroup, connLimiter chan struct{}, endpointInfo EndpointInfo, proxyInfo EndpointInfo, allFileMeta map[string]FileInfo, allFileData map[string][]byte, deployMetrics *DeploymentMetrics) {
	// Signal routine is done after return
	defer wg.Done()

	connLimiter <- struct{}{}
	defer func() { <-connLimiter }()

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
	var proxyClient *ssh.Client
	host.sshClient, proxyClient, err = connectToSSH(endpointInfo, proxyInfo)
	if err != nil {
		err = fmt.Errorf("failed connect to SSH server %v", err)
		deployMetrics.addFile(host.name, allFileMeta, endpointInfo.deploymentFiles...)
		deployMetrics.addHostFailure(host.name, err)
		return
	}
	if proxyClient != nil {
		defer proxyClient.Close()
	}
	defer host.sshClient.Close()

	// Predeployment checks
	err = remoteDeploymentPreparation(&host)
	if err != nil {
		err = fmt.Errorf("Remote system preparation failed: %v", err)
		deployMetrics.addFile(host.name, allFileMeta, endpointInfo.deploymentFiles...)
		deployMetrics.addHostFailure(host.name, err)
		return
	}

	// Deploy files
	deployFiles(host, endpointInfo.deploymentFiles, allFileMeta, allFileData, deployMetrics)

	// Do any remote cleanups are required (non-fatal)
	cleanupRemote(host)
}

// #####################################
//     FILE DEPLOYMENT HANDLING
// #####################################

func deployFiles(host HostMeta, deploymentFiles []string, allFileMeta map[string]FileInfo, allFileData map[string][]byte, deployMetrics *DeploymentMetrics) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			err := fmt.Errorf("%v", fatalError)
			errDescription := fmt.Sprintf("Controller panic during file deployments to host '%s'", host.name)
			logError(errDescription, err, false) // Log and Exit
		}
	}()

	// Separate files with and without reload commands
	printMessage(verbosityProgress, "Host %s: Grouping config files by reload commands\n", host.name)
	reloadIDtoRepoFile, repoFileToReloadID, reloadIDfileCount := groupFilesByReloads(allFileMeta, deploymentFiles)

	// Count of successfully deployed files by their reloadID
	totalDeployedReloadFiles := make(map[string]int)

	// Track remote file metadata (mainly for reload failure restoration)
	remoteFileMetadatas := make(map[string]RemoteFileInfo)

	// Loop through target files and deploy (non-reload required configs)
	for _, repoFilePath := range deploymentFiles {
		printMessage(verbosityData, "Host %s: Starting deployment for '%s'\n", host.name, repoFilePath)

		// Skip this file if any of its dependents failed deployment
		if len(allFileMeta[repoFilePath].dependencies) > 0 {
			var failedDependentFile string

			deployMetrics.fileErrMutex.RLock()
			for _, dependentFile := range allFileMeta[repoFilePath].dependencies {
				if deployMetrics.fileErr[dependentFile] != "" {
					failedDependentFile = dependentFile
					break
				}
			}
			deployMetrics.fileErrMutex.RUnlock()

			if failedDependentFile != "" {
				deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
				deployMetrics.addFileFailure(repoFilePath, fmt.Errorf("unable to deploy this file: dependent file (%s) failed deployment", failedDependentFile))
				continue
			}
		}

		err := runCheckCommands(host, allFileMeta[repoFilePath])
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during check command: %v", err)
			deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
			deployMetrics.addFileFailure(repoFilePath, err)
			continue
		}

		err = runInstallationCommands(host, allFileMeta[repoFilePath])
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during installation command: %v", err)
			deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
			deployMetrics.addFileFailure(repoFilePath, err)
			continue
		}

		// For metrics
		var remoteModified bool
		var transferredBytes int

		// Deploy based on action
		switch allFileMeta[repoFilePath].action {
		case "delete":
			remoteModified, err = deleteFile(host, allFileMeta[repoFilePath].targetFilePath)
			if err != nil {
				if strings.Contains(err.Error(), "failed to remove file") {
					// Record errors where removal of the specific file failed
					deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
					deployMetrics.addFileFailure(repoFilePath, err)
				} else {
					// Show warning to user for other errors (removing empty parent dirs)
					printMessage(verbosityStandard, "Warning: Host %s: %v\n", host.name, err)
				}
				continue
			}
		case "symlinkCreate":
			remoteModified, err = deploySymLink(host, allFileMeta[repoFilePath].targetFilePath, allFileMeta[repoFilePath].linkTarget)
			if err != nil {
				err = fmt.Errorf("failed deployment of symbolic link: %v", err)
				deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
				deployMetrics.addFileFailure(repoFilePath, err)
				continue
			}
		case "dirCreate", "dirModify":
			remoteModified, remoteFileMetadatas[repoFilePath], err = deployDirectory(host, allFileMeta[repoFilePath])
			if err != nil {
				err = fmt.Errorf("failed deployment of directory: %v", err)
				deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
				deployMetrics.addFileFailure(repoFilePath, err)
				continue
			}
		case "create":
			remoteModified, transferredBytes, remoteFileMetadatas[repoFilePath], err = deployFile(host, repoFilePath, allFileMeta[repoFilePath], allFileData)
			if err != nil {
				err = fmt.Errorf("failed deployment of file: %v", err)
				deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
				deployMetrics.addFileFailure(repoFilePath, err)
				continue
			}
		}

		// Increment byte counter
		deployMetrics.addHostBytes(host.name, transferredBytes)

		// Handle reloads
		reloadID, fileHasReloadGroup := repoFileToReloadID[repoFilePath]
		if fileHasReloadGroup {
			// Run reloads when all files in reload group deployed without error
			clearedToReload := checkForReload(host.name, totalDeployedReloadFiles, reloadIDfileCount, reloadID, remoteModified)
			if clearedToReload {
				err = runReloadCommands(host, allFileMeta[repoFilePath].reload)
				if err != nil {
					failedFiles := reloadIDtoRepoFile[reloadID]
					for _, failedFile := range failedFiles {
						// Restore the failed files
						printMessage(verbosityData, "Host %s:   Restoring config file %s due to failed reload command\n", host.name, allFileMeta[repoFilePath].targetFilePath)
						lerr := restoreOldFile(host, allFileMeta[repoFilePath].targetFilePath, remoteFileMetadatas[failedFile])
						if lerr != nil {
							// Only warning for restoration failures
							printMessage(verbosityStandard, "Warning: Host %s:   File restoration failed: %v\n", host.name, lerr)
						}

						deployMetrics.addFileFailure(failedFile, err)
					}

					// Record all the files for the reload group and skip to next file deployment
					deployMetrics.addFile(host.name, allFileMeta, reloadIDtoRepoFile[reloadID]...)
					continue
				}
			} else if totalDeployedReloadFiles[reloadID] != reloadIDfileCount[reloadID] {
				printMessage(verbosityProgress, "Host %s:   Reload group not fully deployed yet (or disabled), not running reloads\n", host.name)
			}
		}

		// Increment metric for modification
		if remoteModified {
			deployMetrics.addFile(host.name, allFileMeta, repoFilePath)
		}
	}
}
