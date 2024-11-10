// controller
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ###########################################
//      SSH/Connection HANDLING
// ###########################################

func SSHIdentityToKey(SSHIdentityFile string, UseSSHAgent bool) (PrivateKey ssh.Signer, err error) {
	// Load SSH private key
	// Parse out which is which here and if pub key use as id for agent keychain
	var SSHKeyType string

	// Load identity from file
	SSHIdentity, err := os.ReadFile(SSHIdentityFile)
	if err != nil {
		err = fmt.Errorf("failed to read ssh identity file: %v", err)
		return
	}

	// Determine key type
	_, err = ssh.ParsePrivateKey(SSHIdentity)
	if err == nil {
		SSHKeyType = "private"
	} else if _, encryptedKey := err.(*ssh.PassphraseMissingError); encryptedKey {
		SSHKeyType = "encrypted"
	}

	_, _, _, _, err = ssh.ParseAuthorizedKey(SSHIdentity)
	if err == nil {
		SSHKeyType = "public"
	}

	// Load key from keyring if requested
	if UseSSHAgent {
		// Ensure user supplied identity is a public key if requesting to use agent
		if SSHKeyType != "public" {
			err = fmt.Errorf("identity file is not a public key, cannot use agent without public key")
			return
		}

		// Find auth socket for agent
		agentSock := os.Getenv("SSH_AUTH_SOCK")
		if agentSock == "" {
			err = fmt.Errorf("cannot use agent, '%s' environment variable is not set", agentSock)
			return
		}

		// Connect to agent socket
		var AgentConn net.Conn
		AgentConn, err = net.Dial("unix", agentSock)
		if err != nil {
			err = fmt.Errorf("failed to connect to agent: %v", err)
			return
		}

		// Establish new client with agent
		sshAgent := agent.NewClient(AgentConn)

		// Get list of keys in agent
		var sshAgentKeys []*agent.Key
		sshAgentKeys, err = sshAgent.List()
		if err != nil {
			err = fmt.Errorf("failed to get list of keys from agent: %v", err)
			return
		}

		// Ensure keys are already loaded
		if len(sshAgentKeys) == 0 {
			err = fmt.Errorf("no keys found in agent")
			return
		}

		// Parse public key from identity
		var PublicKey ssh.PublicKey
		PublicKey, _, _, _, err = ssh.ParseAuthorizedKey(SSHIdentity)
		if err != nil {
			err = fmt.Errorf("failed to parse public key from identity file: %v", err)
			return
		}

		// Get signers from agent
		var signers []ssh.Signer
		signers, err = sshAgent.Signers()
		if err != nil {
			err = fmt.Errorf("failed to get signers from agent: %v", err)
			return
		}

		// Find matching private key to local public key
		for _, sshAgentKey := range signers {
			// Obtain public key from private key in keyring
			sshAgentPubKey := sshAgentKey.PublicKey()

			// Break if public key of priv key in agent matches public key from identity
			if bytes.Equal(sshAgentPubKey.Marshal(), PublicKey.Marshal()) {
				PrivateKey = sshAgentKey
				break
			}
		}
	} else if SSHKeyType == "private" {
		// Parse the private key
		PrivateKey, err = ssh.ParsePrivateKey(SSHIdentity)
		if err != nil {
			err = fmt.Errorf("failed to parse private key from identity file: %v", err)
			return
		}
	} else if SSHKeyType == "encrypted" {
		// Ask user for key password
		fmt.Printf("Enter passphrase for the SSH key `%s`: ", SSHIdentityFile)

		// Read password from input
		var passphrase string
		reader := bufio.NewReader(os.Stdin)
		passphrase, err = reader.ReadString('\n')
		if err != nil {
			return
		}

		// Remove newline char from password
		passphrase = passphrase[:len(passphrase)-1]

		// Decrypt and parse private key with password
		PrivateKey, err = ssh.ParsePrivateKeyWithPassphrase(SSHIdentity, []byte(passphrase))
		if err != nil {
			err = fmt.Errorf("failed to parse encrypted private key from identity file: %v", err)
			return
		}
	} else {
		err = fmt.Errorf("unknown identity file format")
		return
	}

	return
}

func ParseEndpointAddress(endpointIP string, endpointPort int) (endpointSocket string, err error) {
	// Use regex for v4 match
	IPv4RegEx := regexp.MustCompile(`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}$`)

	// Verify endpoint Port
	if endpointPort <= 0 || endpointPort > 65535 {
		err = fmt.Errorf("endpoint port number '%d' out of range", endpointPort)
		return
	}

	// Verify IP address
	IPCheck := net.ParseIP(endpointIP)
	if IPCheck == nil && !IPv4RegEx.MatchString(endpointIP) {
		err = fmt.Errorf("endpoint ip '%s' is not valid", endpointIP)
		return
	}

	// Get endpoint socket by ipv6 or ipv4
	if strings.Contains(endpointIP, ":") {
		endpointSocket = "[" + endpointIP + "]" + ":" + strconv.Itoa(endpointPort)
	} else {
		endpointSocket = endpointIP + ":" + strconv.Itoa(endpointPort)
	}

	return
}

// Handle connecting to remote SSH server and maybe recovering from some failures
func connectToSSH(endpointSocket string, endpointUser string, PrivateKey ssh.Signer) (client *ssh.Client, err error) {
	// Setup config for client
	//      Need to only use a single key algorithm type to avoid getting wrong public key back from the server for the local known_hosts check
	//  Supposedly 'fixed' by allowing the client to specify which algo to use when connecting in https://github.com/golang/go/issues/11722
	//  Yeah bud, totally. Let me just create 3 connections per host just to try and find a match in known_hosts... fucking stupid.
	//  Its ed25519 for my env, change it if you want... Beware it must be the same algo used across all of your ssh servers
	SSHconfig := &ssh.ClientConfig{
		User: endpointUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(PrivateKey),
		},
		// Some IPS rules flag on GO's ssh client string
		ClientVersion: "SSH-2.0-OpenSSH_9.8p1",
		// Don't add multiple values here, you will experience handshake errors when verifying some server pub keys
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoED25519,
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	// Only attempt connection x times
	maxConnectionAttempts := 3

	// Loop so some network errors can recover and try again
	for attempts := 0; attempts <= maxConnectionAttempts; attempts++ {
		// Connect to the SSH server
		client, err = ssh.Dial("tcp", endpointSocket, SSHconfig)

		// Determine if error is recoverable
		if err != nil {
			if strings.Contains(err.Error(), "no route to host") {
				// Re-attempt after waiting for network path
				time.Sleep(200 * time.Millisecond)
				continue
			} else {
				// All other errors, bail from connection attempts
				return
			}
		} else {
			// Connection worked
			break
		}
	}

	return
}

// Custom HostKeyCallback for checking/writing local known_hosts file
func hostKeyCallback(hostname string, remote net.Addr, PubKey ssh.PublicKey) (err error) {
	// Turn remote address into format used with known_hosts file entries
	cleanHost, _, err := net.SplitHostPort(remote.String())
	if err != nil {
		err = fmt.Errorf("error with ssh server key check: unable to determine hostname in address: %v", err)
		return
	}

	// If the remote addr is IPv6, extract the address part (inside brackets)
	if strings.Contains(cleanHost, "]") {
		cleanHost = strings.TrimPrefix(cleanHost, "[")
		cleanHost = strings.TrimSuffix(cleanHost, "]")
	}

	// Convert ssh line protocol public key to known_hosts encoding
	remotePubKey := base64.StdEncoding.EncodeToString(PubKey.Marshal())

	// Get the public key type
	pubKeyType := PubKey.Type()

	// Find an entry that matches the host we are handshaking with
	for _, knownhostkey := range knownhosts {
		// Separate the public key section from the hashed host section
		knownhostkey = strings.TrimPrefix(knownhostkey, "|")
		knownhost := strings.SplitN(knownhostkey, " ", 2)
		if len(knownhost) < 2 {
			continue
		}

		// Only Process hashed lines of known_hosts
		knownHostsPart := strings.Split(knownhost[0], "|")
		if len(knownHostsPart) < 3 || knownHostsPart[0] != "1" {
			continue
		}

		salt := knownHostsPart[1]
		hashedKnownHost := knownHostsPart[2]
		knownkeysPart := strings.Fields(knownhost[1])

		// Ensure Key section has at least algorithm and key fields
		if len(knownkeysPart) < 2 {
			continue
		}

		// Hash the cleaned host name with the salt
		var saltBytes []byte
		saltBytes, err = base64.StdEncoding.DecodeString(salt)
		if err != nil {
			err = fmt.Errorf("error decoding salt: %v", err)
			return
		}

		// Create the HMAC-SHA1 using the salt as the key
		hmacAlgo := hmac.New(sha1.New, saltBytes)
		hmacAlgo.Write([]byte(cleanHost))
		hashed := hmacAlgo.Sum(nil)

		// Return the base64 encoded result
		hashedHost := base64.StdEncoding.EncodeToString(hashed)

		// Compare hashed values of host
		if hashedHost == hashedKnownHost {
			// Grab just the key part from known_hosts
			localPubKey := strings.Join(knownkeysPart[1:], " ")
			// Compare pub keys
			if localPubKey == remotePubKey {
				// nil err means SSH is cleared to continue handshake
				return
			}
		}
	}

	// If global was set, dont ask user to add unknown key
	if addAllUnknownHosts {
		err = writeKnownHost(cleanHost, pubKeyType, remotePubKey)
		if err != nil {
			return
		}
		return
	}

	// Key was not found in known_hosts - Prompt user
	fmt.Printf("Host %s not in known_hosts. Key: %s %s\nDo you want to add this key to known_hosts? [y/N/all]: ", cleanHost, pubKeyType, remotePubKey)

	// Read user choice
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if strings.ToLower(input) == "all" {
		// For the duration of this program run, all unknown remote host keys will be added to known_hosts
		addAllUnknownHosts = true

		// 'all' implies 'yes' to this first host key
		input = "y"
	}

	// User did not say yes, abort connection
	if strings.ToLower(input) != "y" {
		err = fmt.Errorf("not continuing with connection to %s", cleanHost)
		return
	}

	err = writeKnownHost(cleanHost, pubKeyType, remotePubKey)
	if err != nil {
		return
	}

	// SSH is authorized to proceed connection to host
	return
}

func writeKnownHost(cleanHost string, pubKeyType string, remotePubKey string) (err error) {
	// Show progress to user
	fmt.Printf("Writing new host entry in known_hosts... ")

	// Get Salt
	salt := make([]byte, 20)
	_, err = rand.Read(salt)
	if err != nil {
		return
	}

	// Get hashed host
	hmacAlgo := hmac.New(sha1.New, salt)
	hmacAlgo.Write([]byte(cleanHost))
	hashedHost := hmacAlgo.Sum(nil)

	// New line to be added
	newKnownHost := "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" + base64.StdEncoding.EncodeToString(hashedHost) + " " + pubKeyType + " " + remotePubKey

	// Lock file for writing - unlock on func return
	KnownHostMutex.Lock()
	defer KnownHostMutex.Unlock()

	// Open the known_hosts file
	knownHostsfile, err := os.OpenFile(knownHostsFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		err = fmt.Errorf("failed to open known_hosts file: %v", err)
		return
	}
	defer knownHostsfile.Close()

	// Write the new known host string followed by a newline
	if _, err = knownHostsfile.WriteString(newKnownHost + "\n"); err != nil {
		err = fmt.Errorf("failed to write new known host to known_hosts file: %v", err)
		return
	}

	// Show progress to user
	fmt.Printf("Success\n")
	return
}

func RunSFTP(client *ssh.Client, localFileContent []byte) (err error) {
	// Open new session with ssh client
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		err = fmt.Errorf("failed to create sftp session: %v", err)
		return
	}
	defer sftpClient.Close()

	// Context for SFTP wait - add timeout
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Wait for the file transfer
	errChannel := make(chan error)
	go func() {
		// Open remote file
		remoteTempFile, err := sftpClient.Create(tmpRemoteFilePath)
		if err != nil {
			errChannel <- err
			return
		}

		// Write file contents to remote file
		_, err = remoteTempFile.Write([]byte(localFileContent))
		if err != nil {
			errChannel <- err
			return
		}

		// Signal we are done transferring
		errChannel <- nil
	}()
	// Block until errChannel is done, then parse errors
	select {
	// Transfer finishes before timeout with error
	case err = <-errChannel:
		if err != nil {
			if strings.Contains(err.Error(), "permission denied") {
				err = fmt.Errorf("unable to write to %s (is it writable by the user?): %v", tmpRemoteFilePath, err)
			}
			err = fmt.Errorf("error with file transfer: %v", err)
			return
		}
	// Timer finishes before transfer
	case <-ctx.Done():
		sftpClient.Close()
		err = fmt.Errorf("closed ssh session, file transfer timed out")
		return
	}

	return
}

func RunSSHCommand(client *ssh.Client, command string, SudoPassword string) (CommandOutput string, err error) {
	// Check for password
	if SudoPassword == "" {
		err = fmt.Errorf("sudo password is empty: %v", err)
		return
	}

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

	// Add sudo to command
	command = "sudo -S " + command

	// Start the command
	err = session.Start(command)
	if err != nil {
		err = fmt.Errorf("failed to start command: %v", err)
		return
	}

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

	// Context for command wait - 60 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Wait for the command to finish with timeout
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
			CommandError, _ := io.ReadAll(stderr)
			err = fmt.Errorf("error with command '%s': %v : %s", command, err, CommandError)
			return
		}
	// Timer finishes before command
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		session.Close()
		err = fmt.Errorf("closed ssh session, command %s timed out", command)
		return
	}

	// Read commands output from session
	Commandstdout, err := io.ReadAll(stdout)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// Read commands error output from session
	CommandError, err := io.ReadAll(stderr)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// Convert bytes to string
	CommandOutput = string(Commandstdout)

	// If the command had an error on the remote side
	if string(CommandError) != "" {
		err = fmt.Errorf("%s", CommandError)
		return
	}

	return
}
