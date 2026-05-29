package host

import (
	"context"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"strings"
)

// Cleans up any temporarily items on the remote host
// Errors are non-fatal, but will be printed to the user
func CleanupRemote(ctx context.Context, host sshinternal.HostMeta) {
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Cleaning up remote temporary directories\n", host.Name)

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Cleanup temporary files
	command := sshinternal.BuildRmAll(host.TransferBufferDir, host.BackupPath)
	command.DisableSudo = opts.DisableSudo
	command.RunAsUser = opts.RunAsUser
	_, err := command.SSHexec(ctx, host.SSHClient, host.Password)
	if err != nil {
		// Only print error if there was a file to remove in the first place
		if !strings.Contains(err.Error(), "No such file or directory") {
			// Failures to remove the tmp files are not critical, but notify the user regardless
			logctx.LogStdWarn(ctx, "Failed to cleanup temporary buffer files: %v\n", err)
		}
	}
}
