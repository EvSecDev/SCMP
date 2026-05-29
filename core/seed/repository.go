package seed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/deployment/remote"
	"scmp/core/filesystem"
	"scmp/core/filesystem/content"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"
)

// Downloads user selected files/directories and metadata and writes information to repository
func handleSelectedFile(ctx context.Context, remoteFilePath string, host sshinternal.HostMeta, optCache *RepoUserChoiceCache) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "options", global.OpsKey, "config.Opts")

	// Ensure decorators from ls do not get fed into repo
	remoteFilePath = strings.TrimSuffix(remoteFilePath, "*")
	remoteFilePath = strings.TrimSuffix(remoteFilePath, "@")

	// Use target file path and hosts name for repo file location
	localFilePath := str.LocalRepoPath(filepath.Join(string(host.Name), strings.ReplaceAll(remoteFilePath, "/", string(os.PathSeparator))))

	remotePath := str.RemotePath(remoteFilePath)

	command := sshinternal.BuildUnameKernel()
	unameOutput, err := command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed to determine OS, cannot continue: %w", err)
		return
	}
	osName := strings.ToLower(unameOutput)

	// Build stat command based on remote OS
	if strings.Contains(osName, "bsd") {
		command = sshinternal.BuildBSDStat(remotePath)
	} else if strings.Contains(osName, "linux") {
		command = sshinternal.BuildStat(remotePath)
	} else {
		err = fmt.Errorf("received unknown os type: %s", unameOutput)
		return
	}
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser
	statOutput, err := command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %w", err)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Selection '%s': Parsing metadata...\n", remoteFilePath)

	selectionMetadata, err := sshinternal.ExtractMetadataFromStat(statOutput)
	if err != nil {
		err = fmt.Errorf("failed parsing stat output: %w", err)
		return
	}

	if selectionMetadata.FsType == remote.DirType {
		err = content.WriteNewDirectoryMetadata(ctx, localFilePath, selectionMetadata)
		return
	}

	if selectionMetadata.FsType == remote.SymlinkType {
		err = content.WriteSymbolicLinkToRepo(ctx, localFilePath, selectionMetadata)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  File '%s': Downloading file\n", remoteFilePath)

	// Custom cp, no need to use -p
	command = sshinternal.RemoteCommand{
		Raw:          "cp '" + remoteFilePath + "' '" + string(host.TransferBufferDir) + "'",
		DisableSudo:  opts.DisableSudo,
		RunAsUser:    opts.RunAsUser,
		Timeout:      20,
		StreamStdout: false,
	}
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %w", err)
		return
	}

	command = sshinternal.BuildChmod(666, host.TransferBufferDir)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %w", err)
		return
	}

	fileContents, err := sshinternal.SCPDownload(ctx, host.SSHClient, host.TransferBufferDir)
	if err != nil {
		return
	}

	// Retrieve and write to repo parent directory permissions that are unique
	err = writeNewDirectoryTreeMetadata(ctx, string(host.Name), remoteFilePath, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed to walk directory tree metadata for file %s: %w", remoteFilePath, err)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  File '%s': Parsing metadata information\n", remoteFilePath)

	// Metadata header
	var fileMetadata filesystem.MetaHeader
	fileMetadata.TargetFileOwnerGroup = selectionMetadata.Owner + ":" + selectionMetadata.Group
	fileMetadata.TargetFilePermissions = selectionMetadata.Permissions

	// Get reload commands from user
	fileMetadata.ReloadCommands, err = handleNewReloadCommands(ctx, remoteFilePath, string(localFilePath), optCache)
	if err != nil {
		return
	}

	// Check for binary files and handle them separately from text files
	fileMetadata.ExternalContentLocation, err = content.HandleArtifactFiles(ctx, &localFilePath, &fileContents, optCache.ArtifactExtDir)
	if err != nil {
		return
	}

	// Write metadata and content to repository file
	err = content.WriteRepoFile(ctx, localFilePath, fileMetadata, &fileContents)
	if err != nil {
		err = fmt.Errorf("failed to add file to repository: %w", err)
		return
	}

	return
}
