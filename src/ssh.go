// controller
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"golang.org/x/crypto/ssh"
)

// Standard SSH client configuration settings for specific host
func setupSSHConfig(hostInfo EndpointInfo) (config *ssh.ClientConfig) {
	var connectTimeout time.Duration
	if hostInfo.connectTimeout > 0 {
		connectTimeout = time.Duration(hostInfo.connectTimeout) * time.Second
	} else {
		connectTimeout = time.Duration(defaultConnectTimeout) * time.Second
	}

	config = &ssh.ClientConfig{
		User: hostInfo.endpointUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(hostInfo.privateKey),
			ssh.Password(hostInfo.password),
		},
		ClientVersion: sshVersionString,
		HostKeyAlgorithms: []string{
			hostInfo.keyAlgo,
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         connectTimeout,
	}
	return
}

// Handle building client config and connection to remote host
// Attempts to automatically recover from some errors like no route to host by waiting a bit
func connectToSSH(hostInfo EndpointInfo, proxyInfo EndpointInfo) (client *ssh.Client, proxyConn *ssh.Client, err error) {
	printMessage(verbosityProgress, "Host %s: Connecting to SSH server\n", hostInfo.endpointName)

	// Setup config for proxy if required
	var proxySSHconfig *ssh.ClientConfig
	if hostInfo.proxy != "" {
		proxySSHconfig = setupSSHConfig(proxyInfo)
	}

	SSHconfig := setupSSHConfig(hostInfo)

	// Only attempt connection x times
	const maxConnectionAttempts int = 3

	// Loop so some network errors can recover and try again
	for attempts := 0; attempts <= maxConnectionAttempts; attempts++ {
		if hostInfo.proxy != "" {
			printMessage(verbosityProgress, "Endpoint %s: Establishing connection to SSH server through proxy %s (%d/%d)\n", hostInfo.endpoint, proxyInfo.endpoint, attempts, maxConnectionAttempts)

			// SSH Connect to proxy
			proxyConn, err = ssh.Dial("tcp", proxyInfo.endpoint, proxySSHconfig)
			retryAvailable, successfulConnection := checkConnection(err)
			if retryAvailable {
				printMessage(verbosityProgress, "Endpoint %s: No route to SSH proxy server (%d/%d)\n", hostInfo.endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed connection to proxy server: %v", err)
				return
			}

			printMessage(verbosityProgress, "Host %s: Connected to SSH proxy server\n", hostInfo.endpointName)

			// TCP Connect to end server through proxy
			var clientTunnel net.Conn
			clientTunnel, err = proxyConn.Dial("tcp", hostInfo.endpoint)
			retryAvailable, successfulConnection = checkConnection(err)
			if retryAvailable {
				printMessage(verbosityProgress, "Endpoint %s: No route to SSH server (%d/%d)\n", hostInfo.endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed TCP connection to server: %v", err)
				return
			}

			printMessage(verbosityData, "Host %s: Connected by TCP to SSH server\n", hostInfo.endpointName)

			// SSH Hanshake with end server through proxy (error is evaluated below)
			var clientConn ssh.Conn
			var clientChannel <-chan ssh.NewChannel
			var clientRequest <-chan *ssh.Request
			clientConn, clientChannel, clientRequest, err = ssh.NewClientConn(clientTunnel, hostInfo.endpoint, SSHconfig)
			retryAvailable, successfulConnection = checkConnection(err)
			if retryAvailable {
				printMessage(verbosityProgress, "Endpoint %s: No route to SSH server (%d/%d)\n", hostInfo.endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed SSH handshake to server: %v", err)
				return
			}

			// Setup Client
			client = ssh.NewClient(clientConn, clientChannel, clientRequest)
			printMessage(verbosityProgress, "Host %s: Connected to SSH server\n", hostInfo.endpointName)

			break
		} else {
			printMessage(verbosityProgress, "Endpoint %s: Establishing connection to SSH server (%d/%d)\n", hostInfo.endpoint, attempts, maxConnectionAttempts)

			// Connect to the SSH server directly
			client, err = ssh.Dial("tcp", hostInfo.endpoint, SSHconfig)
			retryAvailable, successfulConnection := checkConnection(err)
			if retryAvailable {
				printMessage(verbosityProgress, "Endpoint %s: No route to SSH server (%d/%d)\n", hostInfo.endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed TCP connection to server: %v", err)
				return
			}

			printMessage(verbosityProgress, "Host %s: Connected to SSH server\n", hostInfo.endpointName)

			break
		}
	}

	return
}

// Checks for recoverable network connection errors
func checkConnection(err error) (retryAvailable bool, connectionSucceeded bool) {
	// Determine if error is recoverable
	if err != nil {
		if strings.Contains(err.Error(), "no route to host") {
			// Sleep for small time to wait for network path
			time.Sleep(200 * time.Millisecond)

			// Return to try the connection again
			connectionSucceeded = false
			retryAvailable = true
			return
		} else {
			// All other errors, bail from connection attempts
			connectionSucceeded = false
			retryAvailable = false
			return
		}
	} else {
		connectionSucceeded = true
		retryAvailable = false
		return
	}
}

// Uploads content to specified remote file path via SCP
func SCPUpload(client *ssh.Client, localFileContent []byte, remoteFilePath string) (err error) {
	transferClient, err := scp.NewClientBySSHWithTimeout(client, 900*time.Second)
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
	transferClient, err := scp.NewClientBySSHWithTimeout(client, 90*time.Second)
	if err != nil {
		err = fmt.Errorf("failed to create scp session: %v", err)
		return
	}
	defer transferClient.Close()

	// Buffer to receive bytes from remote
	var localTransferBuffer bytes.Buffer

	_, err = transferClient.CopyFromRemoteFileInfos(context.Background(), &localTransferBuffer, remoteFilePath, nil)
	if err != nil {
		err = fmt.Errorf("failed scp transfer: %v", err)
		return
	}

	// Convert formats
	fileContentBytes = localTransferBuffer.Bytes()
	return
}

// Runs the given remote ssh command optionally with sudo
// runAs input will change to the user using sudo if not it will use root
// disableSudo will determine if command runs with sudo or not (default, will always use sudo)
// Empty sudoPassword will run without assuming the user account doesn't require any passwords
func (command RemoteCommand) SSHexec(client *ssh.Client, runAs string, disableSudo bool, sudoPassword string) (commandOutput string, err error) {
	// Open new session (exec)
	session, err := client.NewSession()
	if err != nil {
		err = fmt.Errorf("failed to create session: %v", err)
		return
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdout pipe: %v", err)
		return
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stderr pipe: %v", err)
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdin pipe: %v", err)
		return
	}
	defer stdin.Close()

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

	printMessage(verbosityDebug, "  Running command '%s'\n", command.string)

	err = session.Start(command.string)
	if err != nil {
		err = fmt.Errorf("failed to start command: %v", err)
		return
	}

	// Only use stdin when sudo is required
	if !disableSudo {
		_, err = stdin.Write([]byte(sudoPassword))
		if err != nil {
			err = fmt.Errorf("failed to write to command stdin: %v", err)
			return
		}

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
	}

	// Setups for timeout, output streaming, error handling
	maxExecutionTime := time.Duration(command.timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), maxExecutionTime)
	defer cancel()

	var stdoutBuffer strings.Builder
	teeReader := io.TeeReader(stdout, &stdoutBuffer)

	var commandstderr []byte
	var exitStatusZero bool

	if command.streamStdout {
		// channel scoped only here
		errChannel := make(chan error)

		go func() {
			_, err := io.Copy(os.Stdout, teeReader)
			if err != nil {
				errChannel <- fmt.Errorf("error streaming remote command stdout to program stdout: %v", err)
				return
			}
			errChannel <- nil
		}()

		err = <-errChannel
		if err != nil {
			err = fmt.Errorf("error streaming remote command stdout to program stdout: %v", err)
			return
		}
	}

	// Wait in background for command
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
				err = fmt.Errorf("error reading error from command '%s': %v", command.string, errorsError)
				return
			}

			// Return commands error
			err = fmt.Errorf("error with command '%s': %v: %s", command.string, err, commandstderr)
			return
		} else {
			// nil from session.Wait() means exit status zero from the command
			exitStatusZero = true
		}
	// Timer finishes before command
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		session.Close()
		err = fmt.Errorf("closed ssh session: exceeded timeout (%d seconds) for command %s", command.timeout, command.string)
		return
	}

	commandstderr, err = io.ReadAll(stderr)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	commandError := string(commandstderr)

	if command.streamStdout {
		commandOutput = stdoutBuffer.String()
	} else {
		var commandstdout []byte
		commandstdout, err = io.ReadAll(stdout)
		if err != nil {
			err = fmt.Errorf("failed to read stdout buffer: %v", err)
			return
		}

		commandOutput = string(commandstdout)
	}

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
