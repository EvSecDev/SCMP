package predeploy

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/internal/logctx"
	"scmp/internal/str"
)

// Creates a host files map that contains copies of the global data for per-host/per-file contextual mutation
func GroupByHost(ctx context.Context, globalFiles *deployment.AllFiles, hostDeploymentFiles map[str.RepoRootDir][]str.LocalRepoPath) (allHostFiles map[str.RepoRootDir]*deployment.HostFiles, err error) {
	allHostFiles = make(map[str.RepoRootDir]*deployment.HostFiles)

	ctx = logctx.AppendCtxTag(ctx, logctx.NSGrouping)

	for host, fileList := range hostDeploymentFiles {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Host: %s: Creating host map of %d files\n", host, len(fileList))

		var hostFiles *deployment.HostFiles
		hostFiles, err = deployment.NewHostFiles()
		if err != nil {
			return
		}
		allHostFiles[host] = hostFiles

		// Copy data to host-owned store so it can modify it freely
		hostFiles.CopyGlobalFiles(fileList, globalFiles)

		if len(fileList) > 0 && len(hostFiles.GetUnorderedList()) == 0 {
			err = fmt.Errorf("copied host file map is empty")
			return
		}
	}
	return
}

// Takes the per-host file object and creates ordered (dependency resolved) and grouped deployment list inside HostFiles object
func SortFiles(ctx context.Context, allHostFiles map[str.RepoRootDir]*deployment.HostFiles) (err error) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSParsing)

	for host, hostFiles := range allHostFiles {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Host: %s: Handling dependencies\n", host)

		// Reorder deployment list into independent trees and by dependencies
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Reordering files based on inter-file dependencies\n")
		var depTrees [][]str.LocalRepoPath
		depTrees, err = HandleFileDependencies(hostFiles.GetUnorderedList(), hostFiles)
		if err != nil {
			return
		}

		// Merge dependency trees to ensure similar reloads/reload groups get deployed in the same thread
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Merging dependency trees based on reload groups/commands\n")
		depTrees = MergeDepTrees(depTrees, hostFiles)

		// Identify reload groups by command and similar commands - used to coordinate when to reload during deployment
		for _, depTree := range depTrees {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Grouping config files by reload commands\n")
			independentDeploymentList := CreateReloadGroups(depTree, hostFiles)

			hostFiles.Groups = append(hostFiles.Groups, independentDeploymentList)
		}

		// Group and set post-installation commands per reload group
		hostFiles.InitPostInstallCmdSet()

		// Quick guard against any unforeseen consequences
		if len(hostFiles.GetUnorderedList()) > 0 && len(hostFiles.Groups) == 0 {
			err = fmt.Errorf("something went wrong: dependency tree sorting resulted in no files")
			return
		} else if len(hostFiles.GetUnorderedList()) > 0 && len(hostFiles.Groups[0].GetOrderedList()) == 0 {
			err = fmt.Errorf("something went wrong: dependency tree sorting resulted in an empty tree with no files")
			return
		}

		err = removeRedundantDeletions(ctx, hostFiles)
		if err != nil {
			err = fmt.Errorf("host %s: %w", host, err)
			return
		}
	}
	return
}

// Removes any deletions when the same (target) file path is being created in the same deployment.
// Prevents potential issues when a file is moved from a host-specific directory to a Universal directory (would cause delete then create).
func removeRedundantDeletions(ctx context.Context, hostFiles *deployment.HostFiles) (err error) {
	type dedupTracker struct {
		repoPaths           map[str.LocalRepoPath]struct{}
		seenTargetPathCount int
	}

	if hostFiles == nil {
		return
	}

	// Count up seen target paths
	seenTargetPaths := make(map[str.RemotePath]dedupTracker)
	for _, path := range hostFiles.GetUnorderedList() {
		fileInfo := hostFiles.GetFileInfo(path)

		existingInfo, seen := seenTargetPaths[fileInfo.TargetFilePath]
		if !seen {
			seenTargetPaths[fileInfo.TargetFilePath] = dedupTracker{
				repoPaths: map[str.LocalRepoPath]struct{}{
					path: {},
				},
				seenTargetPathCount: 1,
			}
		} else {
			existingInfo.repoPaths[path] = struct{}{}
			existingInfo.seenTargetPathCount++
			seenTargetPaths[fileInfo.TargetFilePath] = existingInfo

			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
				"target path '%s' is referenced by repository paths: %#v\n",
				fileInfo.TargetFilePath, existingInfo.repoPaths)
		}
	}

	for _, tracker := range seenTargetPaths {
		if tracker.seenTargetPathCount < 2 {
			// Target file path is referenced by more than one local repository path
			continue
		}

		for repoPath := range tracker.repoPaths {
			info := hostFiles.GetFileInfo(repoPath)

			// Gating redundancy removal for duplicate+delete action
			if info.Action == deployment.ActionDelete {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
					"removing repository path '%s' from deployment: target path '%s' was found marked as create and delete (defaulting to create)\n",
					repoPath, info.Action)

				err = hostFiles.PurgePath(repoPath)
				if err != nil {
					return
				}
			}
		}
	}
	return
}
