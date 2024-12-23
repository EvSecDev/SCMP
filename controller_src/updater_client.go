// controller
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ###################################
//      UPDATE FUNCTIONS
// ###################################

// Entry point for updating remote deployer binary
func updateDeployer(config Config, deployerUpdateFile string, hostOverride string) {
	printMessage(VerbosityStandard, "%s\n", progCLIHeader)
	printMessage(VerbosityStandard, "Pushing deployer update using executable at %s\n", deployerUpdateFile)
	printMessage(VerbosityStandard, "Note: Please wait 15 seconds for deployer to start after update\n")

	// Load binary from file
	deployerUpdateBinary, err := os.ReadFile(deployerUpdateFile)
	logError("failed loading deployer executable file", err, true)

	// Loop deployers
	_, err = connectToDeployers(config, deployerUpdateBinary, hostOverride, "update")
	logError("Failed to update remote deployer executables", err, false)
	printMessage(VerbosityStandard, "===========================================================\n")
}

// Entry point for updating remote updater binary
func updateUpdater(config Config, updaterUpdateFile string, hostOverride string) {
	printMessage(VerbosityStandard, "%s\n", progCLIHeader)
	printMessage(VerbosityStandard, "Pushing updater update using executable at %s\n", updaterUpdateFile)

	// Load binary from file
	updaterUpdateBinary, err := os.ReadFile(updaterUpdateFile)
	logError("failed loading deployer executable file", err, true)

	// Loop deployers - set mode to updateUpdater (also the ssh request type)
	_, err = connectToDeployers(config, updaterUpdateBinary, hostOverride, "updateUpdater")
	logError("Failed to update remote deployer executables", err, false)
	printMessage(VerbosityStandard, "===========================================================\n")
}

// Entry point for checking remote deployer binary version
func getDeployerVersion(config Config, hostOverride string) {
	printMessage(VerbosityStandard, "%s\n", progCLIHeader)

	// Loop deployers
	deployerVersions, err := connectToDeployers(config, nil, hostOverride, "getDeployerVersion")
	logError("Failed to check remote deployer verions", err, false)

	// Show versions to user
	if deployerVersions != "" {
		printMessage(VerbosityStandard, "Deployer executable versions:\n%s", deployerVersions)
	}
	printMessage(VerbosityStandard, "================================================================\n")
}

// Entry point for checking remote updater binary version
func getUpdaterVersion(config Config, hostOverride string) {
	printMessage(VerbosityStandard, "%s\n", progCLIHeader)

	// Loop deployers
	updaterVersions, err := connectToDeployers(config, nil, hostOverride, "getUpdaterVersion")
	logError("Failed to check remote updater verions", err, false)

	// Show versions to user
	if updaterVersions != "" {
		printMessage(VerbosityStandard, "Updater executable versions:\n%s", updaterVersions)
	}
	printMessage(VerbosityStandard, "================================================================\n")
}

// Semi-generic connect to remote deployer endpoints
// Used for checking versions and updating binary of deployer
func connectToDeployers(config Config, updateBinary []byte, hostOverride string, mode string) (returnedData string, err error) {
	// Check local system
	err = localSystemChecks()
	if err != nil {
		return
	}

	if dryRunRequested {
		// Notify user that program is in dry run mode
		printMessage(VerbosityStandard, "\nRequested dry-run, aborting connections - outputting information collected for connections:\n\n")
	}

	// Loop over config endpoints for updater/version
	for endpointName, endpointInfo := range config.DeployerEndpoints {
		printMessage(VerbosityProgress, "Host %s\n", endpointName)

		// Use hosts user specifies if requested
		skipHost := checkForOverride(hostOverride, endpointName)
		if skipHost {
			printMessage(VerbosityProgress, "  Host not desired\n")
			continue
		}

		// Extract vars for endpoint information
		var info EndpointInfo
		info, err = retrieveEndpointInfo(endpointInfo, config.SSHClientDefault)
		if err != nil {
			err = fmt.Errorf("failed to retrieve endpoint information for '%s': %v", endpointName, err)
			return
		}

		// If user requested dry run - print collected information so far and gracefully abort update
		if dryRunRequested {
			printMessage(VerbosityStandard, "Host: %s\n", endpointName)
			printMessage(VerbosityStandard, "  Options:\n")
			printMessage(VerbosityStandard, "       Endpoint Address: %s\n", info.Endpoint)
			printMessage(VerbosityStandard, "       SSH User:         %s\n", info.EndpointUser)
			printMessage(VerbosityStandard, "       SSH Key:          %s\n", info.PrivateKey.PublicKey())
			printMessage(VerbosityStandard, "       Transfer Buffer:  %s\n", info.RemoteTransferBuffer)
			continue
		}

		// Connect to deployer
		var stdout string
		stdout, err = deployerClient(updateBinary, endpointName, info, mode)
		if err != nil {
			// Print error for host - bail further updating
			err = fmt.Errorf("host '%s': %v", endpointName, err)
			return
		}

		// Add version to output and continue to next host
		if mode == "getDeployerVersion" || mode == "getUpdaterVersion" {
			returnedData = returnedData + fmt.Sprintf("%s:%s\n", endpointName, stdout)
			continue
		}

		// Show update progress to user
		if strings.ToLower(stdout) == "update successful" {
			printMessage(VerbosityStandard, "Updated %s\n", endpointName)
		} else {
			printMessage(VerbosityStandard, "Update Pushed to %s (did not receive confirmation)\n", endpointName)
		}
	}

	return
}

// Transfers updated deployer binary to remote temp buffer (file path from global var)
// Calls custom ssh request type to start update process
// If requested, will retrieve deployer version from SSH version in handshake and return
func deployerClient(updateBinary []byte, endpointName string, endpointInfo EndpointInfo, mode string) (cmdOutput string, err error) {
	printMessage(VerbosityProgress, "Host %s: Connecting to SSH server\n", endpointName)

	// Connect to the SSH server
	client, err := connectToSSH(endpointInfo.Endpoint, endpointInfo.EndpointUser, endpointInfo.PrivateKey, endpointInfo.KeyAlgo)
	if err != nil {
		err = fmt.Errorf("failed connect to SSH server: %v", err)
		return
	}
	defer client.Close()

	if mode == "getDeployerVersion" {
		// Get remote host deployer version
		deployerSSHVersion := string(client.Conn.ServerVersion())
		cmdOutput = strings.Replace(deployerSSHVersion, "SSH-2.0-OpenSSH_", "", 1)
		return
	}

	// Transfer update file to remote if in update mode
	if mode == "update" || mode == "updateUpdater" {
		printMessage(VerbosityProgress, "  Transferring update file to remote\n")

		// SFTP to default temp file
		err = RunSFTP(client, updateBinary, endpointInfo.RemoteTransferBuffer)
		if err != nil {
			return
		}
	}

	// Open new session
	session, err := client.NewSession()
	if err != nil {
		err = fmt.Errorf("failed to create session: %v", err)
		return
	}
	defer session.Close()

	// Set custom request
	var requestType string
	if mode == "updateUpdater" {
		requestType = mode
	} else if mode == "update" {
		requestType = mode
	} else if mode == "getUpdaterVersion" {
		requestType = mode
	} else {
		err = fmt.Errorf("invalid mode: unsupported SSH request type")
		return
	}

	// Generate request and send
	err = sendCustomSSHRequest(session, requestType, true, endpointInfo.RemoteTransferBuffer)
	if err != nil {
		return
	}

	// Command output
	stdout, err := session.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdout pipe: %v", err)
		return
	}

	// Command Error
	stderr, err := session.StderrPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stderr pipe: %v", err)
		return
	}

	// Command stdin
	var stdin io.WriteCloser
	stdin, err = session.StdinPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdin pipe: %v", err)
		return
	}
	defer stdin.Close()

	// Send sudo password if updating deployer
	if mode == "update" || mode == "updateUpdater" {
		printMessage(VerbosityProgress, "  Writing sudo password to stdin\n")

		// Write sudo password to stdin
		_, err = stdin.Write([]byte(endpointInfo.SudoPassword))
		if err != nil {
			err = fmt.Errorf("failed to write to command stdin: %v", err)
			return
		}
	}

	// Close stdin to signal no more writing
	err = stdin.Close()
	if err != nil {
		err = fmt.Errorf("failed to close stdin: %v", err)
		return
	}

	printMessage(VerbosityProgress, "  Reading stderr from remote\n")

	// Read error output from session
	updateError, err := io.ReadAll(stderr)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// If the command had an error on the remote side
	if len(updateError) > 0 {
		err = fmt.Errorf("%s ", updateError)
		return
	}

	printMessage(VerbosityProgress, "  Reading stdout from remote\n")

	// Read commands output from session
	updateStdout, err := io.ReadAll(stdout)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}
	// Convert bytes to string
	stdoutString := string(updateStdout)
	cmdOutput = strings.ReplaceAll(strings.ReplaceAll(stdoutString, "\n", ""), "\r", "")

	return
}
