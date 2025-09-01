// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Global for script execution concurrency
var executionErrors string
var executionErrorsMutex sync.Mutex

func entryExec(commandname string, args []string) {
	var hostOverride string
	var remoteFileOverride string

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	setDeployConfArguments(commandFlags)
	commandFlags.StringVar(&hostOverride, "r", "", "Override remote hosts")
	commandFlags.StringVar(&hostOverride, "remote-hosts", "", "Override remote hosts")
	commandFlags.StringVar(&remoteFileOverride, "R", "", "Override remote file(s)")
	commandFlags.StringVar(&remoteFileOverride, "remote-files", "", "Override remote file(s)")
	commandFlags.BoolVar(&config.options.regexEnabled, "regex", false, "Enables regular expression parsing for file/host overrides")
	setSSHArguments(commandFlags)
	setGlobalArguments(commandFlags)

	commandFlags.Usage = func() {
		printHelpMenu(commandFlags, commandname, nil, "<remote command>", false)
	}
	if len(args) < 1 {
		printHelpMenu(commandFlags, commandname, nil, "<remote command>", false)
		os.Exit(1)
	}
	commandFlags.Parse(args[0:])

	executeCommands := strings.Join(commandFlags.Args(), " ")
	if executeCommands == "" {
		printHelpMenu(commandFlags, commandname, nil, "<remote command>", false)
		os.Exit(1)
	}

	err := config.extractOptions(config.filePath)
	logError("Error in controller configuration", err, true)

	// Pull contents of out file URIs
	hostOverride, err = retrieveURIFile(hostOverride)
	logError("Failed to parse remove-hosts URI", err, true)
	remoteFileOverride, err = retrieveURIFile(remoteFileOverride)
	logError("Failed to parse local-files URI", err, true)

	if strings.HasPrefix(executeCommands, "file:") {
		runScript(executeCommands, hostOverride, remoteFileOverride)
	} else if executeCommands != "" {
		runCmd(executeCommands, hostOverride)
	}
}

// Run a single adhoc command on requested hosts
func runCmd(command string, hosts string) {
	// Refused seeding without specific hosts specified
	if hosts == "" {
		logError("Argument error", fmt.Errorf("remote-hosts cannot be empty when running commands"), false)
	}

	var err error

	// Retrieve keys and passwords for any hosts that require it
	for endpointName := range config.hostInfo {
		// Only retrieve for hosts specified
		if checkForOverride(hosts, endpointName) {
			printMessage(verbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// Retrieve host secrets
		config.hostInfo[endpointName], err = retrieveHostSecrets(config.hostInfo[endpointName])
		logError("Error retrieving host secrets", err, true)

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := config.hostInfo[endpointName].proxy
		if proxyName != "" {
			config.hostInfo[proxyName], err = retrieveHostSecrets(config.hostInfo[proxyName])
			logError("Error retrieving proxy secrets", err, true)
		}
	}

	printMessage(verbosityStandard, "Executing command '%s' on host(s) '%s'\n", command, hosts)

	// Semaphore to limit concurrency of host connections go routines
	semaphore := make(chan struct{}, config.options.maxSSHConcurrency)

	// Loop hosts chosen by user and prepare relevant host information for deployment
	var wg sync.WaitGroup
	for endpointName := range config.hostInfo {
		skipHost := checkForOverride(hosts, endpointName)
		if skipHost {
			printMessage(verbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// If user requested dry run - print host information and abort connections
		if config.options.dryRunEnabled {
			printHostInformation(config.hostInfo[endpointName])
			continue
		}

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := config.hostInfo[endpointName].proxy
		if proxyName != "" {
			config.hostInfo[proxyName], err = retrieveHostSecrets(config.hostInfo[proxyName])
			logError("Error retrieving proxy secrets", err, true)
		}

		// Run the command
		wg.Add(1)
		if config.options.maxSSHConcurrency > 1 {
			go executeCommand(&wg, semaphore, config.hostInfo[endpointName], config.hostInfo[proxyName], command, false)
		} else {
			executeCommand(&wg, semaphore, config.hostInfo[endpointName], config.hostInfo[proxyName], command, true)
		}
	}
	wg.Wait()
}

func executeCommand(wg *sync.WaitGroup, semaphore chan struct{}, hostInfo EndpointInfo, proxyInfo EndpointInfo, command string, streamOutput bool) {
	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	// Connect to the SSH server
	client, proxyClient, err := connectToSSH(hostInfo, proxyInfo)
	logError("Failed to connect to host", err, false)
	if proxyClient != nil {
		defer proxyClient.Close()
	}
	defer client.Close()

	if config.options.wetRunEnabled {
		return
	}

	// Execute user command
	var cmdOutput string
	rawCmd := RemoteCommand{command, config.options.executionTimeout, streamOutput}
	if streamOutput {
		printMessage(verbosityStandard, "  Host '%s':\n", hostInfo.endpointName)
		_, err = rawCmd.SSHexec(client, config.options.runAsUser, config.options.disableSudo, hostInfo.password)
	} else {
		cmdOutput, err = rawCmd.SSHexec(client, config.options.runAsUser, config.options.disableSudo, hostInfo.password)
	}
	if err != nil {
		if config.options.forceEnabled {
			printMessage(verbosityStandard, "Error:  %v\n", err)
		} else {
			logError("Command Failed", err, false)
		}
	}

	if cmdOutput != "" {
		printMessage(verbosityStandard, "  Host '%s':\n%s\n", hostInfo.endpointName, cmdOutput)
	} else {
		printMessage(verbosityStandard, "  Host '%s': Command Completed Successfully\n\n", hostInfo.endpointName)
	}
}

// Run a script on host(s)
func runScript(scriptFile string, hosts string, remoteFilePath string) {
	// Not adhering to actual URI standards -- I just want file paths
	localScriptFilePath := strings.TrimPrefix(scriptFile, fileURIPrefix)

	// Check for ~/ and expand if required
	localScriptFilePath = expandHomeDirectory(localScriptFilePath)

	printMessage(verbosityFullData, "File URI Path '%s'\n", localScriptFilePath)

	// Retrieve the file contents
	scriptFileBytes, err := os.ReadFile(localScriptFilePath)
	logError("Failed to read file", err, false)

	// Determine where to put the script on remote host
	if remoteFilePath == "" {
		// Default under /usr to avoid any /tmp restrictions if mounted noexec
		remoteFilePath = "/usr/local/" + filepath.Base(localScriptFilePath)
	} else {
		// If user ever accidentally put CSV into this arg for execution, just use the first path
		remoteFilePaths := strings.Split(remoteFilePath, ",")
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
	scriptHash := SHA256Sum(scriptFileBytes)

	printMessage(verbosityFullData, "Local Script Hash '%s'\n", scriptHash)

	// If user only specified a single host, don't use threads
	if !strings.Contains(hosts, ",") {
		config.options.maxSSHConcurrency = 1
	}

	printMessage(verbosityStandard, "Executing script '%s' on host(s) %s\n", localScriptFilePath, hosts)

	// Semaphore to limit concurrency of host connections go routines
	semaphore := make(chan struct{}, config.options.maxSSHConcurrency)

	if config.options.dryRunEnabled {
		// Notify user that program is in dry run mode
		printMessage(verbosityProgress, "Requested dry-run, outputting information collected for executions:\n")
	}

	// Retrieve keys and passwords for any hosts that require it
	for endpointName := range config.hostInfo {
		// Only retrieve for hosts specified
		if checkForOverride(hosts, endpointName) {
			printMessage(verbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// Retrieve host secrets
		config.hostInfo[endpointName], err = retrieveHostSecrets(config.hostInfo[endpointName])
		logError("Error retrieving host secrets", err, true)

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := config.hostInfo[endpointName].proxy
		if proxyName != "" {
			config.hostInfo[proxyName], err = retrieveHostSecrets(config.hostInfo[proxyName])
			logError("Error retrieving proxy secrets", err, true)
		}
	}

	if config.options.wetRunEnabled {
		printMessage(verbosityStandard, "Wet-run enabled. Connections and uploads will be tested but script will NOT be executed\n")
	}

	// Run script per host
	var wg sync.WaitGroup
	for endpointName := range config.hostInfo {
		// Only run against hosts specified
		if checkForOverride(hosts, endpointName) {
			printMessage(verbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// If user requested dry run - print host information and abort connections
		if config.options.dryRunEnabled {
			printHostInformation(config.hostInfo[endpointName])
			continue
		}

		proxyName := config.hostInfo[endpointName].proxy

		// Upload and execute the script - disable concurrency if maxconns is 1
		wg.Add(1)
		if config.options.maxSSHConcurrency > 1 {
			go executeScriptOnHost(&wg, semaphore, config.hostInfo[endpointName], config.hostInfo[proxyName], scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, false)
		} else {
			executeScriptOnHost(&wg, semaphore, config.hostInfo[endpointName], config.hostInfo[proxyName], scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, true)
			if len(executionErrors) > 0 && !config.options.forceEnabled {
				// Execution error occurred, don't continue with other hosts
				break
			}
		}
	}
	wg.Wait()

	// Print out any errors
	if len(executionErrors) > 0 {
		printMessage(verbosityStandard, "Errors:\n  %v\n", executionErrors)
	}

}

// Connect to a host, upload a script, execute script and print output
func executeScriptOnHost(wg *sync.WaitGroup, semaphore chan struct{}, hostInfo EndpointInfo, proxyInfo EndpointInfo, scriptInterpreter string, remoteFilePath string, scriptFileBytes []byte, scriptHash string, streamOutput bool) {
	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	// Save meta info for this host in a structure to easily pass around required pieces
	var host HostMeta
	host.name = hostInfo.endpointName
	host.password = hostInfo.password

	// Connect to the SSH server
	var err error
	var proxyClient *ssh.Client
	host.sshClient, proxyClient, err = connectToSSH(hostInfo, proxyInfo)
	if err != nil {
		executionErrorsMutex.Lock()
		executionErrors += fmt.Sprintf("  Host '%s': %v\n", hostInfo.endpointName, err)
		executionErrorsMutex.Unlock()
		return
	}
	if proxyClient != nil {
		defer proxyClient.Close()
	}
	defer host.sshClient.Close()

	err = remoteDeploymentPreparation(&host)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "file exists") {
			executionErrorsMutex.Lock()
			executionErrors += fmt.Sprintf("remote system preparation failed: %v", err)
			executionErrorsMutex.Unlock()
			return
		}
		err = nil
	}
	defer cleanupRemote(host)

	// Run the script remotely
	var scriptOutput string
	if streamOutput {
		printMessage(verbosityStandard, "  Host '%s':\n", hostInfo.endpointName)
		_, err = executeScript(host, scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, streamOutput)
	} else {
		scriptOutput, err = executeScript(host, scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash, streamOutput)
	}
	if err != nil {
		executionErrorsMutex.Lock()
		executionErrors += fmt.Sprintf("  Host '%s': %v\n", hostInfo.endpointName, err)
		executionErrorsMutex.Unlock()
	}

	if scriptOutput != "" {
		printMessage(verbosityStandard, "  Host '%s':\n%s\n", hostInfo.endpointName, scriptOutput)
	} else if err == nil && !config.options.wetRunEnabled {
		printMessage(verbosityStandard, "  Host '%s': Script Completed Successfully\n", hostInfo.endpointName)
	}
}
