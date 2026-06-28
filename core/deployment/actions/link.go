package actions

import (
	"context"
	"fmt"
	"path/filepath"
	"scmp/core/deployment/remote"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
)

// Create symbolic link to specific target file (as present in file action string)
func DeploySymLink(ctx context.Context, host sshinternal.HostMeta, linkName str.RemotePath, linkTarget str.RemotePath) (linkModified bool, err error) {
	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Creating symlink %s\n", linkName)

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Check if a file is already there
	oldSymLinkExists, statOutput, err := sshinternal.CheckRemoteFileDirExistence(ctx, host, linkName)
	if err != nil {
		err = fmt.Errorf("failed checking file existence before creating symbolic link: %w", err)
		return
	}

	if oldSymLinkExists {
		// Retrieve existing file information
		var oldMetadata sshinternal.RemoteFileInfo
		oldMetadata, err = sshinternal.ExtractMetadataFromStat(statOutput)
		if err != nil {
			return
		}

		// Error if the remote file is not a link
		if oldMetadata.FsType != remote.SymlinkType {
			err = fmt.Errorf("file already exists where symbolic link is supposed to be created")
			return
		}

		// Nothing to update, return
		if oldMetadata.LinkTarget == linkTarget {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "link target is up-to-date\n")
			return
		}
	}

	// Check if parent directory exists
	directory := str.RemotePath(filepath.Dir(string(linkName)))
	parentDirExists, _, err := sshinternal.CheckRemoteFileDirExistence(ctx, host, directory)
	if err != nil {
		err = fmt.Errorf("failed checking link parent directory existence before creating symbolic link: %w", err)
		return
	}

	if opts.WetRunEnabled {
		linkModified = true // would have been modified
		return
	}

	// Create parent directory if missing
	if !parentDirExists {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Link parent directory '%s' is missing, creating...\n", directory)

		command := sshinternal.BuildMkdir(directory)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser
		_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			return
		}
	}

	// Create symbolic link
	command := sshinternal.BuildLink(linkTarget, linkName)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed to create symbolic link: %w", err)
		return
	}

	linkModified = true
	return
}
