package predeploy

import (
	"context"
	"encoding/base64"
	"os"
	"scmp/core/deployment"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

// Record universal files that are NOT to be used for each host (host has an override file)
func MapDeniedUniversalFiles(ctx context.Context, allHostsFiles map[str.RepoRootDir]map[str.RemotePath]struct{}, universalFiles map[str.RepoRootDir]map[str.RemotePath]struct{}) (deniedUniversalFiles map[str.RepoRootDir]map[str.LocalRepoPath]struct{}) {
	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	// Initialize map
	deniedUniversalFiles = make(map[str.RepoRootDir]map[str.LocalRepoPath]struct{})

	// Created denied map for each host in config
	for endpointName := range config.HostInfo {
		// Initialize inner map
		deniedUniversalFiles[endpointName] = make(map[str.LocalRepoPath]struct{})

		// Find overlaps between group files and host files - record overlapping group files in denied map
		for groupName, groupFiles := range universalFiles {
			// Skip groups not applicable to this host
			_, hostIsInFilesUniversalGroup := config.HostInfo[endpointName].UniversalGroups[groupName]
			if !hostIsInFilesUniversalGroup && groupName != config.UniversalDirectory {
				continue
			}

			// Find overlap files
			for groupFile := range groupFiles {
				_, hostHasUniversalOverride := allHostsFiles[endpointName][groupFile]
				if hostHasUniversalOverride {
					// Host has a file path that is also present in the group universal dir
					// Should never deploy group universal files if host has an identical file path
					deniedFilePath := str.FilePathJoin(str.LocalRepoPath(groupName), str.LocalRepoPath(groupFile))
					deniedUniversalFiles[endpointName][deniedFilePath] = struct{}{}
				}
			}
		}
	}

	return
}

// Uses host list and deployment files to create list of files and hosts specific to deployment
// Also deduplicates host and universal to ensure host override files don't get clobbered
func FilterHostsAndFiles(ctx context.Context, hostList map[str.RepoRootDir]config.EndpointInfo, deniedUniversalFiles map[str.RepoRootDir]map[str.LocalRepoPath]struct{}, commitFiles map[str.LocalRepoPath]str.DeployAction, hostOverride string) (allDeploymentHosts []str.RepoRootDir, allDeploymentFiles map[str.LocalRepoPath]str.DeployAction, hostDeploymentFiles map[str.RepoRootDir][]str.LocalRepoPath) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSParsing)

	// Show progress to user
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Filtering deployment hosts... \n")

	// Initialize maps for deployment info
	allDeploymentFiles = make(map[str.LocalRepoPath]str.DeployAction)   // Map of all (filtered) deployment files and their associated actions
	hostDeploymentFiles = make(map[str.RepoRootDir][]str.LocalRepoPath) // Map of deployment hosts and their list of files

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Creating files per host and all deployment files maps\n")

	// Loop hosts in config and prepare endpoint information and relevant configs for deployment
	for endpointName, hostInfo := range hostList {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Host %s: Filtering files...\n", endpointName)
		// Skip this host if not in override (if override was requested)
		skipHost := parsing.CheckForOverride(ctx, hostOverride, string(endpointName), hostList)
		if skipHost {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "    Host not desired\n")
			continue
		}

		// Get Denied universal files for this host
		hostsDeniedUniversalFiles := deniedUniversalFiles[endpointName]

		// Filter committed files to their specific host and deduplicate against universal directory
		for commitFile, commitFileAction := range commitFiles {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    Filtering file %s\n", commitFile)

			// Split out the host part of the committed file path
			hostAndPath := strings.SplitN(string(commitFile), string(os.PathSeparator), 2)
			commitHost := str.RepoRootDir(hostAndPath[0])

			// Skip files not relevant to this host (either file is local to host, in global universal dir, or in host group universal)
			_, hostIsInFilesUniversalGroup := hostInfo.UniversalGroups[commitHost]
			if commitHost != endpointName && !hostIsInFilesUniversalGroup {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "        File not for this host/host's universal group and not universal \n")
				continue
			}

			// Skip if commitFile is a universal file that is not allowed for this host
			_, fileIsDenied := hostsDeniedUniversalFiles[commitFile]
			if fileIsDenied {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "        File is universal and host has non-universal identical file\n")
				continue
			}

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "        Selected\n")

			// Add file to the host-specific file list and the all-host deployment file map
			allDeploymentFiles[commitFile] = commitFileAction
			hostDeploymentFiles[endpointName] = append(hostDeploymentFiles[endpointName], commitFile)
		}

		// Skip this host if no files to deploy
		if len(hostDeploymentFiles[endpointName]) == 0 {
			continue
		}

		// Track hosts for deployment
		allDeploymentHosts = append(allDeploymentHosts, endpointName)
	}

	return
}

func CreateReloadGroups(fileList []str.LocalRepoPath, deployFiles *deployment.HostFiles) (groupedDeployList *deployment.FileGroup) {
	groupedDeployList = deployment.NewFileGroup(fileList)

	noNamedGroups := make(map[str.ReloadID][]str.LocalRepoPath) // Temp hold any files that aren't part of explicit named group
	reloadIDtoGroupName := make(map[str.ReloadID]str.ReloadID)  // Lookup if a reload array actually should be in a named group

	// Group files with named groups
	for _, file := range fileList {
		info := deployFiles.GetFileInfo(file)

		// No processing for files without reloads or custom group names
		if !info.ReloadRequired && info.ReloadGroup == "" {
			continue
		}

		var reloadID str.ReloadID
		if len(info.Reload) > 0 {
			cmdList := str.Join(info.Reload, "|")
			reloadID = str.ReloadID(base64.URLEncoding.EncodeToString([]byte(cmdList)))
		}

		// Group custom names - once encountered, no need to group by identical commands
		fileReloadGroupName := info.ReloadGroup
		if fileReloadGroupName != "" {
			if reloadID != "" {
				reloadIDtoGroupName[reloadID] = fileReloadGroupName
			}

			groupedDeployList.AppendFileToReloadID(fileReloadGroupName, file)
			continue
		}

		// Put reloads without named groups into temp map for later sorting
		if reloadID != "" {
			noNamedGroups[reloadID] = append(noNamedGroups[reloadID], file)
		}
	}

	// Put reloads into custom groups when custom groups have an identical set
	for reloadID, reloadFiles := range noNamedGroups {
		for _, reloadFile := range reloadFiles {
			groupName, reloadShouldBeInGroup := reloadIDtoGroupName[reloadID]
			if reloadShouldBeInGroup {
				groupedDeployList.AppendFileToReloadID(groupName, reloadFile)
			} else {
				groupedDeployList.AppendFileToReloadID(reloadID, reloadFile)
			}
		}
	}
	groupedDeployList.OrderReloadIDFiles() // Stabilize group file slices

	// Create file to reload id mapping (reverse map)
	groupedDeployList.InitFiletoReloadID()

	// Create command array per group (deduped)
	for _, reloadID := range groupedDeployList.GetReloadIDs() {
		// Duplicate tracker
		seen := make(map[string]bool)

		// Use the main deployment list to preserve order of reload commands
		for _, file := range fileList {
			fileReloadID, _ := groupedDeployList.GetFileReloadID(file)
			if fileReloadID != reloadID {
				continue
			}

			info := deployFiles.GetFileInfo(file)

			for _, fileReloadCmd := range info.Reload {
				// Skip duplicates after the first occurrence - even between files
				if seen[fileReloadCmd] {
					continue
				}

				// Add files command to the groups command list
				groupedDeployList.AppendCmdToReloadID(reloadID, file, fileReloadCmd)

				// Mark so it doesn't get added again
				seen[fileReloadCmd] = true
			}
		}
	}

	// Count files per reload group
	groupedDeployList.RecordReloadIDFileCount()
	return
}
