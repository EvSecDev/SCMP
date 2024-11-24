package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

// ###################################
//      GLOBAL VARIABLES
// ###################################

// Main Yaml config format
type Config struct {
	UpdaterProgram string `yaml:"UpdaterProgram"`
	SSHServer      struct {
		ListenAddress  string   `yaml:"ListenAddress"`
		ListenPort     string   `yaml:"ListenPort"`
		SSHPrivKeyFile string   `yaml:"SSHPrivKeyFile"`
		AuthorizedUser string   `yaml:"AuthorizedUser"`
		AuthorizedKeys []string `yaml:"AuthorizedKeys"`
	} `yaml:"SSHServer"`
}

// ###################################
//      EXCEPTION HANDLING
// ###################################

func logError(errorDescription string, errorMessage error, FatalError bool) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}
	// Log and exit if requested
	if FatalError {
		fmt.Printf("%s: %v\n", errorDescription, errorMessage)
		os.Exit(1)
	}
	// Just print the error otherwise
	fmt.Printf("%s: %v\n", errorDescription, errorMessage)
}

// ###################################
//      MAIN - START
// ###################################

func HelpMenu() {
	fmt.Printf("Usage: %s [OPTIONS]...\n%s", os.Args[0], usage)
}

const usage = `
Options:
    -c, --config </path/to/yaml>       Path to the configuration file [default: scmpd.yaml]
    -s, --start-server                 Start the Deployer SSH Server
    -h, --help                         Show this help menu
    -V, --version                      Show version and packages
    -v, --versionid                    Show only version number

Documentation: <https://github.com/EvSecDev/SCMPusher>
`

func main() {
	progVersion := "v1.0.2"

	// Program Argument Variables
	var configFilePath string
	var startServerFlagExists bool
	var versionFlagExists bool
	var versionNumberFlagExists bool

	// Read Program Arguments
	flag.StringVar(&configFilePath, "c", "scmpd.yaml", "")
	flag.StringVar(&configFilePath, "config", "scmpd.yaml", "")
	flag.BoolVar(&startServerFlagExists, "s", false, "")
	flag.BoolVar(&startServerFlagExists, "start-server", false, "")
	flag.BoolVar(&versionFlagExists, "V", false, "")
	flag.BoolVar(&versionFlagExists, "version", false, "")
	flag.BoolVar(&versionNumberFlagExists, "v", false, "")
	flag.BoolVar(&versionNumberFlagExists, "versionid", false, "")

	// Custom help menu
	flag.Usage = HelpMenu
	flag.Parse()

	// Meta info print out
	if versionFlagExists {
		fmt.Printf("Deployer %s compiled using %s(%s) on %s architecture %s\n", progVersion, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("Packages: runtime strings io github.com/pkg/sftp encoding/base64 flag fmt golang.org/x/crypto/ssh os/exec net os bytes encoding/binary gopkg.in/yaml.v2\n")
		os.Exit(0)
	}
	if versionNumberFlagExists {
		fmt.Println(progVersion)
		os.Exit(0)
	}

	// Grab config file
	yamlConfigFile, err := os.ReadFile(configFilePath)
	logError("Error reading config file", err, true)

	if yamlConfigFile == nil {
		logError("Error reading config file", fmt.Errorf("empty file"), true)
	}

	// Parse all configuration options
	var config Config
	err = yaml.Unmarshal(yamlConfigFile, &config)
	logError("Error unmarshaling config file", err, true)

	// Start ssh server
	if startServerFlagExists {
		RunSSHServer(config, progVersion)
		os.Exit(0)
	}

	// Exit program without any arguments
	fmt.Printf("No arguments specified! Use '-h' or '--help' to guide your way.\n")
}

// ###################################
//      CONNECTION FUNCTIONS
// ###################################

func RunSSHServer(config Config, progVersion string) {
	fmt.Printf("Starting SCM Deployer SSH server...\n")

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
		fmt.Printf("Authorized connection from %s for user %s authenticated by %s key\n", conn.RemoteAddr(), conn.User(), key.Type())
		return nil, nil
	}

	// Start Listener
	listener, err := net.Listen("tcp", socketAddr)
	logError("Failed to listen on port", err, true)
	defer listener.Close()

	fmt.Printf("SCM Deployer (%s) SSH server started on %s\n", progVersion, socketAddr)

	// Processing incoming connections linearly - no more than one at a time
	for {
		// Accept a new connection
		NewConnection, err := listener.Accept()
		if err != nil {
			logError("Connection error", fmt.Errorf("failed to accept connection: %v", err), false)
			continue
		}

		// Establish an SSH connection
		sshConn, chans, reqs, err := ssh.NewServerConn(NewConnection, sshConfig)
		if err != nil {
			logError("SSH Connection error", fmt.Errorf("failed to establish connection: %v", err), false)
			continue
		}

		// Discard all global out-of-band Requests
		go ssh.DiscardRequests(reqs)

		// Handle incoming channel requests
		for newChannel := range chans {
			// Error out channels other than 'session'
			if newChannel.ChannelType() != "session" {
				logError("SSH channel error", fmt.Errorf("unauthorized channel type requested: %s", newChannel.ChannelType()), false)
				return
			}

			// Handle the channel (e.g., execute commands, etc.)
			handleChannel(newChannel, config.UpdaterProgram)
		}
		fmt.Printf("Closed connection from %s for user %s\n", sshConn.RemoteAddr(), sshConn.User())
	}
}

// ###################################
//      CHANNEL PARSING
// ###################################

// Define a handler for SSH connections
func handleChannel(newChannel ssh.NewChannel, UpdaterProgram string) {
	// Recover from panic
	defer func() {
		if r := recover(); r != nil {
			logError("Panic while processing client channel", fmt.Errorf("%v", r), false)
		}
	}()

	// Accept the channel
	channel, requests, err := newChannel.Accept()
	if err != nil {
		logError("SSH channel error", fmt.Errorf("could not accept channel: %v", err), false)
		return
	}
	defer channel.Close()

	// Loop client requests - Only allow SFTP or Exec
	for req := range requests {
		switch req.Type {
		case "exec":
			command, err := StripPayloadHeader(req.Payload)
			if err != nil {
				logError("SSH request error", fmt.Errorf("exec: failed to strip request payload header: %v", err), false)
				break
			}
			if req.WantReply {
				req.Reply(true, nil)
			}
			err = executeCommand(channel, command)
			if err != nil {
				logError("SSH request error", fmt.Errorf("failed command execution: %v", err), false)
				break
			}
		case "subsystem":
			subsystem, err := StripPayloadHeader(req.Payload)
			if err != nil {
				logError("SSH request error", fmt.Errorf("subsystem: failed to strip request payload header: %v", err), false)
				break
			}
			if subsystem != "sftp" {
				req.Reply(false, nil)
				logError("SSH request error", fmt.Errorf("received unauthorized subsystem %s", subsystem), false)
				break
			}
			if req.WantReply {
				req.Reply(true, nil)
			}
			// Handle SFTP
			err = HandleSFTP(channel)
			if err != nil {
				logError("SSH request error", fmt.Errorf("failed sftp: %v", err), false)
				break
			}
		case "update":
			// Retrieve new deployer binary from payload of request
			TransferBuffer, err := StripPayloadHeader(req.Payload)
			if err != nil {
				logError("SSH request error", fmt.Errorf("update: failed to strip request payload header: %v", err), false)
				break
			}
			req.Reply(true, nil)
			// Run updater program given the location of the new deployer binary
			command := UpdaterProgram + " -src " + TransferBuffer
			err = executeCommand(channel, command)
			if err != nil {
				logError("SSH request error", fmt.Errorf("failed updater execution: %v", err), false)
				break
			}
			// Last log entry before exit
			fmt.Printf("Received update request, running update program\n")
		default:
			logError("SSH request error", fmt.Errorf("unauthorized request type %s received", req.Type), false)
			req.Reply(false, nil) // Reject unknown requests
		}
		channel.Close()
	}
}

// ###################################
//      REQUEST HANDLING
// ###################################

func executeCommand(channel ssh.Channel, receivedCommand string) (err error) {
	// Parse command for exe and args
	commandArray := strings.Fields(receivedCommand)
	commandBinary := commandArray[0]

	// Prep command and args for execution
	cmd := exec.Command(commandBinary, commandArray[1:]...)

	// Init command buffers
	var stdout, stderr, channelBuff bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Get stdin from client
	_, err = io.Copy(&channelBuff, channel)
	if err != nil {
		return
	}

	// Prepare stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	defer stdin.Close()

	// Run the command
	err = cmd.Start()
	if err != nil {
		return
	}

	// Write channel contents to stdin and close input
	_, err = stdin.Write(channelBuff.Bytes())
	if err != nil {
		return
	}
	stdin.Close()

	// Wait for command to finish
	err = cmd.Wait()

	// Determine exit code to send back
	var exitCode int
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// Command failed with a non-zero exit code
			exitCode = exitError.ExitCode()
			stderr.WriteString(err.Error())
		} else {
			if strings.Contains(err.Error(), "executable file not found in ") {
				exitCode = 127 // Command not found
				stderr.WriteString(err.Error())
			} else {
				exitCode = 126 // Command exists but cannot execute
				stderr.WriteString("Command exists but cannot execute\n")
			}
		}
	} else {
		exitCode = 0   // Command executed successfully
		stderr.Reset() // Ensure stderr isn't present if exit code is 0 (because sudo -S puts password prompt in stderr)
	}

	// Send command output back through channel
	io.Copy(channel, &stdout)
	io.Copy(channel.Stderr(), &stderr)

	// Send exit status back through channel
	exitStatus := make([]byte, 4)
	binary.BigEndian.PutUint32(exitStatus, uint32(exitCode))
	channel.SendRequest("exit-status", false, exitStatus)

	// Return any errors
	if err != nil {
		return
	}
	return
}

// SFTP abstracted session handling
func HandleSFTP(channel ssh.Channel) (err error) {
	// Create new SFTP server for this channel
	sftpServer, err := sftp.NewServer(channel)
	if err != nil {
		return
	}
	defer sftpServer.Close()

	// Serve any commands from client
	err = sftpServer.Serve()
	if err != nil {
		return
	}
	return
}

// Removes header from SSH request payload and returns string text
func StripPayloadHeader(request []byte) (payload string, err error) {
	// Ignore things less than header length
	if len(request) < 4 {
		err = fmt.Errorf("invalid payload length (did the client send anything?)")
		return
	}

	// Calculate length of payload
	payloadLength := int(request[0])<<24 | int(request[1])<<16 | int(request[2])<<8 | int(request[3])

	// Validate total payload length
	if payloadLength+4 != len(request) {
		err = fmt.Errorf("payload length does not match header metadata")
		return
	}

	// Return payload without header
	payload = string(request[4:])
	return
}
