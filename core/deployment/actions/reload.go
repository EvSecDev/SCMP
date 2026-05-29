package actions

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
)

func CheckForReload(ctx context.Context, endpointName str.RepoRootDir, deploymentList *deployment.FileGroup, totalDeployedReloadFiles map[str.ReloadID]int, reloadIDreadyToReload map[str.ReloadID]bool, repoFilePath str.LocalRepoPath, remoteModified bool) (clearedToReload bool, reloadGroup str.ReloadID) {
	reloadID, fileHasReloadGroup := deploymentList.GetFileReloadID(repoFilePath)

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Nothing to do for this file, early return
	if !fileHasReloadGroup {
		return
	}

	// Increment deployment success for files reload group
	totalDeployedReloadFiles[reloadID]++

	// Any single file modification triggers reload OR user manually requests it
	if remoteModified || opts.ForceEnabled {
		reloadIDreadyToReload[reloadID] = true
	}

	// First, catch not-fully-deployed groups
	if totalDeployedReloadFiles[reloadID] != deploymentList.GetReloadIDFileCount(reloadID) {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Reload group not fully deployed yet, not running reloads\n")
		return
	}

	// Second, catch groups with no remote modifications
	if !reloadIDreadyToReload[reloadID] {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Refusing to run reloads - no remote changes made for reload group\n")
		return
	}

	// Third, catch user disabling all reloads
	if opts.DisableReloads && !opts.ForceEnabled {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Force disabling reloads by user request\n")
		return
	}

	// Reload commands will be immediately run
	reloadGroup = reloadID
	clearedToReload = true
	return
}

func RunReloadCommands(ctx context.Context, host sshinternal.HostMeta, reloadCommands []string) (warning string, err error) {
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
