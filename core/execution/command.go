package execution

import (
	"context"
	"fmt"
	"os"
	"scmp/core/deployment/predeploy"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/secrets"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"sync"
)

// Global for script execution concurrency
var executionErrors string
var executionErrorsMutex sync.Mutex

// Run a single adhoc command on requested hosts
func runCmd(ctx context.Context, command string, hosts string) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSExec)

	// Refused seeding without specific hosts specified
	if hosts == "" {
		fmt.Fprintf(os.Stderr, "Argument error: remote-hosts cannot be empty when running commands")
		os.Exit(1)
	}

	var err error

	// Retrieve keys and passwords for any hosts that require it
	for endpointName := range cfg.HostInfo {
		// Only retrieve for hosts specified
		if parsing.CheckForOverride(ctx, hosts, string(endpointName), cfg.HostInfo) {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// Retrieve host secrets
		cfg.HostInfo[endpointName], err = secrets.GetHostValues(ctx, cfg.HostInfo[endpointName])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error retrieving host secrets: %v\n", err)
			os.Exit(1)
		}

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := cfg.HostInfo[endpointName].Proxy
		if proxyName != "" {
			cfg.HostInfo[str.RepoRootDir(proxyName)], err = secrets.GetHostValues(ctx, cfg.HostInfo[str.RepoRootDir(proxyName)])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error retrieving proxy secrets: %v\n", err)
				os.Exit(1)
			}
		}
	}

	logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Executing command '%s' on host(s) '%s'\n", command, hosts)

	// Semaphore to limit concurrency of host connections go routines
	semaphore := make(chan struct{}, opts.MaxSSHConcurrency)

	// Loop hosts chosen by user and prepare relevant host information for deployment
	var wg sync.WaitGroup
	for endpointName := range cfg.HostInfo {
		skipHost := parsing.CheckForOverride(ctx, hosts, string(endpointName), cfg.HostInfo)
		if skipHost {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// If user requested dry run - print host information and abort connections
		if opts.DryRunEnabled {
			predeploy.PrintHostInformation(ctx, cfg.HostInfo[endpointName])
			continue
		}

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := cfg.HostInfo[endpointName].Proxy
		if proxyName != "" {
			cfg.HostInfo[str.RepoRootDir(proxyName)], err = secrets.GetHostValues(ctx, cfg.HostInfo[str.RepoRootDir(proxyName)])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error retrieving proxy secrets: %v\n", err)
				os.Exit(1)
			}
		}

		// Run the command
		wg.Add(1)
		if opts.MaxSSHConcurrency > 1 {
			go executeCommand(ctx, &wg, semaphore, cfg.HostInfo[endpointName], cfg.HostInfo[str.RepoRootDir(proxyName)], command, false)
		} else {
			executeCommand(ctx, &wg, semaphore, cfg.HostInfo[endpointName], cfg.HostInfo[str.RepoRootDir(proxyName)], command, true)
		}
	}
	wg.Wait()
}

func executeCommand(ctx context.Context, wg *sync.WaitGroup, semaphore chan struct{}, hostInfo config.EndpointInfo, proxyInfo config.EndpointInfo, command string, streamOutput bool) {
	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Connect to the SSH server
	client, proxyClient, err := sshinternal.ConnectToSSH(ctx, hostInfo, proxyInfo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to host: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if proxyClient != nil {
			lerr := proxyClient.Close()
			if err == nil && lerr != nil {
				err = fmt.Errorf("proxy close: %w", lerr)
			}
		}
		lerr := client.Close()
		if err == nil && lerr != nil {
			err = fmt.Errorf("client close: %w", lerr)
		}
	}()

	if opts.WetRunEnabled {
		return
	}

	// Execute user command
	var cmdOutput string
	rawCmd := sshinternal.RemoteCommand{
		Raw:          command,
		RunAsUser:    opts.RunAsUser,
		DisableSudo:  opts.DisableSudo,
		Timeout:      opts.ExecutionTimeout,
		StreamStdout: streamOutput,
	}
	if streamOutput {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host '%s':\n", hostInfo.EndpointName)
		_, err = rawCmd.SSHexec(ctx, client, hostInfo.Password)
	} else {
		cmdOutput, err = rawCmd.SSHexec(ctx, client, hostInfo.Password)
	}
	if err != nil {
		if opts.ForceEnabled {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, " %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Command Failed: %v\n", err)
			os.Exit(1)
		}
	}

	if cmdOutput != "" {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host '%s':\n%s\n", hostInfo.EndpointName, cmdOutput)
	} else {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host '%s': Command Completed Successfully\n\n", hostInfo.EndpointName)
	}
}
