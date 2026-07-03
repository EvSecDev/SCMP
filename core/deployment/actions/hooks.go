// Package for all SSH-agnostic deployment actions
package actions

import (
	"context"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/sshinternal"
)

func RunPreApplyCommands(ctx context.Context, host sshinternal.HostMeta, localMetadata deployment.FileInfo) (err error) {
	if localMetadata.PreapplyRequired {
		err = RunCommandSet(ctx, host, "PreApply", localMetadata.Preapply)
	}
	return
}

func RunPostApplyCommands(ctx context.Context, host sshinternal.HostMeta, localMetadata deployment.FileInfo) (err error) {
	if localMetadata.PostapplyRequired {
		err = RunCommandSet(ctx, host, "PostApply", localMetadata.Postapply)
	}
	return
}

func RunInstallationCommands(ctx context.Context, host sshinternal.HostMeta, localMetadata deployment.FileInfo) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")
	if localMetadata.InstallOptional && opts.RunInstallCommands {
		err = RunCommandSet(ctx, host, "Install", localMetadata.Install)
	}
	return
}
