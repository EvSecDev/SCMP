// Package for all SSH-agnostic deployment actions
package actions

import (
	"context"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

func RunCheckCommands(ctx context.Context, host sshinternal.HostMeta, localMetadata deployment.FileInfo) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	if localMetadata.ChecksRequired {
		for _, command := range localMetadata.Checks {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Running check command '%s'\n", command)

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
				return
			}
		}
	}
	return
}
