package actions

import (
	"context"
	"scmp/core/deployment"
	"scmp/core/deployment/remote"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

func DeployDirectory(ctx context.Context, host sshinternal.HostMeta, dirInfo deployment.FileInfo) (dirModified bool, remoteMetadata sshinternal.RemoteFileInfo, err error) {
	targetDirPath := dirInfo.TargetFilePath
	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Checking directory '%s'\n", targetDirPath)

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Retrieve metadata of remote file if it exists
	remoteMetadata, err = remote.GetOldRemoteInfo(ctx, host, targetDirPath)
	if err != nil {
		return
	}

	// Create directory if it does not exist
	if !remoteMetadata.Exists {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Directory '%s' is missing, creating...\n", targetDirPath)

		if opts.WetRunEnabled {
			return
		}

		command := sshinternal.BuildMkdir(targetDirPath)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser
		_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			return
		}

		// Update metadata var with existence
		remoteMetadata.Exists = true

		// For metrics
		dirModified = true
	}

	// Check if metadata on directory is up-to-date
	_, metadataDiffers := remote.CheckForDiff(ctx, remoteMetadata, dirInfo)
	if !metadataDiffers {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Directory '%s' metadata is up-to-date... skipping changes\n", targetDirPath)
		return
	}

	// Correct metadata of directory
	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Updating metadata for directory %s\n", targetDirPath)

	if opts.WetRunEnabled {
		dirModified = true // would have been modified
		return
	}

	err = sshinternal.ModifyMetadata(ctx, host, remoteMetadata, dirInfo)
	if err != nil {
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Modified Directory %s\n", targetDirPath)

	// For metrics
	dirModified = true

	return
}
