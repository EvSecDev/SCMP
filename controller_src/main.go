// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"golang.org/x/crypto/ssh"
)

// ###################################
//      GLOBAL VARIABLES
// ###################################

// Global for program configurations
var config Config

// Struct for global config
type Config struct {
	FilePath              string                  // Path to main config - ~/.ssh/config
	FailTrackerFilePath   string                  // Path to failtracker file (within same directory as main config)
	OSPathSeparator       string                  // Path separator for compiled OS filesystem
	HostInfo              map[string]EndpointInfo // Hold some basic information about all the hosts
	KnownHostsFilePath    string                  // Path to known server public keys - ~/.ssh/known_hosts
	KnownHosts            []string                // Content of known server public keys - ~/.ssh/known_hosts
	RepositoryPath        string                  // Absolute path to git repository (based on current working dir)
	UniversalDirectory    string                  // Universal config directory inside git repo
	AllUniversalGroups    map[string][]string     // Universal group config directory names and their respective hosts
	IgnoreDirectories     []string                // Directories to ignore inside the git repository
	MaxSSHConcurrency     int                     // Maximum threads for ssh sessions
	DisableSudo           bool                    // Disable using sudo for remote commands
	AllowDeletions        bool                    // Allow deletions in local repo to delete files on remote hosts or vault entries
	DisableReloads        bool                    // Disables all deployment reload commands for this deployment
	RunInstallCommands    bool                    // Run the install command section of all relevant files metadata header section (within the given deployment)
	RunUninstallCommands  bool                    // Run the uninstall command section of all relevant files metadata header section (within the given deployment)
	IgnoreDeploymentState bool                    // Ignore any deployment state for a host in the config
	RegexEnabled          bool                    // Globally enable the use of regex for matching hosts/files
	UserHomeDirectory     string                  // Absolute path to users home directory (to expand '~/' in paths)
	VaultFilePath         string                  // Path to password vault file
	Vault                 map[string]Credential   // Password vault
}

// Struct for host-specific Information
type EndpointInfo struct {
	DeploymentState      string              // Avoids deploying anything to host - so user can prevent deployments to otherwise up and health hosts
	IgnoreUniversal      bool                // Prevents deployments for this host to use anything from the primary Universal configs directory
	RequiresVault        bool                // Direct match to the config option "PasswordRequired"
	UniversalGroups      map[string]struct{} // Map to store the CSV for config option "GroupTags"
	DeploymentFiles      []string            // Created during pre-deployment to track which config files will be deployed to this host
	EndpointName         string              // Name of host as it appears in config and in git repo top-level directory names
	Endpoint             string              // Address:port of the host
	EndpointUser         string              // Login user name of the host
	IdentityFile         string              // Key identity file path (private or public)
	PrivateKey           ssh.Signer          // Actual private key contents
	KeyAlgo              string              // Algorithm of the private key
	Password             string              // Password for the EndpointUser
	RemoteTransferBuffer string              // Temporary Buffer file that will be used to transfer local config to remote host prior to moving into place
	RemoteBackupDir      string              // Temporary directory to store backups of existing remote configs while reloads are performed
}

// Struct for vault passwords
type Credential struct {
	LoginUserPassword string `json:"loginUserPassword"`
}

// Struct for metadata json in config files
type MetaHeader struct {
	TargetFileOwnerGroup    string   `json:"FileOwnerGroup"`
	TargetFilePermissions   int      `json:"FilePermissions"`
	ExternalContentLocation string   `json:"ExternalContentLocation,omitempty"`
	InstallCommands         []string `json:"Install,omitempty"`
	CheckCommands           []string `json:"Checks,omitempty"`
	ReloadCommands          []string `json:"Reload,omitempty"`
}

const Delimiter string = "#|^^^|#"

// Struct for all deployment info for a file
type FileInfo struct {
	Hash            string
	Action          string
	FileOwnerGroup  string
	FilePermissions int
	FileSize        int
	InstallOptional bool
	Install         []string
	ChecksRequired  bool
	Checks          []string
	ReloadRequired  bool
	Reload          []string
}

// Store deployment host metadata to easily pass between SSH functions
type HostMeta struct {
	name               string
	password           string
	sshClient          *ssh.Client
	transferBufferFile string
	backupPath         string
}

// Fail tracker json line format
type ErrorInfo struct {
	EndpointName string   `json:"endpointName"`
	Files        []string `json:"files"`
	ErrorMessage string   `json:"errorMessage"`
}

// #### Written to only from main

var CalledByGitHook bool       // for automatic rollback on parsing error
var SHA256RegEx *regexp.Regexp // for validating hashes received from remote hosts
var SHA1RegEx *regexp.Regexp   // for validating user supplied commit hashes
var dryRunRequested bool       // for printing relevant information and bailing out before outbound remote connections are made

// Integer for printing increasingly detailed information as program progresses
//
//	0 - None: quiet (prints nothing but errors)
//	1 - Standard: normal progress messages
//	2 - Progress: more progress messages (no actual data outputted)
//	3 - Data: shows limited data being processed
//	4 - FullData: shows full data being processed
//	5 - Debug: shows extra data during processing (raw bytes)
var globalVerbosityLevel int

// Descriptive Names for available verbosity levels
const (
	VerbosityNone int = iota
	VerbosityStandard
	VerbosityProgress
	VerbosityData
	VerbosityFullData
	VerbosityDebug
)

// Program Constants
const defaultConfigPath string = "~/.ssh/config"
const directoryMetadataFileName string = ".directory_metadata_information.json"
const autoCommitUserName string = "SCMPController"
const autoCommitUserEmail string = "scmpc@localhost"
const fileURIPrefix string = "file://"
const environmentUnknownSSHHostKey string = "UnknownSSHHostKeyAction"
const maxDirectoryLoopCount int = 1000                         // Maximum recursion for any loop over directories
const artifactPointerFileExtension string = ".remote-artifact" // file extension to identify 'pointer' files for artifact files
const hashingBufferSize int = 64 * 1024                        // 64KB Buffer for stream hashing

// #### Written to in other functions - use mutex

// Global for checking remote hosts keys
var addAllUnknownHosts bool
var KnownHostMutex sync.Mutex

// Used for metrics - counting post deployment
type PostDeploymentMetrics struct {
	files           int
	filesMutex      sync.Mutex
	hosts           int
	hostsMutex      sync.Mutex
	bytes           int
	bytesMutex      sync.Mutex
	sizeTransferred string
}

// Global to track failed go routines' hosts, files, and errors to be able to retry deployment on user request
const FailTrackerFile string = ".scmp-failtracker.json"

var FailTracker string
var FailTrackerMutex sync.Mutex

// Program Meta Info
const progCLIHeader string = "==== Secure Configuration Management Program ===="
const progVersion string = "v4.2.0"
const usage = `Secure Configuration Management Program (SCMP)
  Deploy configuration files from a git repository to Linux servers via SSH
  Deploy ad-hoc commands and scripts to Linux servers via SSH

Options:
  -c, --config </path/to/ssh/config>             Path to the configuration file
                                                 [default: ~/.ssh/config]
  -d, --deploy-changes                           Deploy changed files in the specified commit
                                                 [commit default: head]
  -a, --deploy-all                               Deploy all files in specified commit
                                                 [commit default: head]
  -f, --deploy-failures                          Deploy failed files/hosts using
                                                 failtracker file from last failed deployment
  -e, --execute <"command"|file:///>             Run adhoc single command or upload and
                                                 execute the script on remote hosts
  -r, --remote-hosts <host1,host2,...|file:///>  Override hosts to connect to for deployment
                                                 or adhoc command/script execution
  -R, --remote-files <file1,file2,...|file:///>  Override file(s) to retrieve using seed-repository
                                                 Also override default remote path for script execution
  -l, --local-files <file1,file2,...|file:///>   Override file(s) for deployment
                                                 Must be relative file paths from inside the repository
  -C, --commitid <hash>                          Commit ID (hash) of the commit to
                                                 deploy configurations from
  -T, --dry-run                                  Does everything except start SSH connections
                                                 Prints out deployment information
  -m, --max-conns <15>                           Maximum simultaneous outbound SSH connections
                                                 [default: 10] (1 disables concurrency)
  -p, --modify-vault-password <host>             Create/Change/Delete a hosts password in the
                                                 vault (will create the vault if it doesn't exist)
  -n, --new-repo </path/to/repo>:<branch>        Create a new repository at the given path
                                                 with the given initial branch name
  -s, --seed-repo                                Retrieve existing files from remote hosts to
                                                 seed the local repository (Requires '--remote-hosts')
      --git-add <dir|file>                       Add files/directories/globs to git worktree
                                                 Required for artifact tracking feature
      --git-status                               Check current worktree status
                                                 Prints out file paths that differ from clean worktree
      --git-commit <'message'|file:///>          Commit changes to git repository with message
                                                 File contents will be read and used as message
      --allow-deletions                          Allows deletions (remote files or vault entires)
                                                 Only applies to '--deploy-changes' or '--modify-vault-password'
      --install                                  Runs installation commands in config files metadata JSON header
                                                 Commands are run before file deployments (before checks)
      --disable-reloads                          Disables execution of reload commands for this deployment
                                                 Useful to write configs that normally need reloads without running them
      --disable-privilege-escalation             Disables use of sudo when executing commands remotely
                                                 All commands will be run as the login user
      --ignore-deployment-state                  Ignores the current deployment state in the configuration file
                                                 For example, will deploy to a host marked as offline
      --regex                                    Enables regular expression parsing for specific arguments
                                                 Supported arguments: '-r', '-R', '-l'
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

// ###################################
//	MAIN - START
// ###################################

func main() {
	// Program Argument Variables
	var deployChangesRequested bool
	var deployAllRequested bool
	var deployFailuresRequested bool
	var executeCommands string
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

	// Read Program Arguments - allowing both short and long args
	flag.StringVar(&config.FilePath, "c", defaultConfigPath, "")
	flag.StringVar(&config.FilePath, "config", defaultConfigPath, "")
	flag.BoolVar(&deployChangesRequested, "d", false, "")
	flag.BoolVar(&deployChangesRequested, "deploy-changes", false, "")
	flag.BoolVar(&deployAllRequested, "a", false, "")
	flag.BoolVar(&deployAllRequested, "deploy-all", false, "")
	flag.BoolVar(&deployFailuresRequested, "f", false, "")
	flag.BoolVar(&deployFailuresRequested, "deploy-failures", false, "")
	flag.StringVar(&executeCommands, "e", "", "")
	flag.StringVar(&executeCommands, "execute", "", "")
	flag.StringVar(&commitID, "C", "", "")
	flag.StringVar(&commitID, "commitid", "", "")
	flag.StringVar(&hostOverride, "r", "", "")
	flag.StringVar(&hostOverride, "remote-hosts", "", "")
	flag.StringVar(&remoteFileOverride, "R", "", "")
	flag.StringVar(&remoteFileOverride, "remote-files", "", "")
	flag.StringVar(&localFileOverride, "l", "", "")
	flag.StringVar(&localFileOverride, "local-files", "", "")
	flag.BoolVar(&testConfig, "t", false, "")
	flag.BoolVar(&testConfig, "test-config", false, "")
	flag.BoolVar(&dryRunRequested, "T", false, "")
	flag.BoolVar(&dryRunRequested, "dry-run", false, "")
	flag.IntVar(&config.MaxSSHConcurrency, "m", 10, "")
	flag.IntVar(&config.MaxSSHConcurrency, "max-conns", 10, "")
	flag.StringVar(&modifyVaultHost, "p", "", "")
	flag.StringVar(&modifyVaultHost, "modify-vault-password", "", "")
	flag.StringVar(&createNewRepo, "n", "", "")
	flag.StringVar(&createNewRepo, "new-repo", "", "")
	flag.BoolVar(&seedRepoFiles, "s", false, "")
	flag.BoolVar(&seedRepoFiles, "seed-repo", false, "")
	flag.BoolVar(&config.AllowDeletions, "allow-deletions", false, "")
	flag.BoolVar(&config.RunInstallCommands, "install", false, "")
	flag.BoolVar(&config.DisableReloads, "disable-reloads", false, "")
	flag.BoolVar(&config.DisableSudo, "disable-privilege-escalation", false, "")
	flag.BoolVar(&config.IgnoreDeploymentState, "ignore-deployment-state", false, "")
	flag.BoolVar(&config.RegexEnabled, "regex", false, "")
	flag.BoolVar(&versionInfoRequested, "V", false, "")
	flag.BoolVar(&versionInfoRequested, "version", false, "")
	flag.BoolVar(&versionRequested, "versionid", false, "")
	flag.IntVar(&globalVerbosityLevel, "v", 1, "")
	flag.IntVar(&globalVerbosityLevel, "verbosity", 1, "")
	flag.StringVar(&gitAddRequested, "git-add", "", "")
	flag.BoolVar(&gitStatusRequested, "git-status", false, "")
	flag.StringVar(&gitCommitRequested, "git-commit", "", "")

	// Undocumented internal use only
	flag.BoolVar(&installDefaultConfig, "install-default-config", false, "") // Install the sample config file if it doesn't exist
	flag.BoolVar(&installAAProf, "install-apparmor-profile", false, "")      // Install the profile if system supports it

	// Custom help menu
	flag.Usage = func() { fmt.Printf("Usage: %s [OPTIONS]...\n%s", os.Args[0], usage) }
	flag.Parse()

	// Meta info print out
	if versionInfoRequested {
		fmt.Printf("SCMP Controller %s\n", progVersion)
		fmt.Printf("Built using %s(%s) for %s on %s\n", runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>\n")
		fmt.Print("Direct Package Imports: runtime encoding/hex strings math golang.org/x/term strconv github.com/go-git/go-git/v5/plumbing/object io bufio crypto/sha1 golang.org/x/crypto/ssh/knownhosts encoding/json encoding/base64 flag github.com/coreos/go-systemd/journal github.com/bramvdbogaerde/go-scp context sort fmt time golang.org/x/crypto/argon2 golang.org/x/crypto/ssh crypto/rand github.com/go-git/go-git/v5 os/exec github.com/kevinburke/ssh_config net github.com/go-git/go-git/v5/plumbing crypto/hmac golang.org/x/crypto/ssh/agent regexp os bytes crypto/sha256 golang.org/x/crypto/chacha20poly1305 sync path/filepath github.com/go-git/go-git/v5/plumbing/format/diff testing\n")
		return
	} else if versionRequested {
		fmt.Println(progVersion)
		return
	}

	// Global regex
	SHA256RegEx = regexp.MustCompile(`^[a-fA-F0-9]{64}`)
	SHA1RegEx = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

	// Quick attempt at installing apparmor profile - failures are not printed under normal verbosity
	if installAAProf {
		installAAProfile()
		return
	}
	// Install sample SSH config if it doesn't already exist
	if installDefaultConfig {
		installDefaultSSHConfig()
		return
	}
	// New repository creation if requested
	if createNewRepo != "" {
		createNewRepository(createNewRepo)
		return
	}

	// If user specified any git action, parse then exit
	if gitAddRequested != "" || gitStatusRequested || gitCommitRequested != "" {
		// Check working dir for git repo
		err := retrieveGitRepoPath()
		logError("Repository Error", err, false)

		// Only track artifacts when running add
		if gitAddRequested != "" {
			// Check for artifacts and update pointers if required
			gitArtifactTracking()
		}

		// Open repository
		repo, err := git.PlainOpen(config.RepositoryPath)
		logError("Failed to open repository", err, false)

		// Get working tree
		worktree, err := repo.Worktree()
		logError("Failed to get git worktree", err, false)

		// Check current status
		status, err := worktree.Status()
		logError("Failed to get current worktree status", err, false)

		if gitAddRequested != "" && !status.IsClean() {
			printMessage(VerbosityFullData, "Raw add option: '%s'\n", gitAddRequested)

			// Exit if dry-run requested
			if dryRunRequested {
				printMessage(VerbosityStandard, "Dry-run requested, not altering worktree\n")
				return
			}

			// Add all files to worktree
			err = worktree.AddGlob(gitAddRequested)
			if err != nil {
				return
			}

			return
		} else if gitCommitRequested != "" && !status.IsClean() {
			err = gitCommit(gitCommitRequested, worktree)
			logError("Failed to commit changes", err, false)
		} else if gitStatusRequested && !status.IsClean() {
			currentStatus, err := worktree.Status()
			logError("Failed to retrieve worktree status", err, false)
			printMessage(VerbosityStandard, "%s", currentStatus.String())
			return
		} else if status.IsClean() {
			printMessage(VerbosityStandard, "nothing to commit, working tree clean\n")
			return
		} else if !status.IsClean() {
			printMessage(VerbosityStandard, "Untracked files present, please deal with them\n")
			return
		}
	}

	// Retrieve configuration options - file path is global
	err := parseConfig()
	logError("Error in controller configuration", err, true)

	// Retrieve any files specified by URI by override arguments
	hostOverride, err = retrieveURIFile(hostOverride)
	logError("Failed to parse remove-hosts URI", err, true)
	remoteFileOverride, err = retrieveURIFile(remoteFileOverride)
	logError("Failed to parse remote-files URI", err, true)
	localFileOverride, err = retrieveURIFile(localFileOverride)
	logError("Failed to parse local-files URI", err, true)

	// Parse User Choices - see function comment for what each does
	if testConfig {
		// If user wants to test config, just exit once program gets to this point
		// Any config errors will be discovered prior to this point and exit with whatever error happened
		printMessage(VerbosityStandard, "controller: configuration file %s test is successful\n", config.FilePath)
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
		// No valid arguments or valid combination of arguments (and not committing - so this doesn't print when committing with other args or no args)
		printMessage(VerbosityStandard, "No arguments specified or incorrect argument combination. Use '-h' or '--help' to guide your way.\n")
	}
}
