package main

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

// ###################################
//      CONNECTION HANDLING
// ###################################

func RunSSHServer(config Config, progVersion string) {
	printMessage(VerbosityStandard, "Starting SCM Deployer SSH server...\n")

	// Load SSH private key
	privateKey, err := os.ReadFile(config.SSHServer.SSHPrivKeyFile)
	logError("Error loading SSH Private Key", err, true)

	PrivateKey, err := ssh.ParsePrivateKey(privateKey)
	logError("Error parsing SSH Private Key", err, true)

	// Get socket address
	var socketAddr string
	if strings.Contains(config.SSHServer.ListenAddress, ":") {
		socketAddr = "[" + config.SSHServer.ListenAddress + "]" + ":" + config.SSHServer.ListenPort
	} else {
		socketAddr = config.SSHServer.ListenAddress + ":" + config.SSHServer.ListenPort
	}

	// Set up SSH server config and authentication function
	sshServerVersion := "SSH-2.0-OpenSSH_" + progVersion // embed current deployer version in SSH version
	sshConfig := &ssh.ServerConfig{
		ServerVersion: sshServerVersion,
		PublicKeyAuthAlgorithms: []string{
			PrivateKey.PublicKey().Type(),
		},
		NoClientAuth: false,
		MaxAuthTries: 2,
	}
	sshConfig.AddHostKey(PrivateKey)

	// Verify client function
	sshConfig.PublicKeyCallback = func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		// Verify Username against config
		if conn.User() != config.SSHServer.AuthorizedUser {
			return nil, fmt.Errorf("username is not authorized to log in")
		}

		// Verify Client Key against config
		ClientKey := base64.StdEncoding.EncodeToString(key.Marshal())
		var UserIsAuthorized bool
		for _, AuthorizedKey := range config.SSHServer.AuthorizedKeys {
			// Parse out just the key section
			AuthPubKey := strings.SplitN(AuthorizedKey, " ", 3)
			AuthKey := AuthPubKey[1]

			// Identify if user key is authorized
			UserIsAuthorized = false
			if ClientKey == AuthKey {
				UserIsAuthorized = true
			}
		}

		// Deny if user key is not authorized
		if !UserIsAuthorized {
			return nil, fmt.Errorf("client key is not authorized to log in")
		}

		// Return authorization
		printMessage(VerbosityStandard, "Authorized connection from %s for user %s authenticated by %s key\n", conn.RemoteAddr(), conn.User(), key.Type())
		return nil, nil
	}

	// If user requested dry-run, gracefully exit
	if dryRunRequested {
		printMessage(VerbosityStandard, "deployer: server startup test is successful\n")
		return
	}

	// Start Listener
	listener, err := net.Listen("tcp", socketAddr)
	logError("Failed to listen on port", err, true)
	defer listener.Close()

	printMessage(VerbosityStandard, "SCM Deployer (%s) SSH server started on %s\n", progVersion, socketAddr)

	// Processing incoming connections linearly - no more than one at a time
	for {
		printMessage(VerbosityProgress, "Awaiting new connection\n")

		// Accept a new connection
		NewConnection, err := listener.Accept()
		if err != nil {
			logError("Connection error", fmt.Errorf("failed to accept connection: %v", err), false)
			continue
		}

		printMessage(VerbosityProgress, "Received new connection\n")

		// Setup Signal Handling Channel
		signalReceived := make(chan os.Signal, 1)

		// Start blocking SIGTERM signals while connection is being handled
		signal.Notify(signalReceived, syscall.SIGTERM)

		// Establish an SSH connection
		sshConn, chans, reqs, err := ssh.NewServerConn(NewConnection, sshConfig)
		if err != nil {
			logError("SSH Connection error", fmt.Errorf("failed to establish connection: %v", err), false)
			continue
		}

		// Discard all global out-of-band Requests
		go ssh.DiscardRequests(reqs)

		printMessage(VerbosityProgress, "Handling new channel\n")

		// Handle incoming channel requests
		for newChannel := range chans {
			// Error out channels other than 'session'
			if newChannel.ChannelType() != "session" {
				logError("SSH channel error", fmt.Errorf("unauthorized channel type requested: %s", newChannel.ChannelType()), false)
				return
			}

			// Handle the channel (e.g., execute commands, etc.)
			handleChannel(newChannel)
		}
		printMessage(VerbosityStandard, "Closed connection from %s for user %s\n", sshConn.RemoteAddr(), sshConn.User())

		// Check for sigterm, break processing loop and shutdown server gracefully
		select {
		case <-signalReceived:
			// SIGTERM received, exit program
			printMessage(VerbosityStandard, "SCM Deployer (%s) SSH server shut down\n", progVersion)
			return
		default:
			// No SIGTERM, continue processing connections
			signal.Stop(signalReceived)
			close(signalReceived)

			// Add default connection processing rate limit
			time.Sleep(time.Duration(connectionRateLimit) * time.Millisecond)
			continue
		}
	}
}
