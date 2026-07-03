package actions

import (
	"context"
	"fmt"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

func RunCommandSet(ctx context.Context, host sshinternal.HostMeta, setName string, commands []string) (err error) {
	if len(commands) == 0 {
		return
	}

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
		"Starting execution of %s commands\n", setName)

	for _, command := range commands {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Running %s command '%s'\n", setName, command)

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
			err = fmt.Errorf("failed SSH Command on host during %s command %s: %w", setName, command, err)
			return
		}
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Finished execution of %s commands\n", setName)
	return
}
