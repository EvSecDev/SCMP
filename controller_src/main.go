// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"

	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

// ###################################
//      GLOBAL VARIABLES
// ###################################

// Main Yaml config format
type Config struct {
	Controller struct {
		RepositoryPath string `yaml:"RepositoryPath"`
		LogtoJournald  bool   `yaml:"LogtoJournald"`
	} `yaml:"Controller"`
	SSHClient struct {
		KnownHostsFile     string `yaml:"KnownHostsFile"`
		MaximumConcurrency int    `yaml:"MaximumConnectionsAtOnce"`
	} `yaml:"SSHClient"`
	SSHClientDefault   SSHClientDefaults            `yaml:"SSHClientDefaults"`
	UniversalDirectory string                       `yaml:"UniversalDirectory"`
	IgnoreDirectories  []string                     `yaml:"IgnoreDirectories"`
	DeployerEndpoints  map[string]DeployerEndpoints `yaml:"DeployerEndpoints"`
}

// Struct for default deployer endpoints options
type SSHClientDefaults struct {
	EndpointPort         int    `yaml:"endpointPort"`
	EndpointUser         string `yaml:"endpointUser"`
	SSHIdentityFile      string `yaml:"SSHIdentityFile"`
	UseSSHAgent          bool   `yaml:"UseSSHAgent"`
	SudoPassword         string `yaml:"SudoPassword"`
	RemoteTransferBuffer string `yaml:"RemoteTransferBuffer"`
}

// Struct for deployer endpoints config section - options here can override sshclientdefaults
type DeployerEndpoints struct {
	Endpoint             string `yaml:"endpoint"`
	EndpointPort         int    `yaml:"endpointPort,omitempty"`
	EndpointUser         string `yaml:"endpointUser,omitempty"`
	SSHIdentityFile      string `yaml:"SSHIdentityFile,omitempty"`
	UseSSHAgent          *bool  `yaml:"UseSSHAgent,omitempty"`
	SudoPassword         string `yaml:"SudoPassword,omitempty"`
	RemoteTransferBuffer string `yaml:"RemoteTransferBuffer,omitempty"`
	IgnoreUniversalConfs bool   `yaml:"ignoreUniversalConfs,omitempty"`
}

// Struct for endpoint Information (the eventual combination/deduplication of the two structs above)
type EndpointInfo struct {
	Endpoint             string
	EndpointUser         string
	PrivateKey           ssh.Signer
	KeyAlgo              string
	SudoPassword         string
	RemoteTransferBuffer string
}

// Struct for metadata section
type MetaHeader struct {
	TargetFileOwnerGroup  string   `json:"FileOwnerGroup"`
	TargetFilePermissions int      `json:"FilePermissions"`
	ReloadRequired        bool     `json:"ReloadRequired"`
	ReloadCommands        []string `json:"Reload,omitempty"`
}

const Delimiter = string("#|^^^|#")

// Fail tracker json line format
type ErrorInfo struct {
	EndpointName string   `json:"endpointName"`
	Files        []string `json:"files"`
	ErrorMessage string   `json:"errorMessage"`
}

// #### Written to only in main

var configFilePath string      // for printing recovery command to user
var LogToJournald bool         // for optional logging to journald
var CalledByGitHook bool       // for automatic rollback on parsing error
var knownHostsFilePath string  // for loading known_hosts file
var tmpRemoteFilePath string   // for ssh transfers to remote
var RepositoryPath string      // for parsing commits
var UniversalDirectory string  // for parsing commits
var IgnoreDirectories []string // for parsing commits
var OSPathSeparator string     // for parsing local files and paths
var SHA256RegEx *regexp.Regexp // for validating hashes received from remote hosts
var SHA1RegEx *regexp.Regexp   // for validating user supplied commit hashes

// #### Written to in other functions - use mutex

// Used for metrics - counting post deployment
var postDeployedConfigs int
var postDeploymentHosts int
var MetricCountMutex sync.Mutex

// Global to track failed go routines' hosts, files, and errors to be able to retry deployment on user request
const FailTrackerFile = string(".failtracker.meta")

var FailTracker string
var FailTrackerMutex sync.Mutex

// Global for checking remote hosts keys
var addAllUnknownHosts bool
var knownhosts []string
var KnownHostMutex sync.Mutex

// Program Meta Info
const progCLIHeader = string("==== Secure Configuration Management Pusher ====")
const progVersion = string("v1.5.0")
const usage = `
Examples:
    controller --config </etc/scmpc.yaml> --manual-deploy --commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7> [--remote-hosts <www,proxy,db01>] [--local-files <www/etc/hosts,proxy/etc/fstab>]
    controller --config </etc/scmpc.yaml> --manual-deploy --use-failtracker-only
    controller --config </etc/scmpc.yaml> --deploy-all --remote-hosts <www,proxy,db01> [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>]
    controller --config </etc/scmpc.yaml> --deployer-versions [--remote-hosts <www,proxy,db01>]
    controller --config </etc/scmpc.yaml> --deployer-update-file <~/Downloads/deployer> [--remote-hosts <www,proxy,db01>]
    controller --new-repo /opt/repo1:main
    controller --config </etc/scmpc.yaml> --seed-repo [--remote-hosts <www,proxy,db01>]

Options:
    -c, --config </path/to/yaml>                    Path to the configuration file [default: scmpc.yaml]
    -a, --auto-deploy                               Use latest commit for deployment, normally used by git post-commit hook
    -m, --manual-deploy                             Use specified commit ID for deployment (Requires '--commitid')
    -d, --deploy-all                                Deploy all files in specified commit to specific hosts (Requires '--remote-hosts' and '--manual-deploy')
    -r, --remote-hosts <host1,host2,host3,...>      Override hosts for deployment
    -l, --local-files <file1,file2,...>             Override files for deployment from a specific commit (Requires '--manual-deploy')
    -C, --commitid <hash>                           Commit ID (hash) of the commit to deploy configurations from
    -f, --use-failtracker-only                      If previous deployment failed, use the failtracker to retry (Requires '--manual-deploy', but not '--commitid')
    -q, --deployer-versions                         Query remote host deployer executable versions and print to stdout
    -u, --deployer-update-file </path/to/binary>    Upload and update deployer executable with supplied signed ELF file
    -n, --new-repo </path/to/repo>:<branchname>     Create a new repository at the given path with the given initial branch name
    -s, --seed-repo                                 Retrieve existing files from remote hosts to seed the local repository (Requires user interaction)
    -g, --disable-git-hook                          Disables the automatic deployment git post-commit hook for the current repository
    -G, --enable-git-hook                           Enables the automatic deployment git post-commit hook for the current repository
    -h, --help                                      Show this help menu
    -V, --version                                   Show version and packages
    -v, --versionid                                 Show only version number

Documentation: <https://github.com/EvSecDev/SCMPusher>
`

// ###################################
//	MAIN - START
// ###################################

func main() {
	// Program Argument Variables
	var autoDeployFlagExists bool
	var manualCommitID string
	var hostOverride string
	var fileOverride string
	var deployerUpdateFile string
	var manualDeployFlagExists bool
	var useAllRepoFilesFlag bool
	var useFailTracker bool
	var checkDeployerVersions bool
	var createNewRepo string
	var seedRepoFiles bool
	var disableGitHook bool
	var enabledGitHook bool
	var versionFlagExists bool
	var versionNumberFlagExists bool

	// Read Program Arguments - allowing both short and long args
	flag.StringVar(&configFilePath, "c", "scmpc.yaml", "")
	flag.StringVar(&configFilePath, "config", "scmpc.yaml", "")
	flag.BoolVar(&autoDeployFlagExists, "a", false, "")
	flag.BoolVar(&autoDeployFlagExists, "auto-deploy", false, "")
	flag.BoolVar(&manualDeployFlagExists, "m", false, "")
	flag.BoolVar(&manualDeployFlagExists, "manual-deploy", false, "")
	flag.StringVar(&manualCommitID, "C", "", "")
	flag.StringVar(&manualCommitID, "commitid", "", "")
	flag.StringVar(&hostOverride, "r", "", "")
	flag.StringVar(&hostOverride, "remote-hosts", "", "")
	flag.StringVar(&fileOverride, "l", "", "")
	flag.StringVar(&fileOverride, "local-files", "", "")
	flag.BoolVar(&useAllRepoFilesFlag, "d", false, "")
	flag.BoolVar(&useAllRepoFilesFlag, "deploy-all", false, "")
	flag.BoolVar(&useFailTracker, "f", false, "")
	flag.BoolVar(&useFailTracker, "use-failtracker-only", false, "")
	flag.BoolVar(&checkDeployerVersions, "q", false, "")
	flag.BoolVar(&checkDeployerVersions, "deployer-versions", false, "")
	flag.StringVar(&deployerUpdateFile, "u", "", "")
	flag.StringVar(&deployerUpdateFile, "deployer-update-file", "", "")
	flag.StringVar(&createNewRepo, "n", "", "")
	flag.StringVar(&createNewRepo, "new-repo", "", "")
	flag.BoolVar(&seedRepoFiles, "s", false, "")
	flag.BoolVar(&seedRepoFiles, "seed-repo", false, "")
	flag.BoolVar(&disableGitHook, "g", false, "")
	flag.BoolVar(&disableGitHook, "disable-git-hook", false, "")
	flag.BoolVar(&enabledGitHook, "G", false, "")
	flag.BoolVar(&enabledGitHook, "enable-git-hook", false, "")
	flag.BoolVar(&versionFlagExists, "V", false, "")
	flag.BoolVar(&versionFlagExists, "version", false, "")
	flag.BoolVar(&versionNumberFlagExists, "v", false, "")
	flag.BoolVar(&versionNumberFlagExists, "versionid", false, "")

	// Custom help menu
	flag.Usage = func() { fmt.Printf("Usage: %s [OPTIONS]...\n%s", os.Args[0], usage) }
	flag.Parse()

	// Meta info print out
	if versionFlagExists {
		fmt.Printf("Controller %s compiled using %s(%s) on %s architecture %s\n", progVersion, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("Packages: runtime encoding/hex strings strconv github.com/go-git/go-git/v5/plumbing/object io bufio crypto/sha1 github.com/pkg/sftp encoding/json encoding/base64 flag github.com/coreos/go-systemd/journal context fmt time golang.org/x/crypto/ssh crypto/rand github.com/go-git/go-git/v5 net github.com/go-git/go-git/v5/plumbing crypto/hmac golang.org/x/crypto/ssh/agent regexp os bytes crypto/sha256 sync path/filepath encoding/binary github.com/go-git/go-git/v5/plumbing/format/diff gopkg.in/yaml.v2\n")
		os.Exit(0)
	} else if versionNumberFlagExists {
		fmt.Println(progVersion)
		os.Exit(0)
	}

	// New repository creation if requested
	if createNewRepo != "" {
		createNewRepository(createNewRepo)
		os.Exit(0)
	}

	// Read config file from argument/default option
	yamlConfigFile, err := os.ReadFile(configFilePath)
	logError("Error reading controller config file", err, true)

	// Parse config yaml fields into struct
	var config Config
	err = yaml.Unmarshal(yamlConfigFile, &config)
	logError("Error unmarshaling controller config file: %v", err, true)

	// Check for empty values in critical config fields
	err = checkConfigForEmpty(&config)
	logError("Error in controller configuration: empty value", err, true)

	// Set globals - see global section at top for descriptions
	LogToJournald = config.Controller.LogtoJournald
	CalledByGitHook = autoDeployFlagExists
	knownHostsFilePath = config.SSHClient.KnownHostsFile
	RepositoryPath = config.Controller.RepositoryPath
	UniversalDirectory = config.UniversalDirectory
	IgnoreDirectories = config.IgnoreDirectories
	OSPathSeparator = string(os.PathSeparator)
	SHA256RegEx = regexp.MustCompile(`^[a-fA-F0-9]{64}`)
	SHA1RegEx = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

	// For user choice on toggling git hook
	gitHookFileName := filepath.Join(RepositoryPath, ".git", "hooks", "post-commit")
	disabledGitHookFileName := gitHookFileName + ".disabled"

	// Parse User Choices
	if useFailTracker && manualDeployFlagExists {
		// Retry last failed deployment
		failureDeployment(config)
	} else if useAllRepoFilesFlag && manualDeployFlagExists && hostOverride != "" && manualCommitID != "" {
		// Deployment of all repository files by user chosen commit
		allDeployment(config, manualCommitID, hostOverride, fileOverride)
	} else if autoDeployFlagExists {
		// Deployment of HEAD commit
		autoDeployment(config, hostOverride)
	} else if manualDeployFlagExists && manualCommitID != "" && !useAllRepoFilesFlag {
		// User chosen commit
		manualDeployment(config, manualCommitID, hostOverride, fileOverride)
	} else if checkDeployerVersions {
		// Get version(s) of remote host(s) deployer binary
		getDeployerVersions(config, hostOverride)
	} else if deployerUpdateFile != "" {
		// Push update file for deployer
		updateDeployer(config, deployerUpdateFile, hostOverride)
	} else if seedRepoFiles {
		// Get user selected files from remote hosts to populate new local repository
		seedRepositoryFiles(config, hostOverride)
	} else if disableGitHook {
		// Check presence
		_, err := os.Stat(disabledGitHookFileName)

		// Disable automatic git post-commit hook (if not already disabled)
		if os.IsNotExist(err) {
			err = os.Rename(gitHookFileName, disabledGitHookFileName)
			if err != nil {
				fmt.Printf("Failed to disable git post-commit hook (%v)\n", err)
			}
		}
		fmt.Print("Git post-commit hook disabled.\n")
	} else if enabledGitHook {
		// Check presence
		_, err := os.Stat(gitHookFileName)

		// Enable automatic git post-commit hook (if not already enabled)
		if os.IsNotExist(err) {
			err := os.Rename(disabledGitHookFileName, gitHookFileName)
			if err != nil {
				fmt.Printf("Failed to enable git post-commit hook (%v)\n", err)
			}
		}
		fmt.Print("Git post-commit hook enabled.\n")
	} else {
		// No valid arguments or valid combination of arguments
		fmt.Printf("No arguments specified or incorrect argument combination. Use '-h' or '--help' to guide your way.\n")
	}
}
