package host

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/core/deployment/actions"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
)

func (group *fileGroup) deploy(ctx context.Context, deploymentList *deployment.FileGroup, deployFiles *deployment.HostFiles) {
	defer group.deployWG.Done()

	group.deployLimiter <- struct{}{}
	defer func() { <-group.deployLimiter }()

	// Recover from panic
	defer func() {
		fatalError := recover()
		if fatalError != nil {
			logctx.LogStdFatal(ctx,
				"Controller panic during group file deployment to host '%s': %v\n",
				group.hostState.Name, fatalError)
		}
	}()

	reloadState := NewReloadTracker(deploymentList, deployFiles, group.hostState.Name)

	// Loop through target files and deploy
	for _, repoFilePath := range deploymentList.GetOrderedList() {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Starting deployment for '%s'\n", repoFilePath)
		info := deployFiles.GetFileInfo(repoFilePath)

		skipReason := group.fileCanDeploy(ctx, info)
		if skipReason != nil {
			group.recordFailure(ctx, repoFilePath, deployFiles, skipReason)
			continue
		}

		select {
		case <-ctx.Done():
			err := fmt.Errorf("immediate stop requested before deploying file to host %s ", group.hostState.Name)
			group.recordFailure(ctx, repoFilePath, deployFiles, err)
			return
		default:
		}

		err := actions.RunInstallationCommands(ctx, group.hostState, info)
		if err != nil {
			group.recordFailure(ctx, repoFilePath, deployFiles, err)
			continue
		}

		err = actions.RunPreApplyCommands(ctx, group.hostState, info)
		if err != nil {
			group.recordFailure(ctx, repoFilePath, deployFiles, err)
			continue
		}

		// Deploy the file
		remoteModified, transferredBytes, err := group.applyFile(ctx, info, deployFiles, reloadState)
		if err != nil {
			group.recordFailure(ctx, repoFilePath, deployFiles, err)
			continue
		}

		err = actions.RunPostApplyCommands(ctx, group.hostState, info)
		if err != nil {
			group.recordFailure(ctx, repoFilePath, deployFiles, err)
			continue
		}

		// Increment byte counter post-success-file-transfer
		group.metrics.AddHostBytes(group.hostState.Name, transferredBytes)

		// Handle reloads
		clearedToReload, reloadGroup := reloadState.CheckForReload(ctx, repoFilePath, remoteModified)
		if clearedToReload {
			err = reloadState.RunReload(ctx, group, reloadGroup)
			if err != nil {
				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog, "Reload Group %s: %w", reloadGroup, err)
				group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
				group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)

				err = reloadState.RollbackReload(ctx, group, reloadGroup)
				if err != nil {
					logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog, "Reload Group %s Rollback: %w", reloadGroup, err)
				}
				continue
			}

			err = reloadState.RunPostInstall(ctx, group, reloadGroup)
			if err != nil {
				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog, "Post-Install Group %s: %w", reloadGroup, err)
				group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
				group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
				continue
			}
		}

		// Increment metric for modification
		if remoteModified {
			group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
		}
	}
}

func (group *fileGroup) recordFailure(ctx context.Context, repoFilePath str.LocalRepoPath, deployFiles *deployment.HostFiles, err error) {
	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.ErrorLog, "File '%s': %w\n", repoFilePath, err)
	group.metrics.AddFile(group.hostState.Name, deployFiles, repoFilePath)
	group.metrics.AddFileFailure(group.hostState.Name, repoFilePath, err)
}

// Determines if file is allowed to proceed with deployment
func (group fileGroup) fileCanDeploy(ctx context.Context, info deployment.FileInfo) (skipReason error) {
	// Skip this file if any of its dependents failed deployment
	if len(info.Dependencies) > 0 {
		for _, dependentFile := range info.Dependencies {
			fileErr := group.metrics.HostFileHasError(group.hostState.Name, dependentFile)
			if fileErr != nil {
				skipReason = fmt.Errorf("unable to deploy this file: dependent file (%s) failed deployment", dependentFile)
				return
			}
		}
	}

	// Skip this file if it failed pre-deploy commands
	filePrevError := group.metrics.HostFileHasError(group.hostState.Name, info.RepoFilePath)
	if filePrevError != nil {
		skipReason = filePrevError
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.WarnLog,
			"has pre-existing error: unable to deploy due to: %w\n", filePrevError)
		return
	}
	return
}

// Deploy based on action
func (group fileGroup) applyFile(ctx context.Context,
	info deployment.FileInfo,
	deployFiles *deployment.HostFiles,
	reloadState *reloadTracker,
) (remoteModified bool, transferredBytes int, err error) {
	var remoteMetadata sshinternal.RemoteFileInfo

	switch info.Action {
	case deployment.ActionDirDelete, deployment.ActionFileDelete, deployment.ActionSymLinkDelete:
		remoteModified, err = actions.DeleteFile(ctx, group.hostState, info.TargetFilePath)
		if err != nil {
			return
		}
	case deployment.ActionSymLinkCreate, deployment.ActionSymLinkModify:
		remoteModified, err = actions.DeploySymLink(ctx, group.hostState, info.TargetFilePath, info.LinkTarget)
		if err != nil {
			err = fmt.Errorf("failed deployment of symbolic link: %w", err)
			return
		}
	case deployment.ActionDirCreate, deployment.ActionDirModify:
		remoteModified, remoteMetadata, err = actions.DeployDirectory(ctx, group.hostState, info)
		if err != nil {
			err = fmt.Errorf("failed deployment of directory: %w", err)
			return
		}
	case deployment.ActionFileCreate, deployment.ActionFileModify:
		data := deployFiles.GetFileData(info.Hash)

		remoteModified, transferredBytes, remoteMetadata, err = actions.DeployFile(ctx, group.hostState, info, data)
		if err != nil {
			err = fmt.Errorf("failed deployment of file: %w", err)
			return
		}
	}

	if remoteMetadata != (sshinternal.RemoteFileInfo{}) {
		reloadState.AddRemoteMetadata(info.RepoFilePath, remoteMetadata)
	}
	return
}
