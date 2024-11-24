// controller
package main

import (
	"encoding/binary"
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
	fmt.Printf("%s\n", progCLIHeader)
	fmt.Printf("Pushing deployer update using executable at %s\n", deployerUpdateFile)

	// Load binary from file
	deployerUpdateBinary, err := os.ReadFile(deployerUpdateFile)
	logError("failed loading deployer executable file", err, true)

	// Loop deployers
	_, err = connectToDeployers(config, deployerUpdateBinary, hostOverride, false)
	logError("Failed to update remote deployer executables", err, false)

	// Show status to user
	fmt.Print("Please wait for deployer services to auto-restart (1 min)\n")
	fmt.Print("===========================================================\n")
}

// Entry point for checking remote deployer binary version
func getDeployerVersions(config Config, hostOverride string) {
	fmt.Printf("%s\n", progCLIHeader)

	// Loop deployers
	deployerVersions, err := connectToDeployers(config, nil, hostOverride, true)
	logError("Failed to check remote deployer verions", err, false)

	// Show versions to user
	fmt.Printf("Deployer executable versions:\n%s", deployerVersions)
	fmt.Print("================================================================\n")
}

// Semi-generic connect to remote deployer endpoints
// Used for checking versions and updating binary of deployer
func connectToDeployers(config Config, deployerUpdateBinary []byte, hostOverride string, checkVersion bool) (returnedData string, err error) {
	// Check local system
	err = localSystemChecks()
	if err != nil {
		return
	}

	// Loop over config endpoints for updater/version
	for endpointName, endpointInfo := range config.DeployerEndpoints {
		// Use hosts user specifies if requested
		SkipHost := checkForHostOverride(hostOverride, endpointName)
		if SkipHost {
			continue
		}

		// Extract vars for endpoint information
		var info EndpointInfo
		info, err = retrieveEndpointInfo(endpointInfo, config.SSHClientDefault)
		if err != nil {
			err = fmt.Errorf("failed to retrieve endpoint information for '%s': %v", endpointName, err)
			return
		}

		// Connect to deployer
		var stdout string
		stdout, err = deployerClient(deployerUpdateBinary, endpointName, info, checkVersion)
		if err != nil {
			// Print error for host - bail further updating
			err = fmt.Errorf("host '%s': %v", endpointName, err)
			return
		}

		// If just checking version, Print
		if checkVersion {
			returnedData = returnedData + fmt.Sprintf("%s:%s\n", endpointName, stdout)
		}
	}

	return
}

// Transfers updated deployer binary to remote temp buffer (file path from global var)
// Calls custom ssh request type to start update process
// If requested, will retrieve deployer version from SSH version in handshake and return
func deployerClient(deployerUpdateBinary []byte, endpointName string, endpointInfo EndpointInfo, checkVersion bool) (stdout string, err error) {
	// Connect to the SSH server
	client, err := connectToSSH(endpointInfo.Endpoint, endpointInfo.EndpointUser, endpointInfo.PrivateKey, endpointInfo.KeyAlgo)
	if err != nil {
		err = fmt.Errorf("failed connect to SSH server: %v", err)
		return
	}
	defer client.Close()

	if checkVersion {
		// Get remote host deployer version
		deployerSSHVersion := string(client.Conn.ServerVersion())
		stdout = strings.Replace(deployerSSHVersion, "SSH-2.0-OpenSSH_", "", 1)
		return
	}

	// SFTP to default temp file
	err = RunSFTP(client, deployerUpdateBinary, endpointInfo.RemoteTransferBuffer)
	if err != nil {
		return
	}

	// Open new session
	session, err := client.NewSession()
	if err != nil {
		err = fmt.Errorf("failed to create session: %v", err)
		return
	}
	defer session.Close()

	// Create payload with length header
	var requestPayload []byte
	payload := []byte(endpointInfo.RemoteTransferBuffer)
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(payload)))

	// Add length of payload as header beginning
	requestPayload = append(requestPayload, lengthBytes...)

	// Add the payload data
	requestPayload = append(requestPayload, payload...)

	// Set custom request
	requestType := "update"
	wantReply := true
	reqAccepted, err := session.SendRequest(requestType, wantReply, requestPayload)
	if err != nil {
		err = fmt.Errorf("failed to create update session: %v", err)
		return
	}
	if !reqAccepted {
		err = fmt.Errorf("server did not accept request type '%s'", requestType)
		return
	}

	// Command Error
	stderr, err := session.StderrPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stderr pipe: %v", err)
		return
	}

	// Command stdin
	stdin, err := session.StdinPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdin pipe: %v", err)
		return
	}
	defer stdin.Close()

	// Write sudo password to stdin
	_, err = stdin.Write([]byte(endpointInfo.SudoPassword))
	if err != nil {
		err = fmt.Errorf("failed to write to command stdin: %v", err)
		return
	}

	// Close stdin to signal no more writing
	err = stdin.Close()
	if err != nil {
		err = fmt.Errorf("failed to close stdin: %v", err)
		return
	}

	// Read error output from session
	updateError, err := io.ReadAll(stderr)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// If the command had an error on the remote side
	if len(updateError) > 0 {
		err = fmt.Errorf("%s (check /tmp/updater.log on remote system for more information)", updateError)
		return
	}

	// Show progress to user
	fmt.Printf("Updates Pushed to %s\n", endpointName)

	return
}
