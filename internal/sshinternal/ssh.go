package sshinternal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"scmp/internal/config"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"golang.org/x/crypto/ssh"
)

// Standard SSH client configuration settings for specific host
func setupSSHConfig(ctx context.Context, hostInfo config.EndpointInfo) (config *ssh.ClientConfig) {
	var connectTimeout time.Duration
	if hostInfo.ConnectTimeout > 0 {
		connectTimeout = time.Duration(hostInfo.ConnectTimeout) * time.Second
	} else {
		connectTimeout = time.Duration(DefaultConnectTimeout) * time.Second
	}

	config = &ssh.ClientConfig{
		User: hostInfo.EndpointUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(hostInfo.PrivateKey),
			ssh.Password(hostInfo.Password),
		},
		ClientVersion: SSHVersionString,
		HostKeyAlgorithms: []string{
			hostInfo.KeyAlgo,
		},
		HostKeyCallback: func(hostname string, remote net.Addr, pubKey ssh.PublicKey) error {
			return hostKeyCallback(ctx, hostname, remote, pubKey) // Inject context into callback function
		},
		Timeout: connectTimeout,
	}
	return
}

// Handle building client config and connection to remote host
// Attempts to automatically recover from some errors like no route to host by waiting a bit
func ConnectToSSH(ctx context.Context, hostInfo config.EndpointInfo, proxyInfo config.EndpointInfo) (client *ssh.Client, proxyConn *ssh.Client, err error) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSSSH)

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Connecting to SSH server\n", hostInfo.EndpointName)

	// Setup config for proxy if required
	var proxySSHconfig *ssh.ClientConfig
	if hostInfo.Proxy != "" {
		proxySSHconfig = setupSSHConfig(ctx, proxyInfo)
	}

	SSHconfig := setupSSHConfig(ctx, hostInfo)

	// Only attempt connection x times
	const maxConnectionAttempts int = 3

	// Loop so some network errors can recover and try again
	for attempts := 0; attempts <= maxConnectionAttempts; attempts++ {
		if hostInfo.Proxy != "" {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Endpoint %s: Establishing connection to SSH server through proxy %s (%d/%d)\n", hostInfo.Endpoint, proxyInfo.Endpoint, attempts, maxConnectionAttempts)

			// SSH Connect to proxy
			proxyConn, err = ssh.Dial("tcp", proxyInfo.Endpoint, proxySSHconfig)
			retryAvailable, successfulConnection := checkConnection(err)
			if retryAvailable {
				logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Endpoint %s: No route to SSH proxy server (%d/%d)\n", hostInfo.Endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed connection to proxy server: %w", err)
				return
			}

			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Connected to SSH proxy server\n", hostInfo.EndpointName)

			// TCP Connect to end server through proxy
			var clientTunnel net.Conn
			clientTunnel, err = proxyConn.Dial("tcp", hostInfo.Endpoint)
			retryAvailable, successfulConnection = checkConnection(err)
			if retryAvailable {
				logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Endpoint %s: No route to SSH server (%d/%d)\n", hostInfo.Endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed TCP connection to server: %w", err)
				return
			}

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Connected by TCP to SSH server\n", hostInfo.EndpointName)

			// SSH Handshake with end server through proxy (error is evaluated below)
			var clientConn ssh.Conn
			var clientChannel <-chan ssh.NewChannel
			var clientRequest <-chan *ssh.Request
			clientConn, clientChannel, clientRequest, err = ssh.NewClientConn(clientTunnel, hostInfo.Endpoint, SSHconfig)
			retryAvailable, successfulConnection = checkConnection(err)
			if retryAvailable {
				logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Endpoint %s: No route to SSH server (%d/%d)\n", hostInfo.Endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed SSH handshake to server: %w", err)
				return
			}

			// Setup Client
			client = ssh.NewClient(clientConn, clientChannel, clientRequest)
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Connected to SSH server\n", hostInfo.EndpointName)

			break
		} else {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Endpoint %s: Establishing connection to SSH server (%d/%d)\n", hostInfo.Endpoint, attempts, maxConnectionAttempts)

			// Connect to the SSH server directly
			client, err = ssh.Dial("tcp", hostInfo.Endpoint, SSHconfig)
			retryAvailable, successfulConnection := checkConnection(err)
			if retryAvailable {
				logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Endpoint %s: No route to SSH server (%d/%d)\n", hostInfo.Endpoint, attempts, maxConnectionAttempts)
				continue
			}
			if !successfulConnection {
				err = fmt.Errorf("failed TCP connection to server: %w", err)
				return
			}

			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Connected to SSH server\n", hostInfo.EndpointName)

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

func watchLongTransfer(ctx context.Context, filename str.RemotePath, done chan struct{}) {
	select {
	case <-time.After(10 * time.Second):
		// If task takes more than 10 seconds, print status
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "File transfer still running for '%s'\n", filename)
	case <-done:
		// If the task finishes before 10 seconds, no feedback message
	}
}

// Uploads content to specified remote file path via SCP
func SCPUpload(ctx context.Context, client *ssh.Client, localFileContent []byte, remoteFilePath str.RemotePath) (err error) {
	transferClient, err := scp.NewClientBySSHWithTimeout(client, 900*time.Second)
	if err != nil {
		err = fmt.Errorf("failed to create scp session: %w", err)
		return
	}
	defer transferClient.Close()

	// Convert input data to a Reader for SCP pkg
	localContentReader := bytes.NewReader(localFileContent)
	localContentSize := int64(len(localFileContent))

	// Transfer content to remote file path
	done := make(chan struct{})
	go watchLongTransfer(ctx, remoteFilePath, done)
	err = transferClient.Copy(context.Background(), localContentReader, string(remoteFilePath), "0640", localContentSize)
	close(done)
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			err = fmt.Errorf("unable to write to %s (is it writable by the user?): %w", remoteFilePath, err)
		} else {
			err = fmt.Errorf("failed scp transfer: %w", err)
		}
		return
	}

	return
}

// Downloads a remote files content via SCP
func SCPDownload(ctx context.Context, client *ssh.Client, remoteFilePath str.RemotePath) (fileContentBytes []byte, err error) {
	transferClient, err := scp.NewClientBySSHWithTimeout(client, 90*time.Second)
	if err != nil {
		err = fmt.Errorf("failed to create scp session: %w", err)
		return
	}
	defer transferClient.Close()

	// Buffer to receive bytes from remote
	var localTransferBuffer bytes.Buffer

	done := make(chan struct{})
	go watchLongTransfer(ctx, remoteFilePath, done)
	_, err = transferClient.CopyFromRemoteFileInfos(context.Background(), &localTransferBuffer, string(remoteFilePath), nil)
	close(done)
	if err != nil {
		err = fmt.Errorf("failed scp transfer: %w", err)
		return
	}

	// Convert formats
	fileContentBytes = localTransferBuffer.Bytes()
	return
}

// New SSH channel with retry option (exponential backoff timer)
//
// This accounts for the delay on SSH servers between the reception
// of channel close request (packet type 97) and when the server
// calls 'free: server-session' to decrement nchannel count
func newSessionWithRetry(ctx context.Context, client *ssh.Client) (session *ssh.Session, err error) {
	// Initial sleep should guarantee no server errors on LAN
	// For deployments crossing the internet, it should help minimize "error: no more sessions" on the server side (key word there is minimize)
	// Heavily network-latency dependent
	backoff := 5 * time.Millisecond
	maxBackoff := 500 * time.Millisecond // Remote server should have gc'd by now
	retryCount := 1

	// Not a great solution, but multiple channels are basically guaranteed to breach max sessions without a small artificial wait time
	time.Sleep(backoff)

	for {
		session, err = client.NewSession()
		if err == nil {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
				"Endpoint %s:     New channel request succeeded on attempt %d\n",
				client.RemoteAddr().String(), retryCount)
			// No retries on success
			return
		}

		// Any other errors during channel creation should immediately bail
		if !strings.Contains(err.Error(), "rejected: connect failed (open failed)") {
			err = fmt.Errorf("channel create: %w", err)
			return
		}

		// Loop limit set at max backoff - just bail with error at that point
		if backoff > maxBackoff {
			err = fmt.Errorf("exceeded the remote server's maximum simultaneous channels (reached timeout after %d retries): %w", retryCount, err)
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.WarnLog,
			"Endpoint %s:     New channel request failed on attempt %d: sleeping for %s\n",
			client.RemoteAddr().String(), retryCount, backoff.String())
		retryCount++
		time.Sleep(backoff)
		backoff *= 2
	}
}

// Runs the given remote ssh command optionally with sudo
// runAs input will change to the user using sudo if not it will use root
// disableSudo will determine if command runs with sudo or not (default, will always use sudo)
// Empty sudoPassword will run without assuming the user account doesn't require any passwords
func (command RemoteCommand) SSHexec(ctx context.Context, client *ssh.Client, sudoPassword string) (commandOutput string, err error) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSParsing)

	// Open new session (exec)
	session, err := newSessionWithRetry(ctx, client)
	if err != nil {
		err = fmt.Errorf("session create: %w", err)
		return
	}
	defer func() {
		lerr := session.Close()
		if err == nil && lerr != nil && !errors.Is(lerr, io.EOF) {
			err = fmt.Errorf("failed to close session: %w", lerr)
		}
	}()

	ctx = logctx.AppendCtxTag(ctx, logctx.NSSSH)

	stdout, err := session.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdout pipe: %w", err)
		return
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stderr pipe: %w", err)
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdin pipe: %w", err)
		return
	}
	defer func() {
		lerr := stdin.Close()
		if err == nil && lerr != nil && !errors.Is(lerr, io.EOF) {
			err = fmt.Errorf("failed to close stdin: %w", lerr)
		}
	}()

	cmdPrefix := "sudo "
	if sudoPassword != "" {
		// sudo password provided, adding stdin arg to sudo
		cmdPrefix += "-S "
	}
	if command.RunAsUser != "" && command.RunAsUser != "root" {
		// Non-root other user requested, adding su to sudo
		cmdPrefix += "-u " + command.RunAsUser + " "
	}
	if command.DisableSudo {
		// No sudo requested, remove sudo prefix
		cmdPrefix = ""
	}

	// Add prefix to command
	command.Raw = cmdPrefix + command.Raw

	logctx.LogEvent(ctx, logctx.VerbosityDebug, logctx.InfoLog, "  Running command '%s'\n", command.Raw)

	err = session.Start(command.Raw)
	if err != nil {
		err = fmt.Errorf("failed to start command: %w", err)
		return
	}

	// Only use stdin when sudo is required
	if !command.DisableSudo {
		_, err = stdin.Write([]byte(sudoPassword))
		if err != nil {
			err = fmt.Errorf("failed to write to command stdin: %w", err)
			return
		}

		err = stdin.Close()
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				// End of file is not an error - reset err and don't return
				err = nil
			} else {
				err = fmt.Errorf("failed to close stdin: %w", err)
				return
			}
		}
	}

	// Setups for timeout, output streaming, error handling
	maxExecutionTime := time.Duration(command.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), maxExecutionTime)
	defer cancel()

	var stdoutBuffer strings.Builder
	teeReader := io.TeeReader(stdout, &stdoutBuffer)

	var commandstderr []byte
	var exitStatusZero bool

	if command.StreamStdout {
		// channel scoped only here
		errChannel := make(chan error)

		go func() {
			_, err := io.Copy(os.Stdout, teeReader)
			if err != nil {
				errChannel <- fmt.Errorf("error streaming remote command stdout to program stdout: %w", err)
				return
			}
			errChannel <- nil
		}()

		err = <-errChannel
		if err != nil {
			err = fmt.Errorf("error streaming remote command stdout to program stdout: %w", err)
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
				err = fmt.Errorf("error reading error from command '%s': %w", command.Raw, errorsError)
				return
			}

			if strings.Contains(string(commandstderr), "sudo: a terminal is required to read the password") {
				// Remove ambiguous sudo errors about missing required password - error is on our side
				err = fmt.Errorf("internal failure: command '%s' attempted to run with sudo with no given password but password was required", command.Raw)
				return
			} else {
				// Return commands error
				err = fmt.Errorf("error with command '%s': %w: %s", command.Raw, err, string(commandstderr))
				return
			}
		} else {
			// nil from session.Wait() means exit status zero from the command
			exitStatusZero = true
		}
	// Timer finishes before command
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		_ = session.Close()
		err = fmt.Errorf("closed ssh session: exceeded timeout (%d seconds) for command '%s'", command.Timeout, command.Raw)
		return
	}

	commandstderr, err = io.ReadAll(stderr)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %w", err)
		return
	}

	commandError := string(commandstderr)

	if command.StreamStdout {
		commandOutput = stdoutBuffer.String()
	} else {
		var commandstdout []byte
		commandstdout, err = io.ReadAll(stdout)
		if err != nil {
			err = fmt.Errorf("failed to read stdout buffer: %w", err)
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
