// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sync"

	"golang.org/x/crypto/ssh"
)

// ###################################
//      GLOBAL VARIABLES
// ###################################

// Global for program configurations
var config Config

// Struct for global config
type Config struct {
	FilePath            string                  // Path to main config - ~/.ssh/config
	FailTrackerFilePath string                  // Path to failtracker file (within same directory as main config)
	OSPathSeparator     string                  // Path separator for compiled OS filesystem
	HostInfo            map[string]EndpointInfo // Hold some basic information about all the hosts
	KnownHostsFilePath  string                  // Path to known server public keys - ~/.ssh/known_hosts
	KnownHosts          []string                // Content of known server public keys - ~/.ssh/known_hosts
	RepositoryPath      string                  // Absolute path to git repository (based on current working dir)
	UniversalDirectory  string                  // Universal config directory inside git repo
	AllUniversalGroups  map[string]struct{}     // Universal group config directory names
	IgnoreDirectories   []string                // Directories to ignore inside the git repository
	MaxSSHConcurrency   int                     // Maximum threads for ssh sessions
	UserHomeDirectory   string                  // Absolute path to users home directory (to expand '~/' in paths)
	VaultFilePath       string                  // Path to password vault file
	Vault               map[string]Credential   // Password vault
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
	TargetFileOwnerGroup  string   `json:"FileOwnerGroup"`
	TargetFilePermissions int      `json:"FilePermissions"`
	ReloadCommands        []string `json:"Reload,omitempty"`
}

const Delimiter string = "#|^^^|#"

// Struct for all deployment info for a file
type CommitFileInfo struct {
	Data            string
	Hash            string
	Action          string
	FileOwnerGroup  string
	FilePermissions int
	ReloadRequired  bool
	Reload          []string
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

// #### Written to in other functions - use mutex

// Global for checking remote hosts keys
var addAllUnknownHosts bool
var KnownHostMutex sync.Mutex

// Used for metrics - counting post deployment
var postDeployedConfigs int
var postDeploymentHosts int
var MetricCountMutex sync.Mutex

// Global to track failed go routines' hosts, files, and errors to be able to retry deployment on user request
const FailTrackerFile string = ".scmp-failtracker.json"

var FailTracker string
var FailTrackerMutex sync.Mutex

// Program Meta Info
const progCLIHeader string = "==== Secure Configuration Management Pusher ===="
const progVersion string = "v3.1.0"
const usage = `
Examples:
    controller --config <~/.ssh/config> --deploy-changes [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>] [--remote-hosts <www,proxy,db01>] [--local-files <www/etc/hosts,proxy/etc/fstab>]
    controller --config <~/.ssh/config> --deploy-failures
    controller --config <~/.ssh/config> --deploy-all [--remote-hosts <www,proxy,db01>] [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>]
    controller --new-repo /opt/repo1:main
    controller --config <~/.ssh/config> --seed-repo [--remote-hosts <www,proxy,db01>]

Options:
    -c, --config </path/to/ssh/config>         Path to the configuration file [default: ~/.ssh/config]
    -d, --deploy-changes                       Deploy changed files in the specified commit [commit default: head]
    -a, --deploy-all                           Deploy all files in specified commit [commit default: head]
    -f, --deploy-failures                      Deploy failed files/hosts using failtracker file from last failed deployment
    -r, --remote-hosts <host1,host2,...>       Override hosts for deployment
    -l, --local-files <file1,file2,...>        Override files for deployment (Must be relative file paths from root of the repository)
    -C, --commitid <hash>                      Commit ID (hash) of the commit to deploy configurations from
    -T, --dry-run                              Prints available information and runs through all actions without initiating outbound connections
    -m, --max-conns <15>                       Maximum simultaneous outbound SSH connections [default: 10] (Use 1 to disable deployment concurrency)
    -p, --modify-vault-password <host>         Create/Change/Delete a hosts password in the vault (will create the vault if it doesn't exist)
    -n, --new-repo </path/to/repo>:<branch>    Create a new repository at the given path with the given initial branch name
    -s, --seed-repo                            Retrieve existing files from remote hosts to seed the local repository (Requires user interaction and '--remote-hosts')
    -g, --disable-git-hook                     Disables the automatic deployment git post-commit hook for the current repository
    -G, --enable-git-hook                      Enables the automatic deployment git post-commit hook for the current repository
    -t, --test-config                          Test controller configuration syntax and configuration option validity
    -v, --verbosity <0...5>                    Increase details and frequency of progress messages (Higher number is more verbose) [default: 1]
    -h, --help                                 Show this help menu
    -V, --version                              Show version and packages
        --versionid                            Show only version number

Documentation: <https://github.com/EvSecDev/SCMPusher>
`

// ###################################
//	MAIN - START
// ###################################

func main() {
	// Program Argument Variables
	var deployChangesRequested bool
	var deployAllRequested bool
	var deployFailuresRequested bool
	var commitID string
	var hostOverride string
	var fileOverride string
	var modifyVaultHost string
	var testConfig bool
	var createNewRepo string
	var seedRepoFiles bool
	var disableGitHook bool
	var enableGitHook bool
	var versionInfoRequested bool
	var versionRequested bool

	// Read Program Arguments - allowing both short and long args
	flag.StringVar(&config.FilePath, "c", "~/.ssh/config", "")
	flag.StringVar(&config.FilePath, "config", "~/.ssh/config", "")
	flag.BoolVar(&deployChangesRequested, "d", false, "")
	flag.BoolVar(&deployChangesRequested, "deploy-changes", false, "")
	flag.BoolVar(&deployAllRequested, "a", false, "")
	flag.BoolVar(&deployAllRequested, "deploy-all", false, "")
	flag.BoolVar(&deployFailuresRequested, "f", false, "")
	flag.BoolVar(&deployFailuresRequested, "deploy-failures", false, "")
	flag.StringVar(&commitID, "C", "", "")
	flag.StringVar(&commitID, "commitid", "", "")
	flag.StringVar(&hostOverride, "r", "", "")
	flag.StringVar(&hostOverride, "remote-hosts", "", "")
	flag.StringVar(&fileOverride, "l", "", "")
	flag.StringVar(&fileOverride, "local-files", "", "")
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
	flag.BoolVar(&disableGitHook, "g", false, "")
	flag.BoolVar(&disableGitHook, "disable-git-hook", false, "")
	flag.BoolVar(&enableGitHook, "G", false, "")
	flag.BoolVar(&enableGitHook, "enable-git-hook", false, "")
	flag.BoolVar(&versionInfoRequested, "V", false, "")
	flag.BoolVar(&versionInfoRequested, "version", false, "")
	flag.BoolVar(&versionRequested, "versionid", false, "")
	flag.IntVar(&globalVerbosityLevel, "v", 1, "")
	flag.IntVar(&globalVerbosityLevel, "verbosity", 1, "")

	// Undocumented internal use only
	flag.BoolVar(&CalledByGitHook, "git-hook-mode", false, "") // Differentiate between user using deploy-changes and the git hook using deploy-changes

	// Custom help menu
	flag.Usage = func() { fmt.Printf("Usage: %s [OPTIONS]...\n%s", os.Args[0], usage) }
	flag.Parse()

	// Meta info print out
	if versionInfoRequested {
		fmt.Printf("Controller %s compiled using %s(%s) on %s architecture %s\n", progVersion, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("Packages: runtime encoding/hex strings golang.org/x/term strconv github.com/go-git/go-git/v5/plumbing/object io bufio crypto/sha1 golang.org/x/crypto/ssh/knownhosts encoding/json encoding/base64 flag github.com/coreos/go-systemd/journal github.com/bramvdbogaerde/go-scp context sort fmt time golang.org/x/crypto/argon2 golang.org/x/crypto/ssh crypto/rand github.com/go-git/go-git/v5 github.com/kevinburke/ssh_config net github.com/go-git/go-git/v5/plumbing crypto/hmac golang.org/x/crypto/ssh/agent regexp os bytes crypto/sha256 golang.org/x/crypto/chacha20poly1305 sync path/filepath github.com/go-git/go-git/v5/plumbing/format/diff testing\n")
		return
	} else if versionRequested {
		fmt.Println(progVersion)
		return
	}

	// New repository creation if requested
	if createNewRepo != "" {
		createNewRepository(createNewRepo)
		return
	}

	// Retrieve configuration options - file path is global
	err := parseConfig()
	logError("Error in controller configuration", err, true)

	// Parse User Choices - see function comment for what each does
	if testConfig {
		// If user wants to test config, just exit once program gets to this point
		// Any config errors will be discovered prior to this point and exit with whatever error happened
		printMessage(VerbosityStandard, "controller: configuration file %s test is successful\n", config.FilePath)
	} else if modifyVaultHost != "" {
		err = modifyVault(modifyVaultHost)
		logError("Error modifying vault", err, false)
	} else if disableGitHook {
		toggleGitHook("disable")
	} else if enableGitHook {
		toggleGitHook("enable")
	} else if deployChangesRequested {
		preDeployment("deployChanges", commitID, hostOverride, fileOverride)
	} else if deployAllRequested {
		preDeployment("deployAll", commitID, hostOverride, fileOverride)
	} else if deployFailuresRequested {
		preDeployment("deployFailures", commitID, hostOverride, fileOverride)
	} else if seedRepoFiles {
		seedRepositoryFiles(hostOverride)
	} else {
		// No valid arguments or valid combination of arguments
		printMessage(VerbosityStandard, "No arguments specified or incorrect argument combination. Use '-h' or '--help' to guide your way.\n")
	}
}
