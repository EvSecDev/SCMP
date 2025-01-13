// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sync"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/crypto/ssh"
)

// ###################################
//      GLOBAL VARIABLES
// ###################################

// Struct for endpoint Information used in maps for deployment
// Also contains an array to house the file paths (local paths) that will be deployed
type EndpointInfo struct {
	DeploymentFiles      []string
	EndpointName         string
	Endpoint             string
	EndpointUser         string
	PrivateKey           ssh.Signer
	KeyAlgo              string
	Password             string
	RemoteTransferBuffer string
	RemoteBackupDir      string
}

// Struct for metadata json in config files
type MetaHeader struct {
	TargetFileOwnerGroup  string   `json:"FileOwnerGroup"`
	TargetFilePermissions int      `json:"FilePermissions"`
	ReloadRequired        bool     `json:"ReloadRequired"`
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

// Struct for vault passwords
type Credential struct {
	LoginUserPassword string `json:"loginUserPassword"`
}

var vault map[string]Credential // Hold unlocked cached vault

// Fail tracker json line format
type ErrorInfo struct {
	EndpointName string   `json:"endpointName"`
	Files        []string `json:"files"`
	ErrorMessage string   `json:"errorMessage"`
}

// #### Written to only from main

var configFilePath string                 // for showing user retry command
var config *ssh_config.Config             // for storing host information from .ssh/config
var DeployerEndpoints []string            // for storing list of all host names
var CalledByGitHook bool                  // for automatic rollback on parsing error
var knownHostsFilePath string             // for loading known_hosts file
var RepositoryPath string                 // for parsing commits
var UniversalDirectory string             // for parsing commits
var UniversalGroups map[string][]string   // for parsing local files
var IgnoreDirectories []string            // for parsing commits
var OSPathSeparator string                // for parsing local files and paths
var MaxSSHConcurrency int                 // for limiting threads when SSH'ing to remote hosts
var SHA256RegEx *regexp.Regexp            // for validating hashes received from remote hosts
var SHA1RegEx *regexp.Regexp              // for validating user supplied commit hashes
var dryRunRequested bool                  // for printing relevant information and bailing out before outbound remote connections are made
var userHomeDirectory string              // for replacing ~/ prefixes with a path
var vaultFilePath string                  // for manipulating vault file
var hostsRequireVault map[string]struct{} // for easy reference if a host needs a password

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

// Used for metrics - counting post deployment
var postDeployedConfigs int
var postDeploymentHosts int
var MetricCountMutex sync.Mutex

// Global to track failed go routines' hosts, files, and errors to be able to retry deployment on user request
const FailTrackerFile string = ".failtracker.meta"

var FailTracker string
var FailTrackerMutex sync.Mutex

// Global for checking remote hosts keys
var addAllUnknownHosts bool
var knownhosts []string
var KnownHostMutex sync.Mutex

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
    -m, --max-conns <15>                       Maximum simultaneous outbound SSH connections [default: 10]
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
	flag.StringVar(&configFilePath, "c", "~/.ssh/config", "")
	flag.StringVar(&configFilePath, "config", "~/.ssh/config", "")
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
	flag.IntVar(&MaxSSHConcurrency, "m", 10, "")
	flag.IntVar(&MaxSSHConcurrency, "max-conns", 10, "")
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
		fmt.Print("Packages: runtime encoding/hex strings strconv github.com/go-git/go-git/v5/plumbing/object io bufio crypto/sha1 github.com/pkg/sftp encoding/json encoding/base64 flag github.com/coreos/go-systemd/journal context fmt time golang.org/x/crypto/ssh crypto/rand github.com/go-git/go-git/v5 github.com/kevinburke/ssh_config net github.com/go-git/go-git/v5/plumbing crypto/hmac golang.org/x/crypto/ssh/agent regexp os bytes crypto/sha256 sync path/filepath github.com/go-git/go-git/v5/plumbing/format/diff testing\n")
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
		printMessage(VerbosityStandard, "controller: configuration file %s test is successful\n", configFilePath)
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
