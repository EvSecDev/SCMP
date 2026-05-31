package host

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/core/deployment/actions"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"
)

func (group *fileGroup) deploy(ctx context.Context, deploymentList *deployment.FileGroup, deployFiles *deployment.HostFiles) {
	defer group.deployWG.Done()

	group.deployLimiter <- struct{}{}
	defer func() { <-group.deployLimiter }()

	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logctx.LogStdFatal(ctx,
				"Controller panic during group file deployment to host '%s': %v\n",
				group.hostState.Name, fatalError)
		}
	}()

	// Reload trackers
	totalDeployedReloadFiles := make(map[str.ReloadID]int)                        // Count of successfully deployed files by their reloadID
	reloadIDreadyToReload := make(map[str.ReloadID]bool)                          // Signal when a reload group is cleared to reload
	remoteFileMetadatas := make(map[str.LocalRepoPath]sshinternal.RemoteFileInfo) // Track remote file metadata (mainly for reload failure restoration)

	// Loop through target files and deploy
	for _, repoFilePath := range deploymentList.GetOrderedList() {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Starting deployment for '%s'\n", repoFilePath)

		info := deployFiles.GetFileInfo(repoFilePath)

		// Skip this file if any of its dependents failed deployment
		if len(info.Dependencies) > 0 {
			var failedDependentFile str.LocalRepoPath

			for _, dependentFile := range info.Dependencies {
				fileErr := group.metrics.HostFileHasError(group.hostState.Name, dependentFile)
				if fileErr != nil {
					failedDependentFile = dependentFile
					break
				}
			}

			if failedDependentFile != "" {
				err := fmt.Errorf("unable to deploy this file: dependent file (%s) failed deployment", failedDependentFile)
				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog,
					"File '%s': %w\n", repoFilePath, err)
				group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
				group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
				continue
			}
		}

		// Skip this file if it failed pre-deploy commands
		filePrevError := group.metrics.HostFileHasError(group.hostState.Name, repoFilePath)
		if filePrevError != nil {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.WarnLog,
				"File '%s' has a pre-existing error: unable to deploy due to: %w\n",
				repoFilePath, filePrevError)
			group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
			// Skipping adding to failure map - should have already been added in pre-deploy command function
			continue
		}

		err := actions.RunCheckCommands(ctx, group.hostState, info)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during check command: %w", err)
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog,
				"File '%s': %w\n", repoFilePath, err)
			group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
			group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
			continue
		}

		select {
		case <-ctx.Done():
			err = fmt.Errorf("immediate stop requested before deploying file %s to host %s ",
				info.TargetFilePath, group.hostState.Name)
			group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
			group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
			return
		default:
		}

		err = actions.RunInstallationCommands(ctx, group.hostState, info)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during installation command: %w", err)
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog,
				"File '%s': %w\n", repoFilePath, err)
			group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
			group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
			continue
		}

		// For metrics
		var remoteModified bool
		var transferredBytes int

		// Deploy based on action
		switch info.Action {
		case deployment.ActionDelete:
			remoteModified, err = actions.DeleteFile(ctx, group.hostState, info.TargetFilePath)
			if err != nil {
				if strings.Contains(err.Error(), "failed to remove file") {
					// Record errors where removal of the specific file failed
					logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog,
						"File '%s' failed file deletion: %w\n", repoFilePath, err)
					group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
					group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
				} else {
					// Show warning to user for other errors (removing empty parent dirs)
					logctx.LogStdWarn(ctx, "%w\n", group.hostState.Name, err)
				}
				continue
			}
		case deployment.ActionSymLinkCreate:
			remoteModified, err = actions.DeploySymLink(ctx, group.hostState, info.TargetFilePath, info.LinkTarget)
			if err != nil {
				err = fmt.Errorf("failed deployment of symbolic link: %w", err)
				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog,
					"File '%s': %w\n", repoFilePath, err)
				group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
				group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
				continue
			}
		case deployment.ActionDirCreate, deployment.ActionDirModify:
			remoteModified, remoteFileMetadatas[repoFilePath], err = actions.DeployDirectory(ctx, group.hostState, info)
			if err != nil {
				err = fmt.Errorf("failed deployment of directory: %w", err)
				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog,
					"File '%s': %w\n", repoFilePath, err)
				group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
				group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
				continue
			}
		case deployment.ActionCreate:
			remoteModified, transferredBytes, remoteFileMetadatas[repoFilePath], err = actions.DeployFile(ctx, group.hostState, repoFilePath, deployFiles)
			if err != nil {
				err = fmt.Errorf("failed deployment of file: %w", err)
				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog,
					"File '%s': %w\n", repoFilePath, err)
				group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
				group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
				continue
			}
		}

		// Increment byte counter
		group.metrics.AddHostBytes(group.hostState.Name, transferredBytes)

		// Handle reloads
		clearedToReload, reloadGroup := actions.CheckForReload(ctx,
			group.hostState.Name,
			deploymentList,
			totalDeployedReloadFiles,
			reloadIDreadyToReload,
			repoFilePath,
			remoteModified,
		)
		if clearedToReload {
			// Execute the commands for this reload group
			var warning string
			warning, err = actions.RunReloadCommands(ctx, group.hostState, deploymentList.GetReloadIDCommands(reloadGroup))
			if err != nil {
				if warning != "" {
					logctx.LogStdWarn(ctx, "  %s\n", group.hostState.Name, warning)
				}

				// Reload encountered error, rollback files
				failedFiles := deploymentList.GetReloadIDFilesReverse(reloadGroup)
				for _, failedFile := range failedFiles {
					info := deployFiles.GetFileInfo(failedFile)

					// Restore the failed files
					logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
						"Restoring config file %s due to failed reload command\n", info.TargetFilePath)
					lerr := actions.RestoreOldFile(ctx, group.hostState, info.TargetFilePath, remoteFileMetadatas[failedFile])
					if lerr != nil {
						// Only warning for restoration failures
						logctx.LogStdWarn(ctx, "File restoration failed: %v\n", group.hostState.Name, lerr)
					}

					group.metrics.AddFileFailure(group.hostState.Name, failedFile, err)
				}

				// Record all the files for the reload group and skip to next file deployment
				group.metrics.AddFile(group.hostState.Name, deployFiles, deploymentList.GetReloadIDFiles(reloadGroup)...)

				// Re-execute reload commands after rollback
				warning, err = actions.RunReloadCommands(ctx, group.hostState, deploymentList.GetReloadIDCommands(reloadGroup))
				if err != nil {
					if warning != "" {
						logctx.LogStdWarn(ctx, "%s\n", group.hostState.Name, warning)
					}

					failedRollbackFiles := strings.Builder{}

					failedFiles := deploymentList.GetReloadIDFiles(reloadGroup)
					for _, failedFile := range failedFiles {
						failedRollbackFiles.WriteString(string(failedFile))
						failedRollbackFiles.WriteString("\n")
					}

					logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
						"Failed reload after rollback for file(s):\n%s", failedRollbackFiles.String())
					continue
				}

				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
					"Succeeded reload after rollback for file(s):\n%s", failedFiles)
			}
		}

		// Increment metric for modification
		if remoteModified {
			group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
		}
	}
}
