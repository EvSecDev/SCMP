package repository

import (
	"context"
	"os"
	"scmp/core/deployment"
	"scmp/core/drn"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

// Ensures files in the new commit are valid
// Invalid files include
//
//	non-existent
//	unsupported file type (device, socket, pipe, ect)
//	any files in the root of the repository
//	dirs present in global ignoredirectories array
//	dirs that do not have a match in the controllers config
func fileIsValid(ctx context.Context, path str.LocalRepoPath, mode string) (valid bool) {
	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Validating file %s\n", path)

	// Retrieve the type for this file
	fileType := parsing.DetermineFileType(mode)
	if fileType == "unsupported" {
		return
	}

	// File exists, but no path
	if path == "" {
		return
	}

	// Ensure path conforms to SCMP directory structure
	if repoFileIsNotValid(ctx, path) {
		return
	}

	// File is valid
	valid = true
	return
}

// Checks to ensure a given repository relative file path is:
//  1. A top-level directory name that is a valid host.Name as in DeployerEndpoints
//  2. A top-level directory name that is the universal config directory
//  3. A top-level directory name that is the a valid universal config group as in UniversalGroups
//  4. A file inside any directory (i.e. not a file just in root of repo)
//  5. A file not inside any top level directory with prefix _ (excluding DRN)
func repoFileIsNotValid(ctx context.Context, repoPath str.LocalRepoPath) (fileIsNotValid bool) {
	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	ctx = logctx.AppendCtxTag(ctx, logctx.NSValidation)

	// DRN config directory is always valid at this point (validated later)
	if strings.HasPrefix(string(repoPath), drn.ExternalVariableDirectory) {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File is in DRN config directory, valid\n")
		return
	}

	// Always ignore files in root of repository
	if !strings.ContainsRune(string(repoPath), os.PathSeparator) {
		fileIsNotValid = true
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File is in root of repo, skipping\n")
		return
	}

	// Always ignore (files under) directories with underscore prefix
	if str.HasPrefix(repoPath, deployment.IgnoreDirectoryPrefix) {
		fileIsNotValid = true
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File is in an ignore directory, skipping\n")
		return
	}

	// Get top-level directory name
	fileDirNames := strings.SplitN(string(repoPath), string(os.PathSeparator), 2)
	topLevelDir := str.RepoRootDir(fileDirNames[0])

	// Ensure directory name is valid against config options
	for configHost := range config.HostInfo {
		// file top-level dir is a valid host or the universal directory
		if topLevelDir == configHost || topLevelDir == config.UniversalDirectory {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File is valid (Dir matches Hostname or is Universal Dir)\n")
			fileIsNotValid = false
			return
		}
	}
	_, fileIsInUniversalGroup := config.AllUniversalGroups[topLevelDir]
	if fileIsInUniversalGroup {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File is valid (Dir matches a Universal Group Dir)\n")
		fileIsNotValid = false
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File is not under a valid host directory or a universal directory, skipping\n")
	fileIsNotValid = true
	return
}
