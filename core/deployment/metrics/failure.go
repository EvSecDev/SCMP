package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
)

func GetFailTrackerCommit(filePath string) (commitID string, prevDeploymentSummary Summary, err error) {

	lastSummary, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	err = json.Unmarshal(lastSummary, &prevDeploymentSummary)
	if err != nil {
		err = fmt.Errorf("error unmarshaling json: %w", err)
		return
	}

	if prevDeploymentSummary.CommitID == "" {
		err = fmt.Errorf("commitid missing from failtracker file")
		return
	}
	return
}

// Reads in last deployment summary and retrieves failed files and hosts for retry
func (deploymentSummary Summary) GetFailures(ctx context.Context, fileOverride string) (commitFiles map[str.LocalRepoPath]str.DeployAction, hostOverride string, err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	commitFiles = make(map[str.LocalRepoPath]str.DeployAction)

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Parsing last deployment failures\n")

	// Deployment override host selection by failed hosts
	var hostOverrideArray []str.RepoRootDir

	for _, hostReport := range deploymentSummary.Hosts {
		if hostReport.Name == "" {
			err = fmt.Errorf("hostname is empty: failtracker line: %v", hostReport)
			return
		}

		if hostReport.Status != "Failed" && hostReport.Status != "Partial" {
			continue
		}

		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Parsing failure for host %v\n", hostReport.Name)

		// Add host to override to isolate deployment to just the failed hosts
		hostOverrideArray = append(hostOverrideArray, hostReport.Name)

		for _, itemReport := range hostReport.Items {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "   Parsing failure for file %s\n", itemReport.Name)

			if itemReport.Status != "Failed" {
				continue
			}

			// Skip this file if not in override (if override was requested)
			skipFile := parsing.CheckForOverride(ctx, fileOverride, string(itemReport.Name), cfg.HostInfo)
			if skipFile {
				continue
			}

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File %s for redeployment\n", itemReport.Name)

			commitFiles[itemReport.Name] = itemReport.Action
		}
	}

	// Convert to standard format for override
	hostOverride = str.Join(hostOverrideArray, ",")

	return
}
