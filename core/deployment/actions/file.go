package actions

import (
	"context"
	"encoding/base64"
	"fmt"
	"scmp/core/deployment"
	"scmp/core/deployment/remote"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
)

func DeployFile(ctx context.Context, host sshinternal.HostMeta, repoFilePath str.LocalRepoPath, deployFiles *deployment.AllFiles) (fileModified bool, deployedBytes int, remoteMetadata sshinternal.RemoteFileInfo, err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	localMetadata := deployFiles.GetFileInfo(repoFilePath)

	targetFilePath := localMetadata.TargetFilePath

	// Retrieve metadata of remote file if it exists
	remoteMetadata, err = remote.GetOldRemoteInfo(ctx, host, targetFilePath)
	if err != nil {
		return
	}

	if remoteMetadata.Exists {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Backing up file %s\n", host.Name, remoteMetadata.Name)

		backupFileName := str.RemotePath(base64.StdEncoding.EncodeToString([]byte(remoteMetadata.Name)))
		tmpBackupFilePath := host.BackupPath + "/" + backupFileName

		command := sshinternal.BuildCp(remoteMetadata.Name, tmpBackupFilePath)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser
		_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			err = fmt.Errorf("error making backup of old config file: %w", err)
			return
		}
	}

	// Get remote vs local status
	contentDiffers, metadataDiffers := remote.CheckForDiff(ctx, remoteMetadata, localMetadata)

	// Next file if this one does not need updating
	if !contentDiffers && !metadataDiffers {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"File '%s' hash matches local and metadata up-to-date... skipping this file\n",
			targetFilePath)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
		"File '%s': remote hash: '%s' - local hash: '%s'\n",
		targetFilePath, remoteMetadata.Hash, localMetadata.Hash)

	if opts.WetRunEnabled {
		fileModified = true // would have been modified
		return
	}

	// Create file if local is empty
	if localMetadata.FileSize == 0 && !remoteMetadata.Exists {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"File '%s' is empty and does not exist on remote, creating\n",
			targetFilePath)

		command := sshinternal.BuildTouch(localMetadata.TargetFilePath)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser
		_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			err = fmt.Errorf("unable to create empty file: %w", err)
			return
		}
	}

	// Update file content
	if contentDiffers && localMetadata.FileSize > 0 {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"Transferring config '%s' to remote\n", repoFilePath)

		// Use hash to retrieve file data from map
		hashIndex := localMetadata.Hash
		data := deployFiles.GetFileData(hashIndex)

		// Transfer config file to remote with correct ownership and permissions
		err = sshinternal.CreateRemoteFile(ctx, host, targetFilePath, data, string(localMetadata.Hash), localMetadata.OwnerGroup, localMetadata.Permissions)
		if err != nil {
			lerr := RestoreOldFile(ctx, host, targetFilePath, remoteMetadata)
			if lerr != nil {
				err = fmt.Errorf("%w: restoration failed: %w", err, lerr)
			}
			return
		}

		// Increment byte metric always after a file was uploaded to remote
		deployedBytes += localMetadata.FileSize

		// For metrics
		fileModified = true
	}

	// Update file metadata
	if metadataDiffers {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"Checking if file '%s' needs its metadata updated\n", targetFilePath)

		err = sshinternal.ModifyMetadata(ctx, host, remoteMetadata, localMetadata)
		if err != nil {
			lerr := RestoreOldFile(ctx, host, targetFilePath, remoteMetadata)
			if lerr != nil {
				err = fmt.Errorf("%w: restoration failed: %w", err, lerr)
			}
			return
		}
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"File '%s': updated metadata\n", targetFilePath)

		// For  metrics
		fileModified = true
	}

	return
}

// Moves backup config file into original location after file deployment failure
// Assumes backup file is located in the directory at backupFilePath
// Ensures restoration worked by hashing and comparing to pre-deployment file hash
func RestoreOldFile(ctx context.Context, host sshinternal.HostMeta, targetFilePath str.RemotePath, remoteMetadata sshinternal.RemoteFileInfo) (err error) {
	// Empty oldRemoteFileHash indicates there was nothing to backup, therefore restore should not occur
	if remoteMetadata.Hash == "" {
		return
	}

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Get the unique id for the backup for the given targetFilePath
	backupFileName := str.RemotePath(base64.StdEncoding.EncodeToString([]byte(targetFilePath)))
	backupFilePath := host.BackupPath + "/" + backupFileName

	// Default user options for commands
	var command sshinternal.RemoteCommand
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser

	// Move backup conf into place
	command = sshinternal.BuildMv(backupFilePath, targetFilePath)
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %w", err)
		return
	}
	command = sshinternal.BuildChmod(remoteMetadata.Permissions, targetFilePath)
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %w", err)
		return
	}
	targetRemoteOwnerGroup := remoteMetadata.Owner + ":" + remoteMetadata.Group
	command = sshinternal.BuildChown(targetRemoteOwnerGroup, targetFilePath)
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during restoration of old config file: %w", err)
		return
	}

	// Check to make sure restore worked with hash
	command = sshinternal.BuildHashCmd(targetFilePath)
	commandOutput, err := command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of old config file: %w", err)
		return
	}

	// Parse hash command output to get just the hex
	validHash, remoteFileHash := parsing.HasHex64Prefix(commandOutput)
	if !validHash {
		err = fmt.Errorf("invalid hash received from remote sha256sum command")
		return
	}

	// Ensure restoration succeeded
	if remoteMetadata.Hash != str.FileID(remoteFileHash) {
		err = fmt.Errorf("restored file hash is different than its original hash")
		return
	}

	return
}
