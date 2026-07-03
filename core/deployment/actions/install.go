package actions

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

func RunInstallationCommands(ctx context.Context, host sshinternal.HostMeta, localMetadata deployment.FileInfo) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	if localMetadata.InstallOptional && opts.RunInstallCommands {
		for _, command := range localMetadata.Install {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"Running install command '%s'\n", command)

			if opts.WetRunEnabled {
				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
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
				err = fmt.Errorf("failed SSH Command on host during installation command: %w", err)
				return
			}
		}
	}

	return
}
