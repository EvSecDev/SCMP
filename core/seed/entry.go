// Package for retrieving and formatting remote files into local repository (bootstrapping)
package seed

import (
	"context"
	"fmt"
	"os"
	"scmp/core/deployment/host"
	"scmp/core/deployment/predeploy"
	"scmp/internal/config"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/network"
	"scmp/internal/parsing"
	"scmp/internal/secrets"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Entry point for user to select remote files to download and format into local repository
func SeedRepositoryFiles(ctx context.Context, hostOverride string, remoteFileOverride string) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	defer func() {
		if fatalError := recover(); fatalError != nil {
			fmt.Fprintf(os.Stderr, "Controller panic while seeding repository files: %v\n", fatalError)
			os.Exit(1)
		}
	}()

	// Pull contents of out file URIs
	hostOverride, err := parsing.RetrieveURIFile(ctx, hostOverride)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse remove-hosts URI: %v\n", err)
		os.Exit(1)
	}
	remoteFileOverride, err = parsing.RetrieveURIFile(ctx, remoteFileOverride)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse local-files URI: %v\n", err)
		os.Exit(1)
	}

	err = network.LocalSystemChecks(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in system checks: %v\n", err)
		os.Exit(1)
	}

	_, err = gitinternal.RetrieveRepoPath(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Repository Error: %v\n", err)
		os.Exit(1)
	}

	if opts.DryRunEnabled {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Requested dry-run, aborting deployment\n")
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Outputting information collected for deployment:\n")
	}

	// Loop hosts chosen by user and prepare relevant host information for deployment
	for endpointName, hostInfo := range cfg.HostInfo {
		skipHost := parsing.CheckForOverride(ctx, hostOverride, string(endpointName), cfg.HostInfo)
		if skipHost {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		cfg.HostInfo[endpointName], err = secrets.GetHostValues(ctx, cfg.HostInfo[endpointName])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error retrieving host secrets: %v\n", err)
			os.Exit(1)
		}

		proxyName := cfg.HostInfo[endpointName].Proxy
		if proxyName != "" {
			cfg.HostInfo[str.RepoRootDir(proxyName)], err = secrets.GetHostValues(ctx, cfg.HostInfo[str.RepoRootDir(proxyName)])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error retrieving proxy secrets: %v\n", err)
				os.Exit(1)
			}
		}

		// Retrieve most current global host config
		hostInfo = cfg.HostInfo[endpointName]
		proxyInfo := cfg.HostInfo[str.RepoRootDir(cfg.HostInfo[endpointName].Proxy)]

		// If user requested dry run - print host information and abort connections
		if opts.DryRunEnabled {
			predeploy.PrintHostInformation(ctx, hostInfo)
			continue
		}

		var hostMeta sshinternal.HostMeta
		hostMeta.Name = hostInfo.EndpointName
		hostMeta.Password = hostInfo.Password

		var proxyClient *ssh.Client
		hostMeta.SSHClient, proxyClient, err = sshinternal.ConnectToSSH(ctx, hostInfo, proxyInfo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed connect to SSH server: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if proxyClient != nil {
				lerr := proxyClient.Close()
				if err == nil && lerr != nil {
					err = fmt.Errorf("proxy close: %w", lerr)
				}
			}
			lerr := hostMeta.SSHClient.Close()
			if err == nil && lerr != nil {
				err = fmt.Errorf("client close: %w", lerr)
			}
		}()

		var selectedFiles []string
		if remoteFileOverride == "" {
			selectedFiles, err = interactiveSelection(ctx, hostMeta)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error retrieving remote file list: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Set user choices directly
			selectedFiles = strings.Split(remoteFileOverride, ",")
		}

		err = host.RemoteDeploymentPreparation(ctx, &hostMeta)
		if err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
				fmt.Fprintf(os.Stderr, "Failed to conduct remote system preparations: %v\n", err)
				os.Exit(1)
			}
			err = nil
		}

		// File for transfers
		hostMeta.TransferBufferDir = hostMeta.TransferBufferDir + "/transfer"

		err = sshinternal.SCPUpload(ctx, hostMeta.SSHClient, []byte{12}, hostMeta.TransferBufferDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize buffer file on remote host %s: %v\n", endpointName, err)
			os.Exit(1)
		}

		optCache := &RepoUserChoiceCache{}
		optCache.ReloadCmd = make(map[string][]string)
		optCache.ReloadCnt = make(map[string]int)
		for _, targetFilePath := range selectedFiles {
			err = handleSelectedFile(ctx, targetFilePath, hostMeta, optCache)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error seeding repository: %v\n", err)
				os.Exit(1)
			}
		}

		// Do any remote cleanups are required (non-fatal)
		hostMeta.TransferBufferDir = str.FilePathDir(hostMeta.TransferBufferDir) // remove transfer file from path for cleanup
		host.CleanupRemote(ctx, hostMeta)
	}
}
