package sshinternal

import (
	"context"
	"encoding/base64"
	"fmt"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

// Transfers file into place with correct permissions and ownership
func CreateRemoteFile(ctx context.Context, host HostMeta, targetFilePath str.RemotePath, fileContents []byte, fileContentHash string, fileOwnerGroup string, filePermissions int) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Check if remote dir exists, if not create
	directoryPath := str.FilePathDir(targetFilePath)
	directoryExists, _, err := CheckRemoteFileDirExistence(ctx, host, directoryPath)
	if err != nil {
		err = fmt.Errorf("failed checking directory existence: %w", err)
		return
	}
	if !directoryExists {
		command := BuildMkdir(directoryPath)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser

		_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			err = fmt.Errorf("failed to create directory: %w", err)
			return
		}
	}

	// Unique file name for buffer file
	tempFileName := str.RemotePath(base64.URLEncoding.EncodeToString([]byte(targetFilePath)))
	bufferFilePath := host.TransferBufferDir + "/" + tempFileName

	// SCP to temp file
	err = SCPUpload(ctx, host.SSHClient, fileContents, bufferFilePath)
	if err != nil {
		return
	}

	// Ensure owner/group are correct
	command := BuildChown(fileOwnerGroup, bufferFilePath)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser

	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during owner/group change: %w", err)
		return
	}

	// Ensure permissions are correct
	command = BuildChmod(filePermissions, bufferFilePath)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser

	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during permissions change: %w", err)
		return
	}

	// Move file from tmp dir to actual deployment path
	command = BuildMv(bufferFilePath, targetFilePath)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser

	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed to move new file into place: %w", err)
		return
	}

	// Check if deployed file is present on disk
	newFileExists, _, err := CheckRemoteFileDirExistence(ctx, host, targetFilePath)
	if err != nil {
		err = fmt.Errorf("error checking deployed file presence on remote host: %w", err)
		return
	}
	if !newFileExists {
		err = fmt.Errorf("deployed file on remote host is not present after file transfer")
		return
	}

	// Ensure final file is intact
	command = BuildHashCmd(targetFilePath)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser

	commandOutput, err := command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		err = fmt.Errorf("failed SSH Command on host during hash of deployed file: %w", err)
		return
	}

	validHash, newRemoteFileHash := parsing.HasHex64Prefix(commandOutput)
	if !validHash {
		err = fmt.Errorf("invalid hash received from remote sha256sum command")
		return
	}

	if newRemoteFileHash != fileContentHash {
		err = fmt.Errorf("hash of config file post deployment does not match hash of pre deployment")
		return
	}

	return
}

func ExecuteScript(ctx context.Context, host HostMeta, scriptInterpreter string, remoteFilePath str.RemotePath, scriptFileBytes []byte, scriptHash string, streamOutput bool) (out string, err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Unique file name for buffer file
	tempFileName := str.RemotePath(base64.URLEncoding.EncodeToString([]byte(remoteFilePath)))
	bufferFilePath := host.TransferBufferDir + "/" + tempFileName

	err = SCPUpload(ctx, host.SSHClient, scriptFileBytes, bufferFilePath)
	if err != nil {
		return
	}

	var command RemoteCommand
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser

	command = BuildMv(bufferFilePath, remoteFilePath)
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		return
	}

	command = BuildHashCmd(remoteFilePath)
	remoteScriptHash, err := command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		return
	}
	// Parse hash command output to get just the hex
	validHash, remoteScriptHash := parsing.HasHex64Prefix(remoteScriptHash)
	if !validHash {
		err = fmt.Errorf("invalid hash received from remote sha256sum command")
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Remote Script Hash '%s'\n", remoteScriptHash)

	// Ensure original hash is identical to remote hash
	if remoteScriptHash != scriptHash {
		err = fmt.Errorf("remote script hash does not match local hash, bailing on execution")
		return
	}

	command = BuildChmod(700, remoteFilePath)
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		return
	}

	if !opts.WetRunEnabled {
		command.Raw = scriptInterpreter + " '" + string(remoteFilePath) + "'"
		command.Timeout = opts.ExecutionTimeout
		command.StreamStdout = streamOutput
		out, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			return
		}
	} else {
		// Verify script on wet-run

		var statOutput string
		_, statOutput, err = CheckRemoteFileDirExistence(ctx, host, remoteFilePath)
		if err != nil {
			return
		}

		var scriptInfo RemoteFileInfo
		scriptInfo, err = ExtractMetadataFromStat(statOutput)
		if err != nil {
			return
		}

		if !scriptInfo.Exists {
			err = fmt.Errorf("uploaded script was not found at path %s", remoteFilePath)
			return
		}

		if scriptInfo.Permissions < 700 {
			err = fmt.Errorf("uploaded script could not be made executable")
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host '%s': Script would have executed\n", host.Name)
	}

	// Cleanup
	command = BuildRm(remoteFilePath)
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		return
	}

	return
}

// Checks if file/dir is already present on remote host
// Also retrieve metadata for file/dir
func CheckRemoteFileDirExistence(ctx context.Context, host HostMeta, remotePath str.RemotePath) (exists bool, statOutput string, err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	var command RemoteCommand
	switch host.OSFamily {
	case "bsd":
		command = BuildBSDStat(remotePath)
	case "linux":
		command = BuildStat(remotePath)
	default:
		err = fmt.Errorf("unknown OS family")
		return
	}
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser

	statOutput, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		exists = false
		if strings.Contains(err.Error(), "No such file or directory") {
			err = nil
			return
		}
		return
	}
	exists = true
	return
}

// Modifies metadata if supplied remote file/dir metadata does not match supplied metadata
func ModifyMetadata(ctx context.Context, host HostMeta, remoteMetadata RemoteFileInfo, localMetadata deployment.FileInfo) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Change permissions if different
	if remoteMetadata.Permissions != localMetadata.Permissions {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "   File '%s': changing permissions\n", host.Name, localMetadata.TargetFilePath)

		command := BuildChmod(localMetadata.Permissions, localMetadata.TargetFilePath)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser

		_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during permissions change: %w", err)
			return
		}
	}

	// Change ownership if different
	remoteOwnerGroup := remoteMetadata.Owner + ":" + remoteMetadata.Group
	if remoteOwnerGroup != localMetadata.OwnerGroup {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "   File '%s': changing ownership\n", host.Name, localMetadata.TargetFilePath)

		command := BuildChown(localMetadata.OwnerGroup, localMetadata.TargetFilePath)
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser

		_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during owner/group change: %w", err)
			return
		}
	}

	return
}
