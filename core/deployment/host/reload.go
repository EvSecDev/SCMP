package host

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/core/deployment/actions"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"
)

func NewReloadTracker(deploymentList *deployment.FileGroup, deployFiles *deployment.HostFiles, endpointName str.RepoRootDir) (tracker *reloadTracker) {
	tracker = &reloadTracker{
		fileGroup:                deploymentList,
		hostFiles:                deployFiles,
		hostEndpointName:         endpointName,
		totalDeployedReloadFiles: make(map[str.ReloadID]int),
		reloadIDreadyToReload:    make(map[str.ReloadID]bool),
		remoteFileMetadatas:      make(map[str.LocalRepoPath]sshinternal.RemoteFileInfo),
	}
	return
}

func (tracker *reloadTracker) AddRemoteMetadata(repoPath str.LocalRepoPath, remoteMetadata sshinternal.RemoteFileInfo) {
	tracker.remoteFileMetadatas[repoPath] = remoteMetadata
}

func (tracker *reloadTracker) CheckForReload(ctx context.Context, repoFilePath str.LocalRepoPath, remoteModified bool) (clearedToReload bool, reloadGroup str.ReloadID) {
	reloadID, fileHasReloadGroup := tracker.fileGroup.GetFileReloadID(repoFilePath)

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Nothing to do for this file, early return
	if !fileHasReloadGroup {
		return
	}

	// Increment deployment success for files reload group
	tracker.totalDeployedReloadFiles[reloadID]++

	// Any single file modification triggers reload OR user manually requests it
	if remoteModified || opts.ForceEnabled {
		tracker.reloadIDreadyToReload[reloadID] = true
	}

	// First, catch not-fully-deployed groups
	if tracker.totalDeployedReloadFiles[reloadID] != tracker.fileGroup.GetReloadIDFileCount(reloadID) {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Reload group not fully deployed yet, not running reloads\n")
		return
	}

	// Second, catch groups with no remote modifications
	if !tracker.reloadIDreadyToReload[reloadID] {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Refusing to run reloads - no remote changes made for reload group\n")
		return
	}

	// Third, catch user disabling all reloads
	if opts.DisableReloads && !opts.ForceEnabled {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Force disabling reloads by user request\n")
		return
	}

	// Reload commands will be immediately run
	reloadGroup = reloadID
	clearedToReload = true
	return
}

func (tracker *reloadTracker) RunReload(ctx context.Context, deployGroup *fileGroup, reloadGroup str.ReloadID) (err error) {
	reloadCommands := tracker.fileGroup.GetReloadIDCommands(reloadGroup)

	// Execute the commands for this reload group
	err = actions.RunReloadCommands(ctx, deployGroup.hostState, reloadCommands)
	if err != nil {
		err = fmt.Errorf("reload failed: %w", err)
		return
	}
	return
}

// Reload encountered error, rollback files
func (tracker *reloadTracker) RollbackReload(ctx context.Context, deployGroup *fileGroup, reloadGroup str.ReloadID) (err error) {
	failedFiles := tracker.fileGroup.GetReloadIDFilesReverse(reloadGroup)
	for _, failedFile := range failedFiles {
		info := tracker.hostFiles.GetFileInfo(failedFile)

		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"Restoring config file %s due to failed reload command\n", info.TargetFilePath)

		// Restore the failed files
		lerr := actions.RestoreOldFile(ctx, deployGroup.hostState, info.TargetFilePath, tracker.remoteFileMetadatas[failedFile])
		if lerr != nil {
			// Only warning for restoration failures
			logctx.LogStdWarn(ctx, "File restoration failed: %v\n", deployGroup.hostState.Name, lerr)
		}
	}

	// Re-execute reload commands after rollback
	reloadCommands := tracker.fileGroup.GetReloadIDCommands(reloadGroup)
	err = actions.RunReloadCommands(ctx, deployGroup.hostState, reloadCommands)
	if err != nil {
		reloadFiles := tracker.fileGroup.GetReloadIDFiles(reloadGroup)

		failedRollbackFiles := strings.Builder{}
		for _, failedFile := range reloadFiles {
			failedRollbackFiles.WriteString(string(failedFile))
			failedRollbackFiles.WriteString("\n")
		}

		err = fmt.Errorf("failed reload after rollback for file(s): %w:\n%s", err, failedRollbackFiles.String())
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
		"Succeeded reload after rollback for file(s):\n%v", failedFiles)
	return
}
