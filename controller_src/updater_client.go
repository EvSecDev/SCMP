// controller
package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ###################################
//      UPDATE FUNCTIONS
// ###################################

func simpleLoopHosts(config Config, deployerUpdateFile string, hostOverride string, checkVersion bool) (deployerVersions string) {
	// Check local system
	err := localSystemChecks(config.Controller.RepositoryPath)
	logError("Error in system checks", err, false)

	// Load Binary if updating
	var deployerUpdateBinary []byte
	if !checkVersion {
		// Load binary from file
		deployerUpdateBinary, err = os.ReadFile(deployerUpdateFile)
		logError("failed loading deployer executable file", err, true)
	}

	// Get SSH Private Key
	PrivateKey, err := SSHIdentityToKey(config.SSHClient.SSHIdentityFile, config.SSHClient.UseSSHAgent)
	logError("Error retrieving SSH private key", err, true)

	// Loop over config endpoints for updater/version
	for endpointName, endpointInfo := range config.DeployerEndpoints {
		// Use hosts user specifies if requested
		SkipHost := checkForHostOverride(hostOverride, endpointName)
		if SkipHost {
			continue
		}

		// Extract vars for endpoint information
		endpointIP := endpointInfo[0].Endpoint
		endpointPort := endpointInfo[1].EndpointPort
		endpointUser := endpointInfo[2].EndpointUser

		// Run update
		returnedData, err := DeployerUpdater(deployerUpdateBinary, PrivateKey, config.SSHClient.SudoPassword, checkVersion, endpointUser, endpointIP, endpointPort)
		if err != nil {
			logError(fmt.Sprintf("Error: host '%s'", endpointName), err, true)
			continue
		}

		// If just checking version, Print
		if checkVersion {
			deployerVersions = deployerVersions + fmt.Sprintf("%s:%s\n", endpointName, returnedData)
		}
	}
	return
}

func DeployerUpdater(deployerUpdateBinary []byte, PrivateKey ssh.Signer, SudoPassword string, checkVersion bool, endpointUser string, endpointIP string, endpointPort int) (deployerVersion string, err error) {
	// Network info checks
	endpointSocket, err := ParseEndpointAddress(endpointIP, endpointPort)
	if err != nil {
		err = fmt.Errorf("failed to parse network address: %v", err)
		return
	}

	// Connect to the SSH server
	client, err := connectToSSH(endpointSocket, endpointUser, PrivateKey)
	if err != nil {
		err = fmt.Errorf("failed connect to SSH server: %v", err)
		return
	}
	defer client.Close()

	if checkVersion {
		// Get remote host deployer version
		deployerSSHVersion := string(client.Conn.ServerVersion())
		deployerVersion = strings.Replace(deployerSSHVersion, "SSH-2.0-OpenSSH_", "", 1)
		return
	}

	// SFTP to default temp file
	err = RunSFTP(client, deployerUpdateBinary)
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

	// Set custom request
	requestType := "update"
	wantReply := true
	reqAccepted, err := session.SendRequest(requestType, wantReply, nil)
	if err != nil {
		err = fmt.Errorf("failed to create update session: %v", err)
		return
	}
	if !reqAccepted {
		err = fmt.Errorf("server did not accept request type '%s'", requestType)
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
	_, err = stdin.Write([]byte(SudoPassword))
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

	return
}
