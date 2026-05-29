package sshinternal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"scmp/internal/config"
	"scmp/internal/fsops"
	"scmp/internal/global"
	"scmp/internal/input"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ###########################################
//      SSH/Connection HANDLING
// ###########################################

var knownHostMutex sync.Mutex

// Given an identity file, determines if its a public or private key, and loads the private key (sometimes from the SSH agent)
// Also retrieves key algorithm type for later ssh connect
func IdentityToKey(ctx context.Context, SSHIdentityFile string) (privateKey ssh.Signer, keyAlgo string, err error) {
	// Load SSH private key
	// Parse out which is which here and if pub key use as id for agent keychain
	var SSHKeyType string

	// Load identity from file
	SSHIdentityFile, err = fsops.ExpandHomeDirectory(SSHIdentityFile)
	if err != nil {
		err = fmt.Errorf("failed to resolve absolute path for '%s': %w", SSHIdentityFile, err)
		return
	}

	SSHIdentity, err := os.ReadFile(SSHIdentityFile)
	if err != nil {
		err = fmt.Errorf("ssh identity file: %w", err)
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
		var agentConn net.Conn
		agentConn, err = net.Dial("unix", agentSock)
		if err != nil {
			err = fmt.Errorf("ssh agent: %w", err)
			return
		}

		// Establish new client with agent
		sshAgent := agent.NewClient(agentConn)

		// Get list of keys in agent
		var sshAgentKeys []*agent.Key
		sshAgentKeys, err = sshAgent.List()
		if err != nil {
			err = fmt.Errorf("ssh agent key list: %w", err)
			return
		}

		// Ensure keys are already loaded
		if len(sshAgentKeys) == 0 {
			err = fmt.Errorf("no keys found in agent (Did you forget something?)")
			return
		}

		// Parse public key from identity
		var publicKey ssh.PublicKey
		publicKey, _, _, _, err = ssh.ParseAuthorizedKey(SSHIdentity)
		if err != nil {
			err = fmt.Errorf("invalid public key in identity file: %w", err)
			return
		}

		// Add key algorithm to return value for later connect
		keyAlgo = publicKey.Type()

		// Get signers from agent
		var signers []ssh.Signer
		signers, err = sshAgent.Signers()
		if err != nil {
			err = fmt.Errorf("ssh agent signers: %w", err)
			return
		}

		// Find matching private key to local public key
		for _, sshAgentKey := range signers {
			// Obtain public key from private key in keyring
			sshAgentPubKey := sshAgentKey.PublicKey()

			// Break if public key of priv key in agent matches public key from identity
			if bytes.Equal(sshAgentPubKey.Marshal(), publicKey.Marshal()) {
				privateKey = sshAgentKey
				break
			}
		}
	} else if SSHKeyType == "private" {
		privateKey, err = ssh.ParsePrivateKey(SSHIdentity)
		if err != nil {
			err = fmt.Errorf("invalid private key in identity file: %w", err)
			return
		}

		// Add key algorithm to return value for later connect
		keyAlgo = privateKey.PublicKey().Type()
	} else if SSHKeyType == "encrypted" {
		var passphrase []byte
		passphrase, err = input.AskUserSecret(ctx, "Enter passphrase for the SSH key "+SSHIdentityFile, "")
		if err != nil {
			return
		}

		// Decrypt and parse private key with password
		privateKey, err = ssh.ParsePrivateKeyWithPassphrase(SSHIdentity, passphrase)
		if err != nil {
			err = fmt.Errorf("invalid encrypted private key in identity file: %w", err)
			return
		}

		// Add key algorithm to return value for later connect
		keyAlgo = privateKey.PublicKey().Type()
	} else {
		err = fmt.Errorf("unknown identity file format")
		return
	}

	return
}

// Validates endpoint address and port, then combines both strings
func ParseEndpointAddress(endpointIP string, Port string) (endpointSocket string, err error) {
	// Verify endpoint Port
	endpointPort, _ := strconv.Atoi(Port)
	if endpointPort <= 0 || endpointPort > 65535 {
		err = fmt.Errorf("endpoint port number '%d' out of range", endpointPort)
		return
	}

	// Verify IP address
	IPCheck := net.ParseIP(endpointIP)
	if IPCheck == nil {
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

// Custom HostKeyCallback for validating remote public key against known pub keys
// If unknown, will ask user if it should trust the remote host
func hostKeyCallback(ctx context.Context, hostname string, remote net.Addr, PubKey ssh.PublicKey) (err error) {
	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	_ = hostname

	const environmentUnknownSSHHostKey string = "UnknownSSHHostKeyAction"

	// Turn remote address into format used with known_hosts file entries
	cleanHost, _, err := net.SplitHostPort(remote.String())
	if err != nil {
		err = fmt.Errorf("error with ssh server key check: unable to determine hostname in address: %w", err)
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
			err = fmt.Errorf("error decoding salt: %w", err)
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

	// If global was set, don't ask user to add unknown key
	if config.AddAllUnknownHosts {
		err = writeKnownHost(config.KnownHostsFilePath, cleanHost, pubKeyType, remotePubKey)
		if err != nil {
			return
		}
		return
	}

	// If env var is set, use as prompt answer
	envaddToKnownHosts := os.Getenv(environmentUnknownSSHHostKey)

	// Key was not found in known_hosts - Prompt user
	fmt.Printf("Host %s not in known_hosts. Key: %s %s\n", cleanHost, pubKeyType, remotePubKey)
	var addToKnownHosts string
	if envaddToKnownHosts != "" {
		// Put environment answer into answer var - also show answered prompt
		addToKnownHosts = envaddToKnownHosts
		fmt.Printf("Do you want to add this key to known_hosts? [y/N/all/skip]: %s\n", addToKnownHosts)
	} else {
		addToKnownHosts, err = input.AskUser(ctx, "Do you want to add this key to known_hosts? [y/N/all/skip]", "")
		if err != nil {
			return
		}
	}
	addToKnownHosts = strings.TrimSpace(addToKnownHosts)
	addToKnownHosts = strings.ToLower(addToKnownHosts)

	// Parse user response
	if addToKnownHosts == "all" {
		// User wants to trust all future pub key prompts 'all' implies 'yes' to this first host key
		// For the duration of this program run, all unknown remote host keys will be added to known_hosts
		config.AddAllUnknownHosts = true
	} else if addToKnownHosts == "skip" {
		// Continue connection, but don't write host key
		return
	} else if addToKnownHosts != "y" {
		// User did not say yes, abort connection
		err = fmt.Errorf("not continuing with connection to %s", cleanHost)
		return
	}

	// Add remote pubkey to known_hosts file
	err = writeKnownHost(config.KnownHostsFilePath, cleanHost, pubKeyType, remotePubKey)
	if err != nil {
		return
	}

	// SSH is authorized to proceed connection to host
	return
}

// Writes new public key for remote host to known_hosts file
func writeKnownHost(knownHostsFilePath string, cleanHost string, pubKeyType string, remotePubKey string) (err error) {
	// Show progress to user
	fmt.Println("Writing new host entry in known_hosts... ")

	// Get hashed host
	hashSection := knownhosts.HashHostname(cleanHost)

	// New line to be added
	newKnownHost := hashSection + " " + pubKeyType + " " + remotePubKey

	// Lock file for writing - unlock on func return
	knownHostMutex.Lock()
	defer knownHostMutex.Unlock()

	knownHostsfile, err := os.OpenFile(knownHostsFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		err = fmt.Errorf("failed to open known_hosts file: %w", err)
		return
	}
	defer func() {
		lerr := knownHostsfile.Close()
		if err == nil && lerr != nil {
			err = lerr
		}
	}()

	if _, err = knownHostsfile.WriteString(newKnownHost + "\n"); err != nil {
		err = fmt.Errorf("failed to write new known host to known_hosts file: %w", err)
		return
	}
	fmt.Printf("Success\n")

	return
}
