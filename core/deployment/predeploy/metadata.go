package predeploy

import (
	"context"
	"scmp/core/deployment"
	"scmp/core/filesystem"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"
)

// Parse JSON metadata into File Info Struct
func jsonToFileInfo(ctx context.Context, repoFilePath str.LocalRepoPath, json filesystem.MetaHeader, fileSize int, commitFileAction str.DeployAction, fileID str.FileID) (info deployment.FileInfo) {
	info.Action = commitFileAction
	_, info.TargetFilePath = translateLocalPathtoRemotePath(ctx, repoFilePath)
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
		fileMacroToValue(ctx, repoFilePath, &info.Predeploy)
	} else {
		info.PredeployRequired = false
	}

	info.Reload = json.ReloadCommands
	if len(info.Reload) > 0 {
		info.ReloadRequired = true
		fileMacroToValue(ctx, repoFilePath, &info.Reload)
	} else {
		info.ReloadRequired = false
	}

	if json.ReloadGroup != "" {
		info.ReloadGroup = json.ReloadGroup
	}

	info.Checks = json.CheckCommands
	if len(info.Checks) > 0 {
		info.ChecksRequired = true
		fileMacroToValue(ctx, repoFilePath, &info.Checks)
	} else {
		info.ChecksRequired = false
	}

	info.Install = json.InstallCommands
	if len(info.Install) > 0 {
		info.InstallOptional = true
		fileMacroToValue(ctx, repoFilePath, &info.Install)
	} else {
		info.InstallOptional = false
	}

	info.Dependencies = json.Dependencies
	if len(info.Dependencies) > 0 {
		var strDeps []string
		for _, dep := range info.Dependencies {
			strDeps = append(strDeps, string(dep))
		}
		fileMacroToValue(ctx, repoFilePath, &strDeps)
	}

	if len(fileID) > 0 {
		info.Hash = fileID
	}

	// Print verbose file metadata information
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Owner and Group:  %s\n", info.OwnerGroup)
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Permissions:      %d\n", info.Permissions)
	if info.LinkTarget != "" {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Link Target  %s\n", info.LinkTarget)
	}
	if len(info.Hash) > 0 {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Content Hash:     %s\n", info.Hash)
	}
	if len(info.Dependencies) > 0 {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Dependencies  %v\n", info.Dependencies)
	}
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Install Required? %t\n", info.InstallOptional)
	if info.InstallOptional {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Install Commands  %s\n", info.Install)
	}
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Checks Required?  %t\n", info.ChecksRequired)
	if info.ChecksRequired {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Check Commands    %s\n", info.Checks)
	}
	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Reload Required?  %t\n", info.ReloadRequired)
	if info.ReloadRequired {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Reload Commands   %s\n", info.Reload)
	}
	if info.ReloadGroup != "" {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Reload Group      %s\n", info.ReloadGroup)
	}
	return
}

// Convert any macros to their actual values
// Alters input value to replace all occurrences of supported macros
func fileMacroToValue(ctx context.Context, filePath str.LocalRepoPath, inputs *[]string) {
	const fileNameMacro string = "{@FILENAME}"
	const filePathMacro string = "{@FILEPATH}"
	const fileDirMacro string = "{@FILEDIR}"
	const repoBaseDirMacro string = "{@REPOBASEDIR}"

	// Get hostname for config lookups for macro values
	repoBaseDir, targetFilePath := translateLocalPathtoRemotePath(ctx, filePath)
	baseFileName := str.FilePathBase(targetFilePath)
	fileDirPath := str.FilePathDir(targetFilePath)

	// Replace values in inputs
	for index, input := range *inputs {
		// Replace all occurrences of all macros
		input = strings.ReplaceAll(input, fileNameMacro, string(baseFileName))
		input = strings.ReplaceAll(input, filePathMacro, string(targetFilePath))
		input = strings.ReplaceAll(input, fileDirMacro, string(fileDirPath))
		input = strings.ReplaceAll(input, repoBaseDirMacro, string(repoBaseDir))

		// Save back to original
		(*inputs)[index] = input
	}
}
