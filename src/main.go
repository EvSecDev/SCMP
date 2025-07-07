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
	metaDelimiter                string = "#|^^^|#"                              // Start and stop delimiter for repository file metadata header
	defaultConfigPath            string = "~/.ssh/config"                        // Default to users home directory ssh config file
	artifactPointerFileExtension string = ".remote-artifact"                     // file extension to identify 'pointer' files for artifact files
	directoryMetadataFileName    string = ".directory_metadata_information.json" // hidden file to identify parent directories metadata
	fileURIPrefix                string = "file://"                              // Used by the user to tell certain arguments to load file content
	maxDirectoryLoopCount        int    = 200                                    // Maximum recursion for any loop over directories
	defaultRemoteCommandTimeout  int    = 10                                     // Time in seconds for remote command to be considered dead

	dirType       string = "directory" // Descriptive Names for stats fs types
	fileType      string = "regular file"
	fileEmptyType string = "regular empty file"
	symlinkType   string = "symbolic link"
	deviceType    string = "block special file"
	charType      string = "character special file"
	socketType    string = "socket"
	portType      string = "port"
	fifoType      string = "fifo"
)

// Descriptive Names for available verbosity levels
const (
	verbosityNone int = iota
	verbosityStandard
	verbosityProgress
	verbosityData
	verbosityFullData
	verbosityDebug
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
}

// Struct for host-specific Information
type EndpointInfo struct {
	deploymentState string              // Avoids deploying anything to host - so user can prevent deployments to otherwise up and health hosts
	ignoreUniversal bool                // Prevents deployments for this host to use anything from the primary Universal configs directory
	requiresVault   bool                // Direct match to the config option "PasswordRequired"
	universalGroups map[string]struct{} // Map to store the CSV for config option "GroupTags"
	deploymentList  DeploymentList      // Ordered list of files and their groupings
	endpointName    string              // Name of host as it appears in config and in git repo top-level directory names
	proxy           string              // Name of the proxy host to use (if any)
	endpoint        string              // Address:port of the host
	endpointUser    string              // Login user name of the host
	identityFile    string              // Key identity file path (private or public)
	privateKey      ssh.Signer          // Actual private key contents
	keyAlgo         string              // Algorithm of the private key
	password        string              // Password for the EndpointUser
	remoteBufferDir string              // Temporary Buffer directory that will be used to transfer local config to remote host prior to moving into place
	remoteBackupDir string              // Temporary directory to store backups of existing remote configs while reloads are performed
}

type DeploymentList struct {
	files             []string            // Ordered list of everything to deploy
	reloadIDtoFile    map[string][]string // Lookup of file list by reload ID
	fileToReloadID    map[string]string   // Lookup of a files reload ID
	reloadIDfileCount map[string]int      // Total files in reload group
	reloadIDcommands  map[string][]string // Ordered list of reload commands
}

type Credential struct {
	LoginUserPassword string `json:"loginUserPassword"` // For secrets vault
}

// Struct for metadata json in config files
type MetaHeader struct {
	TargetFileOwnerGroup    string   `json:"FileOwnerGroup"`
	TargetFilePermissions   int      `json:"FilePermissions"`
	ExternalContentLocation string   `json:"ExternalContentLocation,omitempty"`
	SymbolicLinkTarget      string   `json:"SymbolicLinkTarget,omitempty"`
	Dependencies            []string `json:"Dependencies,omitempty"`
	InstallCommands         []string `json:"Install,omitempty"`
	CheckCommands           []string `json:"Checks,omitempty"`
	ReloadCommands          []string `json:"Reload,omitempty"`
	ReloadGroup             string   `json:"ReloadGroup,omitempty"`
}

// Struct for deployment file metadata
type FileInfo struct {
	hash            string
	targetFilePath  string
	action          string
	ownerGroup      string
	permissions     int
	fileSize        int
	linkTarget      string
	dependencies    []string
	installOptional bool
	install         []string
	checksRequired  bool
	checks          []string
	reloadRequired  bool
	reload          []string
	reloadGroup     string
}

// Struct for remote file metadata
type RemoteFileInfo struct {
	hash        string
	name        string
	fsType      string
	permissions int
	owner       string
	group       string
	size        int
	linkTarget  string
	exists      bool
}

// Deployment host metadata to easily pass between SSH functions
type HostMeta struct {
	name              string
	osFamily          string
	password          string
	sshClient         *ssh.Client
	transferBufferDir string
	backupPath        string
}

// Type for commands run remotely
type RemoteCommand struct {
	string            // Command string
	timeout      int  // In seconds
	streamStdout bool // Progressively stream output of command to stdout of this program (almost always false)
}

// Used for metrics - counting post deployment
type DeploymentMetrics struct {
	startTime       int64
	hostFiles       map[string][]string
	hostFilesMutex  sync.Mutex
	hostErr         map[string]string
	hostErrMutex    sync.Mutex
	fileErr         map[string]string
	fileErrMutex    sync.RWMutex
	fileAction      map[string]string
	fileActionMutex sync.Mutex
	hostBytes       map[string]int
	hostBytesMutex  sync.Mutex
	endTime         int64
}

// Summary of actions done and collected metrics
// Status could be UpToDate,Deployed,Partial,Failed
type DeploymentSummary struct {
	Status          string `json:"Status"`
	StartTime       string `json:"Start-Time"`
	EndTime         string `json:"End-Time"`
	ElapsedTime     string `json:"Elapsed-Time"`     // Human readable
	TransferredData string `json:"Transferred-Size"` // Human readable
	Counters        struct {
		Hosts          int `json:"Hosts" `
		Items          int `json:"Items"`
		CompletedHosts int `json:"Hosts-Completed"`
		CompletedItems int `json:"Items-Completed"`
		FailedHosts    int `json:"Hosts-Failed"`
		FailedItems    int `json:"Items-Failed"`
	} `json:"Counters"`
	CommitID string        `json:"Deployment-Commit-Hash"`
	Hosts    []HostSummary `json:"Hosts,omitempty"`
}

type HostSummary struct {
	Name            string        `json:"Name"`
	Status          string        `json:"Status,omitempty"`
	ErrorMsg        string        `json:"Error-Message,omitempty"`
	TotalItems      int           `json:"Total-Items,omitempty"`
	TransferredData string        `json:"Transferred-Size,omitempty"`
	Items           []ItemSummary `json:"Items,omitempty"`
}

type ItemSummary struct {
	Name     string `json:"Name"`
	Action   string `json:"Deployment-Action"`
	Status   string `json:"Status,omitempty"`
	ErrorMsg string `json:"Error-Message,omitempty"`
}

// For seed repository - keeping track of directories in menu
type DirectoryState struct {
	current string
	stack   []string
}

// For abstracting file information away from git for testing
type GitChangedFileMetadata struct {
	fromNotOnFS bool
	fromPath    string
	fromMode    string
	toNotOnFS   bool
	toPath      string
	toMode      string
}

type SeedRepoUserChoiceCache struct {
	reloadCmd      map[string][]string
	reloadCnt      map[string]int
	artifactExtDir map[string]int
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

// #### Written to in other functions - use mutex

// Global for checking remote hosts keys
var addAllUnknownHosts bool
var knownHostMutex sync.Mutex

// ###################################
//	MAIN - START
// ###################################

func main() {
	// Program Argument Variables
	var sshConfigPath string
	var deployChangesRequested bool
	var deployAllRequested bool
	var deployFailuresRequested bool
	var executeCommands string
	var scpRequested bool
	var commitID string
	var hostOverride string
	var remoteFileOverride string
	var localFileOverride string
	var modifyVaultHost string
	var testConfig bool
	var createNewRepo string
	var seedRepoFiles bool
	var installAAProf bool
	var installDefaultConfig bool
	var versionInfoRequested bool
	var versionRequested bool
	var gitAddRequested string
	var gitStatusRequested bool
	var gitCommitRequested string

	const usage = `
Secure Configuration Management Program (SCMP)
  Deploy configuration files from a git repository to Linux servers via SSH
  Deploy ad-hoc commands and scripts to Linux servers via SSH

  Options:
    -c, --config </path/to/ssh/config>             Path to the configuration file
                                                   [default: ~/.ssh/config]
    -d, --deploy-changes                           Deploy changed files in the specified commit
                                                   [default: HEAD]
    -a, --deploy-all                               Deploy all files in specified commit
                                                   [default: HEAD]
    -f, --deploy-failures                          Deploy failed files/hosts using
                                                   cached deployment summary file
    -e, --execute <"command"|file:///>             Run adhoc single command or upload and
                                                   execute the script on remote hosts
    -S, --scp                                      Transfer files only
                                                   Use -r, -l, -R - one-to-one mapping between -l/-R
    -u, --run-as-user <username>                   User name to run sudo commands as
                                                   [default: root]
    -r, --remote-hosts <host1,host2,...|file://>   Override hosts to connect to for deployment
                                                   or adhoc command/script execution
    -R, --remote-files <file1,file2,...|file://>   Override file(s) to retrieve using seed-repository
                                                   Also override default remote path for script execution
    -l, --local-files <file1,file2,...|file://>    Override file(s) for deployment
                                                   Must be relative file paths from inside the repository
    -C, --commitid <hash>                          Commit ID (hash) to deploy from
                                                   Effective with both '-a' and '-d'
    -T, --dry-run                                  Does everything except start SSH connections
                                                   Prints out deployment information
    -w, --wet-run                                  Connects to remotes and tests deployment without mutating actions
                                                   [default: false]
    -m, --max-conns <15>                           Maximum simultaneous outbound SSH connections
                                                   [default: 10] (1 disables concurrency)
    -p, --modify-vault-password <host>             Create/Change/Delete a hosts password in the
                                                   vault (will create the vault if it doesn't exist)
    -n, --new-repo </path/to/repo>:<branch>        Create a new repository with given path/branch
                                                   [branch default: main]
    -s, --seed-repo                                Retrieve existing files from remote hosts to
                                                   seed the local repository (Requires '--remote-hosts')
        --git-add <dir|file>                       Add files/directories/globs to git worktree
                                                   Required for artifact tracking feature
        --git-status                               Check current worktree status
                                                   Prints out file paths that differ from clean worktree
        --git-commit <'message'|file://>           Commit changes to git repository with message
                                                   File contents will be read and used as message
        --allow-deletions                          Allows deletions (remote files or vault entires)
                                                   [default: false]
        --install                                  Runs installation commands in files metadata JSON header
                                                   [default: false]
        --force                                    Ignores checks and forces writes and reloads
                                                   [default: false]
        --disable-reloads                          Disables execution of reload commands for this deployment
                                                   [default: false]
        --disable-privilege-escalation             Disables use of sudo when executing commands remotely
                                                   [default: false]
        --ignore-deployment-state                  Treats all applicable hosts as 'Online'
                                                   [default: false]
        --regex                                    Enables regular expression parsing for specific arguments
                                                   Supported arguments: '-r', '-R', '-l'
        --log-file                                 Write out events to log file
                                                   Output verbosity follows program-wide '--verbose'
        --with-summary                             Generate detailed summary report of the deployment
                                                   Output is JSON
    -t, --test-config                              Test controller configuration syntax
                                                   and configuration option validity
    -v, --verbose <0...5>                          Increase details and frequency of progress messages
                                                   (Higher is more verbose) [default: 1]
    -h, --help                                     Show this help menu
    -V, --version                                  Show version and packages
        --versionid                                Show only version number

  Report bugs to: dev@evsec.net
  SCMP home page: <https://github.com/EvSecDev/SCMP>
  General help using GNU software: <https://www.gnu.org/gethelp/>
`
	// Read Program Arguments
	flag.StringVar(&sshConfigPath, "c", defaultConfigPath, "")
	flag.StringVar(&sshConfigPath, "config", defaultConfigPath, "")
	flag.BoolVar(&deployChangesRequested, "d", false, "")
	flag.BoolVar(&deployChangesRequested, "deploy-changes", false, "")
	flag.BoolVar(&deployAllRequested, "a", false, "")
	flag.BoolVar(&deployAllRequested, "deploy-all", false, "")
	flag.BoolVar(&deployFailuresRequested, "f", false, "")
	flag.BoolVar(&deployFailuresRequested, "deploy-failures", false, "")
	flag.StringVar(&executeCommands, "e", "", "")
	flag.StringVar(&executeCommands, "execute", "", "")
	flag.BoolVar(&scpRequested, "S", false, "")
	flag.BoolVar(&scpRequested, "scp", false, "")
	flag.StringVar(&commitID, "C", "", "")
	flag.StringVar(&commitID, "commitid", "", "")
	flag.StringVar(&config.options.runAsUser, "u", "root", "")
	flag.StringVar(&config.options.runAsUser, "run-as-user", "root", "")
	flag.StringVar(&hostOverride, "r", "", "")
	flag.StringVar(&hostOverride, "remote-hosts", "", "")
	flag.StringVar(&remoteFileOverride, "R", "", "")
	flag.StringVar(&remoteFileOverride, "remote-files", "", "")
	flag.StringVar(&localFileOverride, "l", "", "")
	flag.StringVar(&localFileOverride, "local-files", "", "")
	flag.BoolVar(&testConfig, "t", false, "")
	flag.BoolVar(&testConfig, "test-config", false, "")
	flag.BoolVar(&config.options.dryRunEnabled, "T", false, "")
	flag.BoolVar(&config.options.dryRunEnabled, "dry-run", false, "")
	flag.BoolVar(&config.options.wetRunEnabled, "w", false, "")
	flag.BoolVar(&config.options.wetRunEnabled, "wet-run", false, "")
	flag.IntVar(&config.options.maxSSHConcurrency, "m", 10, "")
	flag.IntVar(&config.options.maxSSHConcurrency, "max-conns", 10, "")
	flag.StringVar(&modifyVaultHost, "p", "", "")
	flag.StringVar(&modifyVaultHost, "modify-vault-password", "", "")
	flag.StringVar(&createNewRepo, "n", "", "")
	flag.StringVar(&createNewRepo, "new-repo", "", "")
	flag.BoolVar(&seedRepoFiles, "s", false, "")
	flag.BoolVar(&seedRepoFiles, "seed-repo", false, "")
	flag.BoolVar(&config.options.allowDeletions, "allow-deletions", false, "")
	flag.BoolVar(&config.options.runInstallCommands, "install", false, "")
	flag.BoolVar(&config.options.forceEnabled, "force", false, "")
	flag.BoolVar(&config.options.disableReloads, "disable-reloads", false, "")
	flag.BoolVar(&config.options.disableSudo, "disable-privilege-escalation", false, "")
	flag.BoolVar(&config.options.ignoreDeploymentState, "ignore-deployment-state", false, "")
	flag.BoolVar(&config.options.regexEnabled, "regex", false, "")
	flag.BoolVar(&config.options.detailedSummaryRequested, "with-summary", false, "")
	flag.StringVar(&config.logFilePath, "log-file", "", "")
	flag.BoolVar(&versionInfoRequested, "V", false, "")
	flag.BoolVar(&versionInfoRequested, "version", false, "")
	flag.BoolVar(&versionRequested, "versionid", false, "")
	flag.IntVar(&globalVerbosityLevel, "v", 1, "")
	flag.IntVar(&globalVerbosityLevel, "verbosity", 1, "")
	flag.StringVar(&gitAddRequested, "git-add", "", "")
	flag.BoolVar(&gitStatusRequested, "git-status", false, "")
	flag.StringVar(&gitCommitRequested, "git-commit", "", "")

	// Undocumented(in help menu) - bootstrap use only
	flag.BoolVar(&installDefaultConfig, "install-default-config", false, "") // Install the sample config file if it doesn't exist
	flag.BoolVar(&installAAProf, "install-apparmor-profile", false, "")      // Install the profile if system supports it

	flag.Usage = func() { fmt.Printf("Usage: %s [OPTIONS]...%s", os.Args[0], usage) }
	flag.Parse()

	const progVersion string = "v4.8.1"
	if versionInfoRequested {
		fmt.Printf("SCMP Controller %s\n", progVersion)
		fmt.Printf("Built using %s(%s) for %s on %s\n", runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>\n")
		fmt.Print("Direct Package Imports: runtime encoding/hex strings math golang.org/x/term strconv github.com/go-git/go-git/v5/plumbing/object io bufio crypto/sha1 golang.org/x/crypto/ssh/knownhosts slices encoding/json encoding/base64 flag github.com/coreos/go-systemd/journal github.com/bramvdbogaerde/go-scp context sort fmt time golang.org/x/crypto/argon2 golang.org/x/crypto/ssh crypto/rand github.com/go-git/go-git/v5 os/exec github.com/kevinburke/ssh_config net github.com/go-git/go-git/v5/plumbing crypto/hmac golang.org/x/crypto/ssh/agent regexp os bytes crypto/sha256 golang.org/x/crypto/chacha20poly1305 sync path/filepath\n")
		return
	} else if versionRequested {
		fmt.Println(progVersion)
		return
	}

	if installAAProf {
		installAAProfile()
		return
	}
	if installDefaultConfig {
		installDefaultSSHConfig()
		return
	}
	if createNewRepo != "" {
		createNewRepository(createNewRepo)
		return
	}

	// Git commit separated from regular conditional block so it can be included with other arguments
	if gitCommitRequested != "" {
		err := gitCommit(gitCommitRequested)
		logError("Failed to commit changes", err, false)
	}

	err := config.extractOptions(sshConfigPath)
	logError("Error in controller configuration", err, true)

	// Pull contents of out file URIs
	hostOverride, err = retrieveURIFile(hostOverride)
	logError("Failed to parse remove-hosts URI", err, true)
	remoteFileOverride, err = retrieveURIFile(remoteFileOverride)
	logError("Failed to parse remote-files URI", err, true)
	localFileOverride, err = retrieveURIFile(localFileOverride)
	logError("Failed to parse local-files URI", err, true)

	// Parse User Choices - see function comment for what each does
	if testConfig {
		printMessage(verbosityStandard, "controller: configuration file %s test is successful\n", config.filePath)
	} else if gitStatusRequested {
		_, status, err := gitOpenCWD()
		logError("Failed to retrieve worktree status", err, false)

		if status.IsClean() {
			printMessage(verbosityStandard, "no changes, working tree clean\n")
		} else if !status.IsClean() {
			printMessage(verbosityStandard, "%s", status.String())
		}
	} else if gitAddRequested != "" {
		err := gitAdd(gitAddRequested)
		logError("Failed to add changes to working tree", err, false)
	} else if scpRequested {
		err = bulkFileTransfer(hostOverride, localFileOverride, remoteFileOverride)
		logError("Failed to transfer files", err, false)
	} else if modifyVaultHost != "" {
		err = modifyVault(modifyVaultHost)
		logError("Error modifying vault", err, false)
	} else if deployChangesRequested {
		preDeployment("deployChanges", commitID, hostOverride, localFileOverride)
	} else if deployAllRequested {
		preDeployment("deployAll", commitID, hostOverride, localFileOverride)
	} else if deployFailuresRequested {
		preDeployment("deployFailures", commitID, hostOverride, localFileOverride)
	} else if seedRepoFiles {
		seedRepositoryFiles(hostOverride, remoteFileOverride)
	} else if strings.Contains(executeCommands, "file:") {
		runScript(executeCommands, hostOverride, remoteFileOverride)
	} else if executeCommands != "" {
		runCmd(executeCommands, hostOverride)
	} else if gitCommitRequested == "" {
		// Exclude git commit so this doesn't print when committing with other args or no args)
		printMessage(verbosityStandard, "No arguments specified or incorrect argument combination. Use '-h' or '--help' to guide your way.\n")
	}

	// Write global logs to disk
	if config.logFile != nil {
		defer config.logFile.Close()

		allEvents := strings.Join(config.eventLog, "")
		_, err = config.logFile.WriteString(allEvents + "\n")
		if err != nil {
			fmt.Printf("Failed to write to log file: %v\n", err)
		}
	}
}
