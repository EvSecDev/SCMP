// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// ###################################
//	GLOBAL CONSTANTS
// ###################################

const (
	// Descriptive Names for available verbosity levels
	verbosityNone int = iota
	verbosityStandard
	verbosityProgress
	verbosityData
	verbosityFullData
	verbosityDebug

	progVersion                  string = "v5.1.0"
	metaDelimiter                string = "#|^^^|#"                              // Start and stop delimiter for repository file metadata header
	defaultConfigPath            string = "~/.ssh/config"                        // Default to users home directory ssh config file
	artifactPointerFileExtension string = ".remote-artifact"                     // file extension to identify 'pointer' files for artifact files
	directoryMetadataFileName    string = ".directory_metadata_information.json" // hidden file to identify parent directories metadata
	fileURIPrefix                string = "file://"                              // Used by the user to tell certain arguments to load file content
	maxDirectoryLoopCount        int    = 200                                    // Maximum recursion for any loop over directories
	defaultRemoteCommandTimeout  int    = 10                                     // Time in seconds for remote command to be considered dead
	defaultConnectTimeout        int    = 30                                     // Time in seconds for SSH connection timeout
	sshVersionString             string = "SSH-2.0-OpenSSH_10.0p2"               // Some IPS rules flag on GO's ssh client string
	remoteTmpDir                 string = "/tmp"                                 // Temporary directory to use on remote systems

	// Descriptive Names for stats fs types
	dirType       string = "directory"
	fileType      string = "regular file"
	fileEmptyType string = "regular empty file"
	symlinkType   string = "symbolic link"
	// deviceType    string = "block special file"
	// charType      string = "character special file"
	// socketType    string = "socket"
	// portType      string = "port"
	// fifoType      string = "fifo"

	helpMenuTitle    = "Secure Configuration Management Program (SCMP)"
	helpMenuSubTitle = `  Deploy configuration files from a git repository to Linux servers via SSH
  Deploy ad-hoc commands and scripts to Linux servers via SSH

`
	helpMenuTrailer = `
Report bugs to: dev@evsec.net
SCMP home page: <https://github.com/EvSecDev/SCMP>
General help using GNU software: <https://www.gnu.org/gethelp/>
`
)

// ###################################
//  GLOBAL VARIABLES
// ###################################

// Global for program configurations
var config Config

// Struct for global config
type Config struct {
	filePath            string                  // Path to main config - ~/.ssh/config
	logFilePath         string                  // Path to user requested log file
	logFile             *os.File                // File to write logs to
	eventLog            []string                // Global log storage
	eventLogMutex       sync.Mutex              // Allow concurrent access to log storage
	failTrackerFilePath string                  // Path to failtracker file (within same directory as main config)
	osPathSeparator     string                  // Path separator for compiled OS filesystem
	hostInfo            map[string]EndpointInfo // Hold some basic information about all the hosts
	knownHostsFilePath  string                  // Path to known server public keys - ~/.ssh/known_hosts
	knownHosts          []string                // Content of known server public keys - ~/.ssh/known_hosts
	repositoryPath      string                  // Absolute path to git repository (based on current working dir)
	universalDirectory  string                  // Universal config directory inside git repo
	allUniversalGroups  map[string][]string     // Universal group config directory names and their respective hosts
	ignoreDirectories   []string                // Directories to ignore inside the git repository
	options             Opts                    // Options specified/implied by the user
	userHomeDirectory   string                  // Absolute path to users home directory (to expand '~/' in paths)
	vaultFilePath       string                  // Path to password vault file
	vault               map[string]Credential   // Password vault
}

type Opts struct {
	maxSSHConcurrency        int    // Maximum threads for ssh sessions
	maxDeployConcurrency     int    // Maximum threads for file deployments per host
	calledByGitHook          bool   // Signal to err handling that rollback is available
	dryRunEnabled            bool   // Tests deployment setup without connecting to remotes
	wetRunEnabled            bool   // Tests deployment on remotes without mutating anything
	runAsUser                string // User to run commands as (not login user)
	disableSudo              bool   // Disable using sudo for remote commands
	allowDeletions           bool   // Allow deletions in local repo to delete files on remote hosts or vault entries
	disableReloads           bool   // Disables all deployment reload commands for this deployment
	runInstallCommands       bool   // Run the install command section of all relevant files metadata header section (within the given deployment)
	ignoreDeploymentState    bool   // Ignore any deployment state for a host in the config
	regexEnabled             bool   // Globally enable the use of regex for matching hosts/files
	forceEnabled             bool   // Atomic mode
	detailedSummaryRequested bool   // Generate a summary report of the deployment
	executionTimeout         int    // Timeout in seconds for user-defined commands (Reloads,checks,exec,ect.)
}

// Struct for host-specific Information
type EndpointInfo struct {
	deploymentState string              // Avoids deploying anything to host - so user can prevent deployments to otherwise up and health hosts
	ignoreUniversal bool                // Prevents deployments for this host to use anything from the primary Universal configs directory
	requiresVault   bool                // Direct match to the config option "PasswordRequired"
	universalGroups map[string]struct{} // Map to store the CSV for config option "GroupTags"
	deploymentList  []DeploymentList    // Ordered list of files and their groupings (separated by fully-independent groups)
	endpointName    string              // Name of host as it appears in config and in git repo top-level directory names
	proxy           string              // Name of the proxy host to use (if any)
	endpoint        string              // Address:port of the host
	endpointUser    string              // Login user name of the host
	identityFile    string              // Key identity file path (private or public)
	privateKey      ssh.Signer          // Actual private key contents
	keyAlgo         string              // Algorithm of the private key
	password        string              // Password for the EndpointUser
	connectTimeout  int                 // Timeout in seconds for connection to this host
}

// Integer for printing increasingly detailed information as program progresses
//
//	0 - None: quiet (prints nothing but errors)
//	1 - Standard: normal progress messages
//	2 - Progress: more progress messages (no actual data outputted)
//	3 - Data: shows limited data being processed
//	4 - FullData: shows full data being processed
//	5 - Debug: shows extra data during processing (raw bytes)
var globalVerbosityLevel int

// ###################################
//	MAIN
// ###################################

func main() {
	availCommands := map[string]func(string, []string){
		"deploy":  entryDeploy,
		"seed":    entrySeed,
		"exec":    entryExec,
		"scp":     entrySCP,
		"git":     entryGit,
		"secrets": entrySecrets,
		"install": entryInstall,
		"version": entryVersion,
	}
	var commandList []string
	for cmd := range availCommands {
		commandList = append(commandList, cmd)
	}

	args := os.Args
	commandFlags := flag.NewFlagSet(args[0], flag.ExitOnError)
	setDeployConfArguments(commandFlags)
	setGlobalArguments(commandFlags)

	commandFlags.Usage = func() {
		printHelpMenu(commandFlags, "", commandList, "", true)
	}
	if len(args) < 2 {
		printHelpMenu(commandFlags, "", commandList, "", true)
		os.Exit(1)
	}
	commandFlags.Parse(args[1:])

	// Retrieve command and args
	command := args[1]
	args = args[2:]

	// Process commands
	entryFunc, validCommand := availCommands[command]
	if validCommand {
		entryFunc(command, args)
	} else {
		printHelpMenu(commandFlags, "", commandList, "", true)
		os.Exit(1)
	}

	// Write global logs to disk
	if config.logFile != nil {
		defer config.logFile.Close()

		allEvents := strings.Join(config.eventLog, "")
		_, err := config.logFile.WriteString(allEvents + "\n")
		if err != nil {
			fmt.Printf("Failed to write to log file: %v\n", err)
		}
	}
}

func entryVersion(commandname string, args []string) {
	if len(args) > 0 && (args[0] == "--verbosity" || args[0] == "-v") {
		fmt.Printf("SCMP Controller %s\n", progVersion)
		fmt.Printf("Built using %s(%s) for %s on %s\n", runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>\n")
		fmt.Print("Direct Package Imports: runtime encoding/hex strings math golang.org/x/term strconv github.com/go-git/go-git/v5/plumbing/object io bufio crypto/sha1 golang.org/x/crypto/ssh/knownhosts slices encoding/json encoding/base64 flag github.com/coreos/go-systemd/journal github.com/bramvdbogaerde/go-scp os/signal context sort fmt time golang.org/x/crypto/argon2 golang.org/x/crypto/ssh crypto/rand github.com/go-git/go-git/v5 os/exec github.com/kevinburke/ssh_config net github.com/go-git/go-git/v5/plumbing crypto/hmac golang.org/x/crypto/ssh/agent syscall regexp os bytes crypto/sha256 golang.org/x/crypto/chacha20poly1305 sync path/filepath\n")
	} else {
		fmt.Println(progVersion)
	}
}
