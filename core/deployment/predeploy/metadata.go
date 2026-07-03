package predeploy

import (
	"context"
	"scmp/core/deployment"
	"scmp/core/filesystem"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
)

// Parse JSON metadata into File Info Struct
func jsonToFileInfo(ctx context.Context, repoFilePath str.LocalRepoPath, json filesystem.MetaHeader, fileSize int, commitFileAction str.DeployAction, fileID str.FileID) (info deployment.FileInfo) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	info.Action = commitFileAction
	info.RepoFilePath = repoFilePath
	_, info.TargetFilePath = parsing.TranslateLocalPathtoRemotePath(cfg.RepositoryPath, repoFilePath)
	info.OwnerGroup = json.TargetFileOwnerGroup
	info.Permissions = json.TargetFilePermissions

	info.LinkTarget = json.SymbolicLinkTarget
	if info.LinkTarget != "" {
		info.Action = deployment.ActionSymLinkCreate
	}

	if fileSize > 0 {
		info.FileSize = fileSize
	}

	info.Predeploy = json.PreDeployCommands
	if len(info.Predeploy) > 0 {
		info.PredeployRequired = true
	} else {
		info.PredeployRequired = false
	}

	info.Reload = json.ReloadCommands
	if len(info.Reload) > 0 {
		info.ReloadRequired = true
	} else {
		info.ReloadRequired = false
	}

	if json.ReloadGroup != "" {
		info.ReloadGroup = json.ReloadGroup
	}

	info.Preapply = json.PreapplyCommands
	if len(info.Preapply) > 0 {
		info.PreapplyRequired = true
	} else {
		info.PreapplyRequired = false
	}

	info.Postapply = json.PostapplyCommands
	if len(info.Postapply) > 0 {
		info.PostapplyRequired = true
	} else {
		info.PostapplyRequired = false
	}

	info.Install = json.InstallCommands
	info.PostInstall = json.PostInstallCommands
	if len(info.Install) > 0 || len(info.PostInstall) > 0 {
		info.InstallOptional = true
	} else if len(info.Install) == 0 && len(info.PostInstall) == 0 {
		info.InstallOptional = false
	}

	info.Dependencies = json.Dependencies

	if len(fileID) > 0 {
		info.Hash = fileID
	}

	// Print verbose file metadata information
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Owner and Group:      %s\n", info.OwnerGroup)
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Permissions:          %d\n", info.Permissions)
	if info.LinkTarget != "" {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Link Target           %s\n", info.LinkTarget)
	}
	if len(info.Hash) > 0 {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Content Hash:         %s\n", info.Hash)
	}
	if len(info.Dependencies) > 0 {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Dependencies          %v\n", info.Dependencies)
	}
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Install Required?     %t\n", info.InstallOptional)
	if info.InstallOptional {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Install Commands      %s\n", info.Install)

		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      PostInstall Commands  %s\n", info.PostInstall)
	}
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Preapply Required?    %t\n", info.PreapplyRequired)
	if info.PreapplyRequired {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Preapply Commands     %s\n", info.Preapply)
	}
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Postapply Required?   %t\n", info.PostapplyRequired)
	if info.PostapplyRequired {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Postapply Commands     %s\n", info.Postapply)
	}
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Reload Required?      %t\n", info.ReloadRequired)
	if info.ReloadRequired {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Reload Commands       %s\n", info.Reload)
	}
	if info.ReloadGroup != "" {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Reload Group          %s\n", info.ReloadGroup)
	}
	return
}
