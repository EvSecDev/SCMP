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

		// Quick guard against any unforeseen consequences
		if len(hostFiles.GetUnorderedList()) > 0 && len(hostFiles.Groups) == 0 {
			err = fmt.Errorf("something went wrong: dependency tree sorting resulted in no files")
			return
		} else if len(hostFiles.GetUnorderedList()) > 0 && len(hostFiles.Groups[0].GetOrderedList()) == 0 {
			err = fmt.Errorf("something went wrong: dependency tree sorting resulted in an empty tree with no files")
			return
		}
	}
	return
}
