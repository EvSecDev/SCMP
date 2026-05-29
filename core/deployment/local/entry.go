// Package for pre-deployment preparation orchestration
package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"scmp/cli"
	"scmp/core/deployment"
	"scmp/core/deployment/host"
	"scmp/core/deployment/metrics"
	"scmp/core/deployment/predeploy"
	"scmp/core/deployment/repository"
	"scmp/internal/config"
	"scmp/internal/fsops"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/network"
	"scmp/internal/parsing"
	"scmp/internal/secrets"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"sync"
)

// Parses and prepares deployment information
func StartDeploy(ctx context.Context, deployMode string, commitID string, hostOverride string, fileOverride string) (rollbackCommit bool, err error) {
	// Retrieve required deployment options
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSDeploy)

	// Pull contents of out file URIs
	hostOverride, err = parsing.RetrieveURIFile(ctx, hostOverride)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("failed to parse remote-hosts URI: %w", err)
		return
	}
	fileOverride, err = parsing.RetrieveURIFile(ctx, fileOverride)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("failed to parse local-files URI: %w", err)
		return
	}

	_, err = gitinternal.RetrieveRepoPath(ctx)
	if err != nil {
		err = fmt.Errorf("repository error: %w", err)
		return
	}

	// Set path to failtracker file (in config directory)
	configDirectory := filepath.Dir(sshinternal.DefaultConfigPath)
	failTrackerFilePath := filepath.Join(configDirectory, deployment.FailTrackerFile)
	failTrackerFilePath, err = fsops.ExpandHomeDirectory(failTrackerFilePath)
	if err != nil {
		err = fmt.Errorf("failed to find home directory for '%s': %w", failTrackerFilePath, err)
		return
	}

	// Override commitID with one from failtracker if redeploy requested
	var lastDeploymentSummary metrics.Summary
	if deployMode == deployment.ModeRetry {
		commitID, lastDeploymentSummary, err = metrics.GetFailTrackerCommit(failTrackerFilePath)
		if err != nil {
			rollbackCommit = true
			err = fmt.Errorf("failed to extract commitID/failures from failtracker file: %w", err)
			return
		}
	}

	// Open repo and get details - using HEAD commit if commitID is empty
	// Pass by reference to ensure commitID can be used later if user did not specify one
	tree, commit, err := gitinternal.GetCommit(ctx, &commitID)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("error retrieving commit details: %w", err)
		return
	}

	var commitFiles map[str.LocalRepoPath]str.DeployAction

	switch deployMode {
	case deployment.ModeDiff:
		changedFiles, lerr := repository.GetChangedFiles(ctx, commit)
		if lerr != nil {
			rollbackCommit = true
			err = fmt.Errorf("failed to retrieve changed files: %w", lerr)
			return
		}

		commitFiles = repository.ParseChangedFiles(ctx, changedFiles, fileOverride)
	case deployment.ModeAll:
		commitFiles, err = repository.GetRepoFiles(ctx, tree, fileOverride)
	case deployment.ModeRetry:
		commitFiles, hostOverride, err = lastDeploymentSummary.GetFailures(ctx, fileOverride)
	default:
		err = fmt.Errorf("unknown deployment mode: mode must be one of '%v'", cli.GetImmediateChildren(cli.GetCLICmds(), "deploy"))
		return
	}

	if err != nil {
		err = fmt.Errorf("failed to retrieve files: %w", err)
		return
	}

	if len(commitFiles) == 0 {
		// Non-error - can happen under normal operations: When committing files outside of host directories
		logctx.LogStdInfo(ctx, "No files available for deployment.\n")
		return
	}

	allHostsFiles, universalFiles, err := repository.ParseAllRepoFiles(ctx, tree)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("failed to track files by host/universal directory: %w", err)
		return
	}

	deniedUniversalFiles := predeploy.MapDeniedUniversalFiles(ctx, allHostsFiles, universalFiles)

	allDeploymentHosts, allDeploymentFiles, hostDeploymentFiles := predeploy.FilterHostsAndFiles(ctx, cfg.HostInfo, deniedUniversalFiles, commitFiles, hostOverride)
	if len(allDeploymentFiles) == 0 || len(allDeploymentHosts) == 0 {
		// Non-error - can happen under normal operations: if user specifies change deploy mode with a host that didn't have any changes in the specified commit
		logctx.LogStdInfo(ctx, "No deployment files for available hosts.\n")
		return
	}

	rawFileContent, err := predeploy.LoadGitFileContent(ctx, allDeploymentFiles, tree)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("error loading files: %w", err)
		return
	}

	deployFiles, err := predeploy.ParseFileContent(ctx, allDeploymentFiles, rawFileContent)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("error parsing loaded files: %w", err)
		return
	}

	hostFiles, err := predeploy.SortFiles(ctx, hostDeploymentFiles, deployFiles)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("failed sorting deployment files: %w", err)
		return
	}

	err = network.LocalSystemChecks(ctx)
	if err != nil {
		rollbackCommit = true
		err = fmt.Errorf("error in local system checks: %w", err)
		return
	}

	logctx.LogStdInfo(ctx, "Deploying %d item(s) to %d host(s)\n", deployFiles.Count(), len(allDeploymentHosts))

	if opts.DryRunEnabled {
		predeploy.PrintDeploymentInformation(ctx, deployFiles, allDeploymentHosts, hostFiles)
		return
	}

	select {
	case <-ctx.Done():
		err = fmt.Errorf("immediate stop requested before deployment start")
		return
	default:
	}

	// Retrieve keys and passwords for any hosts that require it
	for _, endpointName := range allDeploymentHosts {
		// Retrieve host secrets
		cfg.HostInfo[endpointName], err = secrets.GetHostValues(ctx, cfg.HostInfo[endpointName])
		if err != nil {
			rollbackCommit = true
			err = fmt.Errorf("error retrieving host secrets: %w", err)
			return
		}

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := cfg.HostInfo[endpointName].Proxy
		if proxyName != "" {
			cfg.HostInfo[str.RepoRootDir(proxyName)], err = secrets.GetHostValues(ctx, cfg.HostInfo[str.RepoRootDir(proxyName)])
			if err != nil {
				rollbackCommit = true
				err = fmt.Errorf("error retrieving proxy secrets: %w", err)
				return
			}
		}
	}

	// Metric collection
	deployMetrics := metrics.New()

	// Start SSH Deployments
	// All failures and errors from here on are soft stops - program will finish, errors are tracked within deployment metrics, git commit will NOT be rolled back
	var wg sync.WaitGroup
	connLimiter := make(chan struct{}, opts.MaxSSHConcurrency)
	for _, endpointName := range allDeploymentHosts {
		deployer := host.New(&wg,
			connLimiter,
			cfg.HostInfo[endpointName],
			cfg.HostInfo[str.RepoRootDir(cfg.HostInfo[endpointName].Proxy)],
			deployMetrics,
			opts.MaxDeployConcurrency,
		)

		wg.Add(1)
		if opts.MaxSSHConcurrency > 1 {
			go deployer.Deploy(ctx, hostFiles[endpointName])
		} else {
			// Max conns of <=1 disables using go routine
			deployer.Deploy(ctx, hostFiles[endpointName])

			// Don't continue to the next host on errors
			if deployMetrics.HostHasError(endpointName) {
				break
			}
		}
	}
	wg.Wait()

	deployMetrics.Stop()
	deploymentSummary := deployMetrics.CreateReport(commitID)

	if opts.WetRunEnabled {
		logctx.LogStdInfo(ctx, "Wet-run enabled. No mutating actions taken, theoretical deployment summary:\n")
	}

	// Show user what was done during deployment
	if opts.DetailedSummaryRequested {
		// Detailed Summary
		var deploymentSummaryJSON []byte
		deploymentSummaryJSON, err = json.MarshalIndent(deploymentSummary, "", " ")
		if err != nil {
			err = fmt.Errorf("failed to marshal detailed deployment summary JSON: %w", err)
			return
		}

		logctx.LogStdInfo(ctx, "%s\n", string(deploymentSummaryJSON))
	} else {
		logctx.LogStdInfo(ctx,
			"Status: %s. Deployed %d item(s) (%s) to %d host(s). Deployment took %s\n",
			deploymentSummary.Status,
			deploymentSummary.Counters.CompletedItems,
			deploymentSummary.TransferredData,
			deploymentSummary.Counters.CompletedHosts,
			deploymentSummary.ElapsedTime,
		)

		err = deploymentSummary.PrintFailures(ctx)
		if err != nil {
			err = fmt.Errorf("error in printing deployment failures: %w", err)
			return
		}
	}

	err = deploymentSummary.SaveReport(ctx, failTrackerFilePath)
	if err != nil {
		err = fmt.Errorf("error in recording deployment failures: %w", err)
		return
	}

	if !deployMetrics.AnyErrorsPresent() {
		// Remove fail tracker file after successful redeployment - best effort
		err = os.Remove(failTrackerFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				// No warning if the file doesn't exist
				err = nil
			} else {
				err = fmt.Errorf("failed removing failtracker file: %w", err)
				return
			}
		}
	}
	return
}
