// controller
package main

import (
	"bytes"
	"context"
	"crypto/hmac"
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

	"github.com/bramvdbogaerde/go-scp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ###########################################
//      SSH/Connection HANDLING
// ###########################################

// Given an identity file, determines if its a public or private key, and loads the private key (sometimes from the SSH agent)
// Also retrieves key algorithm type for later ssh connect
func SSHIdentityToKey(SSHIdentityFile string) (PrivateKey ssh.Signer, KeyAlgo string, err error) {
	// Load SSH private key
	// Parse out which is which here and if pub key use as id for agent keychain
	var SSHKeyType string

	// Load identity from file
	SSHIdentity, err := os.ReadFile(expandHomeDirectory(SSHIdentityFile))
	if err != nil {
		err = fmt.Errorf("ssh identity file: %v", err)
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
	if SSHKeyType == "public" {
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
			err = fmt.Errorf("ssh agent: %v", err)
			return
		}

		// Establish new client with agent
		sshAgent := agent.NewClient(AgentConn)

		// Get list of keys in agent
		var sshAgentKeys []*agent.Key
		sshAgentKeys, err = sshAgent.List()
		if err != nil {
			err = fmt.Errorf("ssh agent key list: %v", err)
			return
		}

		// Ensure keys are already loaded
		if len(sshAgentKeys) == 0 {
			err = fmt.Errorf("no keys found in agent (Did you forget something?)")
			return
		}

		// Parse public key from identity
		var PublicKey ssh.PublicKey
		PublicKey, _, _, _, err = ssh.ParseAuthorizedKey(SSHIdentity)
		if err != nil {
			err = fmt.Errorf("invalid public key in identity file: %v", err)
			return
		}

		// Add key algorithm to return value for later connect
		KeyAlgo = PublicKey.Type()

		// Get signers from agent
		var signers []ssh.Signer
		signers, err = sshAgent.Signers()
		if err != nil {
			err = fmt.Errorf("ssh agent signers: %v", err)
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
			err = fmt.Errorf("invalid private key in identity file: %v", err)
			return
		}

		// Add key algorithm to return value for later connect
		KeyAlgo = PrivateKey.PublicKey().Type()
	} else if SSHKeyType == "encrypted" {
		// Ask user for key password
		var passphrase string
		passphrase, err = promptUserForSecret("Enter passphrase for the SSH key `%s`: ", SSHIdentityFile)
		if err != nil {
			return
		}

		// Decrypt and parse private key with password
		PrivateKey, err = ssh.ParsePrivateKeyWithPassphrase(SSHIdentity, []byte(passphrase))
		if err != nil {
			err = fmt.Errorf("invalid encrypted private key in identity file: %v", err)
			return
		}

		// Add key algorithm to return value for later connect
		KeyAlgo = PrivateKey.PublicKey().Type()
	} else {
		err = fmt.Errorf("unknown identity file format")
		return
	}

	return
}

// Validates endpoint address and port, then combines both strings
func ParseEndpointAddress(endpointIP string, Port string) (endpointSocket string, err error) {
	// Use regex for v4 match
	IPv4RegEx := regexp.MustCompile(`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}$`)

	// Verify endpoint Port
	endpointPort, _ := strconv.Atoi(Port)
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

// Handle building client config and connection to remote host
// Attempts to automatically recover from some errors like no route to host by waiting a bit
func connectToSSH(endpointSocket string, endpointUser string, LoginPassword string, PrivateKey ssh.Signer, keyAlgorithm string) (client *ssh.Client, err error) {
	// Setup config for client
	SSHconfig := &ssh.ClientConfig{
		User: endpointUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(PrivateKey),
			ssh.Password(LoginPassword),
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
		printMessage(VerbosityProgress, "Endpoint %s: Establishing connection to SSH server (%d/%d)\n", endpointSocket, attempts, maxConnectionAttempts)

		// Connect to the SSH server direct
		client, err = ssh.Dial("tcp", endpointSocket, SSHconfig)

		// Determine if error is recoverable
		if err != nil {
			if strings.Contains(err.Error(), "no route to host") {
				printMessage(VerbosityProgress, "Endpoint %s: No route to SSH server (%d/%d)\n", endpointSocket, attempts, maxConnectionAttempts)
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

// Custom HostKeyCallback for validating remote public key against known pub keys
// If unknown, will ask user if it should trust the remote host
func hostKeyCallback(hostname string, remote net.Addr, PubKey ssh.PublicKey) (err error) {
	// Turn remote address into format used with known_hosts file entries
	cleanHost, _, err := net.SplitHostPort(remote.String())
	if err != nil {
		err = fmt.Errorf("error with ssh server key check: unable to determine hostname in address: %v", err)
		return
	}

	// If the remote addr is IPv6, extract the address part
	// Only inside the brackets - OpenSSH does not include brackets when checking against known_hosts
	if strings.Contains(cleanHost, "]") {
		cleanHost = strings.TrimPrefix(cleanHost, "[")
		cleanHost = strings.TrimSuffix(cleanHost, "]")
	}

	// Convert ssh line protocol public key to known_hosts encoding
	remotePubKey := base64.StdEncoding.EncodeToString(PubKey.Marshal())

	// Get the public key type
	pubKeyType := PubKey.Type()

	// Find an entry that matches the host we are handshaking with
	for _, knownhostkey := range config.KnownHosts {
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

		// Retrieve fields from known_hosts hash section
		salt := knownHostsPart[1]
		hashedKnownHost := knownHostsPart[2]
		knownkeysPart := strings.Fields(knownhost[1])

		// Ensure Key section has at least algorithm and key fields
		if len(knownkeysPart) < 2 {
			continue
		}

		// Hash the cleaned host name with the salt from known_hosts line
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

		// Convert hash hosts name to hex base64
		hashedHost := base64.StdEncoding.EncodeToString(hashed)

		// Compare hashed values of host and known_host host
		if hashedHost == hashedKnownHost {
			// Grab just the key part from known_hosts
			localPubKey := strings.Join(knownkeysPart[1:], " ")
			// Compare public keys
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
	fmt.Printf("Host %s not in known_hosts. Key: %s %s\n", cleanHost, pubKeyType, remotePubKey)
	addToKnownHosts, err := promptUser("Do you want to add this key to known_hosts? [y/N/all/skip]: ")
	if err != nil {
		return
	}
	addToKnownHosts = strings.TrimSpace(addToKnownHosts)
	addToKnownHosts = strings.ToLower(addToKnownHosts)

	// Parse user response
	if addToKnownHosts == "all" {
		// User wants to trust all future pub key prompts 'all' implies 'yes' to this first host key
		// For the duration of this program run, all unknown remote host keys will be added to known_hosts
		addAllUnknownHosts = true
	} else if addToKnownHosts == "skip" {
		// Continue connection, but don't write host key
		return
	} else if addToKnownHosts != "y" {
		// User did not say yes, abort connection
		err = fmt.Errorf("not continuing with connection to %s", cleanHost)
		return
	}

	// Add remote pubkey to known_hosts file
	err = writeKnownHost(cleanHost, pubKeyType, remotePubKey)
	if err != nil {
		return
	}

	// SSH is authorized to proceed connection to host
	return
}

// Writes new public key for remote host to known_hosts file
func writeKnownHost(cleanHost string, pubKeyType string, remotePubKey string) (err error) {
	// Show progress to user
	printMessage(VerbosityStandard, "Writing new host entry in known_hosts... ")

	// Get hashed host
	hashSection := knownhosts.HashHostname(cleanHost)

	// New line to be added
	newKnownHost := hashSection + " " + pubKeyType + " " + remotePubKey

	// Lock file for writing - unlock on func return
	KnownHostMutex.Lock()
	defer KnownHostMutex.Unlock()

	// Open the known_hosts file
	knownHostsfile, err := os.OpenFile(config.KnownHostsFilePath, os.O_APPEND|os.O_WRONLY, 0644)
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
	printMessage(VerbosityStandard, "Success\n")
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
func SCPDownload(client *ssh.Client, remoteFilePath string) (fileContent string, err error) {
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
	remoteFileBytes := localTransferBuffer.Bytes()
	fileContent = string(remoteFileBytes)
	return
}

// Runs the given remote ssh command with sudo
// runAs input will change to the user using sudo if not root
// disableSudo will determine if command runs with sudo or not (default, will always use sudo)
// Empty sudoPassword will run without assuming the user account doesn't require any passwords
// timeout is the max execution time in seconds for the given command
func RunSSHCommand(client *ssh.Client, command string, runAs string, disableSudo bool, sudoPassword string, timeout int) (CommandOutput string, err error) {
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
	command = cmdPrefix + command

	printMessage(VerbosityDebug, "  Running command '%s'\n", command)

	// Start the command
	err = session.Start(command)
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
	var Commandstderr []byte
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
			Commandstderr, errorsError = io.ReadAll(stderr)
			if errorsError != nil {
				// Return at any errors reading the command error
				err = fmt.Errorf("error reading error from command '%s': %v", command, errorsError)
				return
			}

			// Return commands error
			err = fmt.Errorf("error with command '%s': %v: %s", command, err, Commandstderr)
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
	Commandstdout, err := io.ReadAll(stdout)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// Read commands error output from session
	Commandstderr, err = io.ReadAll(stderr)
	if err != nil {
		err = fmt.Errorf("error reading from io.Reader: %v", err)
		return
	}

	// Convert bytes to string
	CommandOutput = string(Commandstdout)
	CommandError := string(Commandstderr)

	// If the command had an error on the remote side and session indicated non-zero exit status
	if CommandError != "" && !exitStatusZero {
		// Only return valid errors
		if strings.Contains(CommandError, "[sudo] password for") {
			// Sudo puts password prompts into stderr when running with '-S'
			err = nil
		} else {
			err = fmt.Errorf("%s", CommandError)
			return
		}
	}

	return
}

func executeScript(sshClient *ssh.Client, SudoPassword string, remoteTransferBuffer string, scriptInterpreter string, remoteFilePath string, scriptFileBytes []byte, scriptHash string) (out string, err error) {
	// Upload script contents
	err = SCPUpload(sshClient, scriptFileBytes, remoteTransferBuffer)
	if err != nil {
		return
	}

	// Move script into execution location
	command := "mv " + remoteTransferBuffer + " " + remoteFilePath
	_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, SudoPassword, 10)
	if err != nil {
		return
	}

	// Hash remote script file
	command = "sha256sum " + remoteFilePath
	remoteScriptHash, err := RunSSHCommand(sshClient, command, "root", config.DisableSudo, SudoPassword, 90)
	if err != nil {
		return
	}
	// Parse hash command output to get just the hex
	remoteScriptHash = SHA256RegEx.FindString(remoteScriptHash)

	printMessage(VerbosityFullData, "Remote Script Hash '%s'\n", remoteScriptHash)

	// Ensure original hash is identical to remote hash
	if remoteScriptHash != scriptHash {
		err = fmt.Errorf("remote script hash does not match local hash, bailing on execution")
		return
	}

	// Change permissions on remote file
	command = "chmod 700 " + remoteFilePath
	_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, SudoPassword, 10)
	if err != nil {
		return
	}

	// Execute script
	command = scriptInterpreter + " " + remoteFilePath
	out, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, SudoPassword, 900)
	if err != nil {
		return
	}

	// Cleanup: Remove script
	command = "rm " + remoteFilePath
	_, err = RunSSHCommand(sshClient, command, "root", config.DisableSudo, SudoPassword, 10)
	if err != nil {
		return
	}

	return
}
