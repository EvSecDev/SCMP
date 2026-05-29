package actions

import (
	"context"
	"fmt"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"
)

// Deletes given file from remote and parent directory if empty
func DeleteFile(ctx context.Context, host sshinternal.HostMeta, targetFilePath str.RemotePath) (fileDeleted bool, err error) {
	// Note: technically inefficient; if a file is moved within same directory, this will delete the file and parent dir(maybe)
	//                                then when deploying the moved file, it will recreate folder that was just deleted.

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Deleting file '%s'\n", targetFilePath)

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	if opts.WetRunEnabled {
		fileDeleted = true // implied that file will always (try) to be deleted
		return
	}

	// Attempt remove file
	command := sshinternal.BuildRm(targetFilePath)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser
	_, err = command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		// Real errors only if file was present to begin with
		if !strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
			err = fmt.Errorf("failed to remove file '%s': %w", targetFilePath, err)
			return
		}

		// Reset err var
		err = nil
	}

	// Deletion occurred, signal as such
	fileDeleted = true
	return
}
