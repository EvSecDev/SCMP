// controller
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"golang.org/x/crypto/ssh"
)

// Handle building client config and connection to remote host
// Attempts to automatically recover from some errors like no route to host by waiting a bit
func connectToSSH(endpointName string, endpointSocket string, endpointUser string, loginPassword string, privateKey ssh.Signer, keyAlgorithm string) (client *ssh.Client, err error) {
	printMessage(verbosityProgress, "Host %s: Connecting to SSH server\n", endpointName)

	// Setup config for client
	SSHconfig := &ssh.ClientConfig{
		User: endpointUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(privateKey),
			ssh.Password(loginPassword),
		},
		// Some IPS rules flag on GO's ssh client string
		ClientVersion: "SSH-2.0-OpenSSH_9.8p1",
		HostKeyAlgorithms: []string{
			keyAlgorithm,
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	// Only attempt connection x times
	maxConnectionAttempts := 3

	// Loop so some network errors can recover and try again
	for attempts := 0; attempts <= maxConnectionAttempts; attempts++ {
		printMessage(verbosityProgress, "Endpoint %s: Establishing connection to SSH server (%d/%d)\n", endpointSocket, attempts, maxConnectionAttempts)

		// Connect to the SSH server direct
		client, err = ssh.Dial("tcp", endpointSocket, SSHconfig)

		// Determine if error is recoverable
		if err != nil {
			if strings.Contains(err.Error(), "no route to host") {
				printMessage(verbosityProgress, "Endpoint %s: No route to SSH server (%d/%d)\n", endpointSocket, attempts, maxConnectionAttempts)
				// Re-attempt after waiting for network path
				time.Sleep(200 * time.Millisecond)
				continue
			} else {
				// All other errors, bail from connection attempts
				return
			}
		} else {
			// Connection worked
			printMessage(verbosityProgress, "Host %s: Connected to SSH server\n", endpointName)
			break
		}
	}

	return
}

// Uploads content to specified remote file path via SCP
func SCPUpload(client *ssh.Client, localFileContent []byte, remoteFilePath string) (err error) {
	// Open SCP client
	transferClient, err := scp.NewClientBySSHWithTimeout(client, 90*time.Second)
	if err != nil {
		err = fmt.Errorf("failed to create scp session: %v", err)
		return
	}
	defer transferClient.Close()

	// Convert input data to a Reader for SCP pkg
	localContentReader := bytes.NewReader(localFileContent)
	localContentSize := int64(len(localFileContent))

	// Transfer content to remote file path
	err = transferClient.Copy(context.Background(), localContentReader, remoteFilePath, "0640", localContentSize)
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			err = fmt.Errorf("unable to write to %s (is it writable by the user?): %v", remoteFilePath, err)
		} else {
			err = fmt.Errorf("failed scp transfer: %v", err)
		}
		return
	}

	return
}

// Downloads a remote files content via SCP
func SCPDownload(client *ssh.Client, remoteFilePath string) (fileContentBytes []byte, err error) {
	// Open SCP client
	transferClient, err := scp.NewClientBySSHWithTimeout(client, 90*time.Second)
	if err != nil {
		err = fmt.Errorf("failed to create scp session: %v", err)
		return
	}
	defer transferClient.Close()

	// Buffer to receive bytes from remote
	var localTransferBuffer bytes.Buffer

	// Get remote contents
	_, err = transferClient.CopyFromRemoteFileInfos(context.Background(), &localTransferBuffer, remoteFilePath, nil)
	if err != nil {
		err = fmt.Errorf("failed scp transfer: %v", err)
		return
	}

	// Convert formats
	fileContentBytes = localTransferBuffer.Bytes()
	return
}

// Runs the given remote ssh command with sudo
// runAs input will change to the user using sudo if not root
// disableSudo will determine if command runs with sudo or not (default, will always use sudo)
// Empty sudoPassword will run without assuming the user account doesn't require any passwords
// timeout is the max execution time in seconds for the given command
func (command RemoteCommand) SSHexec(client *ssh.Client, runAs string, disableSudo bool, sudoPassword string, timeout int) (commandOutput string, err error) {
	// Open new session (exec)
	session, err := client.NewSession()
	if err != nil {
		err = fmt.Errorf("failed to create session: %v", err)
		return
	}
	defer session.Close()

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
	stdin, err := session.StdinPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdin pipe: %v", err)
		return
	}
	defer stdin.Close()

	// Prepare command prefix
	cmdPrefix := "sudo "
	if sudoPassword != "" {
		// sudo password provided, adding stdin arg to sudo
		cmdPrefix += "-S "
	}
	if runAs != "" && runAs != "root" {
		// Non-root other user requested, adding su to sudo
		cmdPrefix += "-u " + runAs + " "
	}
	if disableSudo {
		// No sudo requested, remove sudo prefix
		cmdPrefix = ""
	}

	// Add prefix to command
	command.string = cmdPrefix + command.string

	printMessage(verbosityDebug, "  Running command '%s'\n", command)

	// Start the command
	err = session.Start(command.string)
	if err != nil {
		err = fmt.Errorf("failed to start command: %v", err)
		return
	}

	// Write sudo password to stdin - write even if password is empty
	_, err = stdin.Write([]byte(sudoPassword))
	if err != nil {
		err = fmt.Errorf("failed to write to command stdin: %v", err)
		return
	}

	// Close stdin to signal no more writing
	err = stdin.Close()
	if err != nil {
		if strings.Contains(err.Error(), "EOF") {
			// End of file is not an error - reset err and dont return
			err = nil
		} else {
			err = fmt.Errorf("failed to close stdin: %v", err)
			return
		}
	}

	// Context for command wait based on timeout declared in global
	maxExecutionTime := time.Duration(timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), maxExecutionTime)
	defer cancel()

	// Wait for the command to finish with timeout
	var commandstderr []byte
	var exitStatusZero bool
	errChannel := make(chan error)
	go func() {
		errChannel <- session.Wait()
	}()
	// Block until errChannel is done, then parse errors
	select {
	// Command finishes before timeout with error
	case err = <-errChannel:
		if err != nil {
			// Return both exit status and stderr (readall errors are ignored as exit status will still be present)
			var errorsError error // Store local error
			commandstderr, errorsError = io.ReadAll(stderr)
			if errorsError != nil {
				// Return at any errors reading the command error
				err = fmt.Errorf("error reading error from command '%s': %v", command, errorsError)
				return
			}

			// Return commands error
			err = fmt.Errorf("error with command '%s': %v: %s", command, err, commandstderr)
			return
		} else {
			// nil from session.Wait() means exit status zero from the command
			exitStatusZero = true
		}
	// Timer finishes before command
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		session.Close()
		err = fmt.Errorf("closed ssh session: exceeded timeout (%d seconds) for command %s", timeout, command)
		return
	}

	// Read commands output from session
	commandstdout, err := io.ReadAll(stdout)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// Read commands error output from session
	commandstderr, err = io.ReadAll(stderr)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// Convert bytes to string
	commandOutput = string(commandstdout)
	commandError := string(commandstderr)

	// If the command had an error on the remote side and session indicated non-zero exit status
	if commandError != "" && !exitStatusZero {
		// Only return valid errors
		if strings.Contains(commandError, "[sudo] password for") {
			// Sudo puts password prompts into stderr when running with '-S'
			err = nil
		} else {
			err = fmt.Errorf("%s", commandError)
			return
		}
	}

	return
}
