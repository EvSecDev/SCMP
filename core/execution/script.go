package execution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/deployment/host"
	"scmp/core/deployment/predeploy"
	"scmp/internal/config"
	"scmp/internal/crypto"
	"scmp/internal/fsops"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/secrets"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Run a script on host(s)
func runScript(ctx context.Context, scriptFile string, hosts string, remoteFilePath str.RemotePath) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSExec)

	// Not adhering to actual URI standards -- I just want file paths
	localScriptFilePath := strings.TrimPrefix(scriptFile, global.FileURIPrefix)

	// Check for ~/ and expand if required
	localScriptFilePath, err := fsops.ExpandHomeDirectory(localScriptFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve absolute path for '%s': %v\n", localScriptFilePath, err)
		os.Exit(1)
	}

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "File URI Path '%s'\n", localScriptFilePath)

	// Retrieve the file contents
	scriptFileBytes, err := os.ReadFile(localScriptFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read file: %v\n", err)
		os.Exit(1)
	}

	// Determine where to put the script on remote host
	if remoteFilePath == "" {
		// Default under /usr to avoid any /tmp restrictions if mounted noexec
		remoteFilePath = str.RemotePath("/usr/local/" + filepath.Base(localScriptFilePath))
	} else {
		// If user ever accidentally put CSV into this arg for execution, just use the first path
		remoteFilePaths := str.Split(remoteFilePath, ",")
		remoteFilePath = remoteFilePaths[0]
	}

	// Determine what interpreter to use for the script based on shebang '#!'
	var scriptInterpreter string
	scriptFileStr := string(scriptFileBytes)
	scriptLines := strings.Split(scriptFileStr, "\n")
	if strings.HasPrefix(scriptLines[0], "#!") {
		scriptInterpreter = strings.TrimSpace(scriptLines[0][2:])
	}

	// Hash local script contents
	scriptHash := crypto.SHA256Sum(scriptFileBytes)

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Local Script Hash '%s'\n", scriptHash)

	// If user only specified a single host, don't use threads
	if !strings.Contains(hosts, ",") {
		opts.MaxSSHConcurrency = 1
	}

	logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Executing script '%s' on host(s) %s\n", localScriptFilePath, hosts)

	// Semaphore to limit concurrency of host connections go routines
	semaphore := make(chan struct{}, opts.MaxSSHConcurrency)

	if opts.DryRunEnabled {
		// Notify user that program is in dry run mode
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Requested dry-run, outputting information collected for executions:\n")
	}

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

	if opts.WetRunEnabled {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Wet-run enabled. Connections and uploads will be tested but script will NOT be executed\n")
	}

	// Run script per host
	var wg sync.WaitGroup
	for endpointName := range cfg.HostInfo {
		// Only run against hosts specified
		if parsing.CheckForOverride(ctx, hosts, string(endpointName), cfg.HostInfo) {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// If user requested dry run - print host information and abort connections
		if opts.DryRunEnabled {
			predeploy.PrintHostInformation(ctx, cfg.HostInfo[endpointName])
			continue
		}

		proxyName := cfg.HostInfo[endpointName].Proxy

		// Upload and execute the script - disable concurrency if maxconns is 1
		wg.Add(1)
		if opts.MaxSSHConcurrency > 1 {
			go executeScriptOnHost(ctx, &wg, semaphore, cfg.HostInfo[endpointName], cfg.HostInfo[str.RepoRootDir(proxyName)], scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, false)
		} else {
			executeScriptOnHost(ctx, &wg, semaphore, cfg.HostInfo[endpointName], cfg.HostInfo[str.RepoRootDir(proxyName)], scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, true)
			if len(executionErrors) > 0 && !opts.ForceEnabled {
				// Execution error occurred, don't continue with other hosts
				break
			}
		}
	}
	wg.Wait()

	// Print out any errors
	if len(executionErrors) > 0 {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Errors:\n  %v\n", executionErrors)
	}

}

// Connect to a host, upload a script, execute script and print output
func executeScriptOnHost(ctx context.Context, wg *sync.WaitGroup, semaphore chan struct{}, hostInfo config.EndpointInfo, proxyInfo config.EndpointInfo, scriptInterpreter string, remoteFilePath str.RemotePath, scriptFileBytes []byte, scriptHash string, streamOutput bool) {
	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Save meta info for this host in a structure to easily pass around required pieces
	var hostMeta sshinternal.HostMeta
	hostMeta.Name = hostInfo.EndpointName
	hostMeta.Password = hostInfo.Password

	// Connect to the SSH server
	var err error
	var proxyClient *ssh.Client
	hostMeta.SSHClient, proxyClient, err = sshinternal.ConnectToSSH(ctx, hostInfo, proxyInfo)
	if err != nil {
		executionErrorsMutex.Lock()
		executionErrors += fmt.Sprintf("  Host '%s': %v\n", hostInfo.EndpointName, err)
		executionErrorsMutex.Unlock()
		return
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

	err = host.RemoteDeploymentPreparation(ctx, &hostMeta)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			executionErrorsMutex.Lock()
			executionErrors += fmt.Sprintf("remote system preparation failed: %v", err)
			executionErrorsMutex.Unlock()
			return
		}
		err = nil
	}
	defer host.CleanupRemote(ctx, hostMeta)

	// Run the script remotely
	var scriptOutput string
	if streamOutput {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host '%s':\n", hostInfo.EndpointName)
		_, err = sshinternal.ExecuteScript(ctx, hostMeta, scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, streamOutput)
	} else {
		scriptOutput, err = sshinternal.ExecuteScript(ctx, hostMeta, scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, streamOutput)
	}
	if err != nil {
		executionErrorsMutex.Lock()
		executionErrors += fmt.Sprintf("  Host '%s': %v\n", hostInfo.EndpointName, err)
		executionErrorsMutex.Unlock()
	}

	if scriptOutput != "" {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host '%s':\n%s\n", hostInfo.EndpointName, scriptOutput)
	} else if err == nil && !opts.WetRunEnabled {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "  Host '%s': Script Completed Successfully\n", hostInfo.EndpointName)
	}
}
