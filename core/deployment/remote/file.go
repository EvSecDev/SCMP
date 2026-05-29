// Package for remote handling logic that is not low-level SSH code
package remote

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
)

// Retrieves metadata about file/dir from stat
func GetOldRemoteInfo(ctx context.Context, host sshinternal.HostMeta, targetPath str.RemotePath) (remoteMetadata sshinternal.RemoteFileInfo, err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Find if target file exists on remote
	exists, statOutput, err := sshinternal.CheckRemoteFileDirExistence(ctx, host, targetPath)
	if err != nil {
		err = fmt.Errorf("failed checking file presence on remote host: %w", err)
		return
	}

	// Return early if not present
	remoteMetadata.Exists = exists
	if !exists {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "   File %s: remote does not exist, not extracting metadata\n", host.Name, targetPath)
		return
	}

	// Get metadata from the output of the remote stat command
	remoteMetadata, err = sshinternal.ExtractMetadataFromStat(statOutput)
	if err != nil {
		return
	}
	if remoteMetadata.FsType != FileType && remoteMetadata.FsType != DirType && remoteMetadata.FsType != FileEmptyType {
		err = fmt.Errorf("expected remote path to be file or directory, but got type '%s' instead", remoteMetadata.FsType)
		return
	}

	// Ensure name in metadata is the path we received
	remoteMetadata.Name = targetPath

	// Only hash if its a file
	if remoteMetadata.FsType == FileType || remoteMetadata.FsType == FileEmptyType {
		// Get the SHA256 hash of the remote old conf file
		command := sshinternal.BuildHashCmd(targetPath)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser

		var commandOutput string
		commandOutput, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during hash of old config file: %w", err)
			return
		}

		// Parse hash command output to get just the hex
		validHash, hash := parsing.HasHex64Prefix(commandOutput)
		if !validHash {
			err = fmt.Errorf("invalid hash received from remote sha256sum command")
			return
		}
		remoteMetadata.Hash = str.FileID(hash)
	}

	return
}

// Compares compiled metadata from local and remote file and compares them and reports what is different
// Only compares hashes, owner+group, and permission bits
func CheckForDiff(ctx context.Context, remoteMetadata sshinternal.RemoteFileInfo, localMetadata deployment.FileInfo) (contentDiffers bool, metadataDiffers bool) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// If user requested force, return early, as deployment will be atomic
	if opts.ForceEnabled {
		contentDiffers = true
		metadataDiffers = true
		return
	}

	// Check if remote content differs from local
	if remoteMetadata.Hash != localMetadata.Hash {
		contentDiffers = true
	} else if remoteMetadata.Hash == localMetadata.Hash {
		contentDiffers = false
	}

	// Check if remote permissions differs from expected
	var permissionsDiffer bool
	if remoteMetadata.Permissions != localMetadata.Permissions {
		permissionsDiffer = true
	} else if remoteMetadata.Permissions == localMetadata.Permissions {
		permissionsDiffer = false
	}

	// Prevent comparing the literal character ':' against local metadata
	var remoteOwnerGroup string
	if remoteMetadata.Owner != "" && remoteMetadata.Group != "" {
		remoteOwnerGroup = remoteMetadata.Owner + ":" + remoteMetadata.Group
	}

	// Check if remote ownership match expected
	var ownershipDiffers bool
	if remoteOwnerGroup != localMetadata.OwnerGroup {
		ownershipDiffers = true
	} else if remoteOwnerGroup == localMetadata.OwnerGroup {
		ownershipDiffers = false
	}

	// If either piece of metadata differs, whole metadata is different
	if ownershipDiffers || permissionsDiffer {
		metadataDiffers = true
	} else if !ownershipDiffers && !permissionsDiffer {
		metadataDiffers = false
	}

	return
}
