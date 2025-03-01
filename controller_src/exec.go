// controller
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Global for script execution concurrency
var executionErrors string
var executionErrorsMutex sync.Mutex

// Run a single adhoc command on requested hosts
func runCmd(command string, hosts string) {
	// Refused seeding without specific hosts specified
	if hosts == "" {
		logError("Argument error", fmt.Errorf("remote-hosts cannot be empty when running commands"), false)
	}

	printMessage(VerbosityStandard, "Executing command '%s' on hosts '%s'\n", command, hosts)

	// Loop hosts chosen by user and prepare relevant host information for deployment
	for endpointName := range config.HostInfo {
		SkipHost := checkForOverride(hosts, endpointName)
		if SkipHost {
			printMessage(VerbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// Retrieve host secrets (keys,passwords)
		err := retrieveHostSecrets(endpointName)
		logError("Failed to retrieve host secrets", err, false)

		// Retrieve most current global host config
		hostInfo := config.HostInfo[endpointName]

		// If user requested dry run - print host information and abort connections
		if dryRunRequested {
			printHostInformation(hostInfo)
			continue
		}

		// Run the command
		executeCommand(hostInfo, command)
	}
}

func executeCommand(hostInfo EndpointInfo, command string) {
	// Connect to the SSH server
	client, err := connectToSSH(hostInfo.Endpoint, hostInfo.EndpointUser, hostInfo.Password, hostInfo.PrivateKey, hostInfo.KeyAlgo)
	logError("Failed to connect to host", err, false)
	defer client.Close()

	// Execute user command
	commandOutput, err := RunSSHCommand(client, command, "", config.DisableSudo, hostInfo.Password, 900)
	logError("Command Failed", err, false)

	// Show command output
	printMessage(VerbosityStandard, "  Host '%s' Command Output: %s\n", hostInfo.EndpointName, commandOutput)
}

// Run a script on host(s)
func runScript(scriptFile string, hosts string, remoteFilePath string) {
	// Not adhering to actual URI standards -- I just want file paths
	localScriptFilePath := strings.TrimPrefix(scriptFile, fileURIPrefix)

	// Check for ~/ and expand if required
	localScriptFilePath = expandHomeDirectory(localScriptFilePath)

	printMessage(VerbosityFullData, "File URI Path '%s'\n", localScriptFilePath)

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
	scriptHash := SHA256Sum(scriptFileStr)

	printMessage(VerbosityFullData, "Local Script Hash '%s'\n", scriptHash)

	printMessage(VerbosityStandard, "Executing script '%s'\n", localScriptFilePath)

	// Semaphore to limit concurrency of host connections go routines as specified in main config
	semaphore := make(chan struct{}, config.MaxSSHConcurrency)

	if dryRunRequested {
		// Notify user that program is in dry run mode
		printMessage(VerbosityStandard, "Requested dry-run, aborting deployment\n")
		if globalVerbosityLevel < 2 {
			// If not running with higher verbosity, no need to collect deployment information
			return
		}
		printMessage(VerbosityProgress, "Outputting information collected for deployment:\n")
	}

	// Run script per host
	var wg sync.WaitGroup
	for endpointName := range config.HostInfo {
		// Only run against hosts specified
		if checkForOverride(hosts, endpointName) {
			printMessage(VerbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// Retrieve host secrests (keys,passwords)
		err = retrieveHostSecrets(endpointName)
		logError("Failed to retrieve host secrets", err, false)

		// Retrieve most current global host config
		hostInfo := config.HostInfo[endpointName]

		// If user requested dry run - print host information and abort connections
		if dryRunRequested {
			printHostInformation(hostInfo)
			continue
		}

		// Upload and execute the script - disable concurrency if maxconns is 1
		wg.Add(1)
		if config.MaxSSHConcurrency > 1 {
			go executeScriptOnHost(&wg, semaphore, hostInfo, scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash)
		} else {
			executeScriptOnHost(&wg, semaphore, hostInfo, scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash)
			if len(executionErrors) > 0 {
				// Execution error occured, don't continue with other hosts
				break
			}
		}
	}
	wg.Wait()

	// Print out any errors
	if len(executionErrors) > 0 {
		printMessage(VerbosityStandard, "Errors:\n  %v\n", executionErrors)
	}

}

// Connect to a host, upload a script, execute script and print output
func executeScriptOnHost(wg *sync.WaitGroup, semaphore chan struct{}, hostInfo EndpointInfo, scriptInterpreter string, remoteFilePath string, scriptFileBytes []byte, scriptHash string) {
	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	// Connect to the SSH server
	client, err := connectToSSH(hostInfo.Endpoint, hostInfo.EndpointUser, hostInfo.Password, hostInfo.PrivateKey, hostInfo.KeyAlgo)
	if err != nil {
		executionErrorsMutex.Lock()
		executionErrors += fmt.Sprintf("  Host '%s': %v\n", hostInfo.EndpointName, err)
		executionErrorsMutex.Unlock()
	}
	defer client.Close()

	// Run the script remotely
	scriptOutput, err := executeScript(client, hostInfo.Password, hostInfo.RemoteTransferBuffer, scriptInterpreter, remoteFilePath, scriptFileBytes, scriptHash)
	if err != nil {
		executionErrorsMutex.Lock()
		executionErrors += fmt.Sprintf("  Host '%s': %v\n", hostInfo.EndpointName, err)
		executionErrorsMutex.Unlock()
	}

	printMessage(VerbosityStandard, "  Host '%s':\n%s\n", hostInfo.EndpointName, scriptOutput)
}
