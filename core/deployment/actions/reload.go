package actions

import (
	"context"
	"fmt"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

func RunReloadCommands(ctx context.Context, host sshinternal.HostMeta, reloadCommands []string) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
		"Starting execution of reload commands\n")

	for _, command := range reloadCommands {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Running reload command '%s'\n", command)

		if opts.WetRunEnabled {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
				"Wet-run enabled, skipping command")
			continue
		}

		done := make(chan struct{})
		go watchLongCommand(ctx, command, done)

		rawCmd := sshinternal.RemoteCommand{
			Raw:          command,
			RunAsUser:    opts.RunAsUser,
			DisableSudo:  opts.DisableSudo,
			Timeout:      opts.ExecutionTimeout,
			StreamStdout: false,
		}
		_, err = rawCmd.SSHexec(ctx, host.SSHClient, host.Password)
		close(done)
		if err != nil {
			err = fmt.Errorf("failed SSH Command on host during reload command %s: %w", command, err)
			return
		}
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Finished execution of reload commands\n")
	return
}
