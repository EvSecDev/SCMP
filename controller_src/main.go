// controller
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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
		SSHIdentityFile      string `yaml:"SSHIdentityFile"`
		UseSSHAgent          bool   `yaml:"UseSSHAgent"`
		KnownHostsFile       string `yaml:"KnownHostsFile"`
		RemoteTransferBuffer string `yaml:"RemoteTransferBuffer"`
		MaximumConcurrency   int    `yaml:"MaximumConnectionsAtOnce"`
		SudoPassword         string `yaml:"SudoPassword"`
	} `yaml:"SSHClient"`
	TemplateDirectory string `yaml:"TemplateDirectory"`
	DeployerEndpoints map[string][]struct {
		Endpoint        string `yaml:"endpoint"`
		EndpointPort    int    `yaml:"endpointPort"`
		EndpointUser    string `yaml:"endpointUser"`
		IgnoreTemplates bool   `yaml:"ignoreTemplates"`
	} `yaml:"DeployerEndpoints"`
}

// Struct for metadata section
type MetaHeader struct {
	TargetFileOwnerGroup  string   `json:"FileOwnerGroup"`
	TargetFilePermissions int      `json:"FilePermissions"`
	ReloadRequired        bool     `json:"ReloadRequired"`
	ReloadCommands        []string `json:"Reload,omitempty"`
}

// Fail tracker json line format
type ErrorInfo struct {
	EndpointName string `json:"endpointName"`
	FilePath     string `json:"filePath"`
	ErrorMessage string `json:"errorMessage"`
}

// #### Written to only in main

// Know if auto mode or manual mode
var CalledByGitHook bool

// Know if using journald logging
var LogToJournald bool

// Used for rolling back commit upon early failure
var RepositoryPath string

// Remote file path for sftp as unprivileged user
var tmpRemoteFilePath string

// #### Written to in other functions - use mutex

// Used for metrics - counting post deployment
var postDeployedConfigs int
var postDeploymentHosts int
var MetricCountMutex sync.Mutex

// Global to track failed go routines' hosts, files, and errors to be able to retry deployment on user request
var FailTracker string
var FailTrackerMutex sync.Mutex

// Global for checking remote hosts keys
var addAllUnknownHosts bool
var knownhosts []string
var knownHostsFilePath string
var KnownHostMutex sync.Mutex

// Program Meta Info
const progVersion = "v1.3.0"
const usage = `
Examples:
    controller --config </etc/scmpc.yaml> --manual-deploy --commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>
    controller --config </etc/scmpc.yaml> --manual-deploy --use-failtracker-only
    controller --config </etc/scmpc.yaml> --deploy-all --remote-hosts <www,proxy,db01> [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>]
    controller --config </etc/scmpc.yaml> --deployer-versions [--remote-hosts <www,proxy,db01>]
    controller --config </etc/scmpc.yaml> --deployer-update-file <~/Downloads/deployer> [--remote-hosts <www,proxy,db01>]
    controller --config </etc/scmpc.yaml> --seed-repo [--remote-hosts <www,proxy,db01>]

Options:
    -c, --config </path/to/yaml>                    Path to the configuration file [default: scmpc.yaml]
    -a, --auto-deploy                               Use latest commit for deployment, normally used by git post-commit hook
    -m, --manual-deploy                             Use specified commit ID for deployment (Requires '--commitid')
    -d, --deploy-all                                Deploy all files in specified commit to specific hosts (Requires '--remote-hosts')
    -r, --remote-hosts <host1,host2,host3...>       Override hosts for deployment
    -C, --commitid <hash>                           Commit ID (hash) of the commit to deploy configurations from
    -f, --use-failtracker-only                      If previous deployment failed, use the failtracker to retry (Requires '--manual-deploy')
    -q  --deployer-versions                         Query remote host deployer executable versions and print to stdout
    -u  --deployer-update-file </path/to/binary>    Upload and update deployer executable with supplied signed ELF file
    -s  --seed-repo                                 Retrieve existing files from remote hosts to seed the local repository (Requires user interaction)
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
	var configFilePath string
	var manualCommitID string
	var hostOverride string
	var deployerUpdateFile string
	var manualDeployFlagExists bool
	var useAllRepoFilesFlag bool
	var useFailTracker bool
	var checkDeployerVersions bool
	var seedRepoFiles bool
	var versionFlagExists bool
	var versionNumberFlagExists bool

	// Read Program Arguments - allowing both short and long args
	flag.StringVar(&configFilePath, "c", "scmpc.yaml", "")
	flag.StringVar(&configFilePath, "config", "scmpc.yaml", "")
	flag.BoolVar(&CalledByGitHook, "a", false, "")
	flag.BoolVar(&CalledByGitHook, "auto-deploy", false, "")
	flag.BoolVar(&manualDeployFlagExists, "m", false, "")
	flag.BoolVar(&manualDeployFlagExists, "manual-deploy", false, "")
	flag.StringVar(&manualCommitID, "C", "", "")
	flag.StringVar(&manualCommitID, "commitid", "", "")
	flag.StringVar(&hostOverride, "r", "", "")
	flag.StringVar(&hostOverride, "remote-hosts", "", "")
	flag.BoolVar(&useAllRepoFilesFlag, "d", false, "")
	flag.BoolVar(&useAllRepoFilesFlag, "deploy-all", false, "")
	flag.BoolVar(&useFailTracker, "f", false, "")
	flag.BoolVar(&useFailTracker, "use-failtracker-only", false, "")
	flag.BoolVar(&checkDeployerVersions, "q", false, "")
	flag.BoolVar(&checkDeployerVersions, "deployer-versions", false, "")
	flag.StringVar(&deployerUpdateFile, "u", "", "")
	flag.StringVar(&deployerUpdateFile, "deployer-update-file", "", "")
	flag.BoolVar(&seedRepoFiles, "s", false, "")
	flag.BoolVar(&seedRepoFiles, "seed-repo", false, "")
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
		fmt.Printf("First party packages: runtime bufio crypto/hmac crypto/rand crypto/sha1 crypto/sha256 encoding/base64 encoding/hex encoding/json flag context fmt io net os path/filepath regexp strconv strings sync time\n")
		fmt.Printf("Third party packages: github.com/coreos/go-systemd/v22/journal github.com/go-git/go-git/v5 github.com/go-git/go-git/v5/plumbing github.com/go-git/go-git/v5/plumbing/object github.com/pkg/sftp golang.org/x/crypto/ssh golang.org/x/crypto/ssh/agent gopkg.in/yaml.v2\n")
		os.Exit(0)
	}
	if versionNumberFlagExists {
		fmt.Println(progVersion)
		os.Exit(0)
	}

	// Read config file from argument/default option
	yamlConfigFile, err := os.ReadFile(configFilePath)
	logError("Error reading controller config file", err, true)

	// Parse yaml fields into struct
	var config Config
	err = yaml.Unmarshal(yamlConfigFile, &config)
	logError("Error unmarshaling controller config file: %v", err, true)

	// Check for empty values in critical config fields
	err = checkConfigForEmpty(&config)
	logError("Error in controller configuration: empty value", err, true)

	// Global for awareness (for error handling functions)
	if config.Controller.LogtoJournald {
		LogToJournald = true
	}

	// Global for awareness (checking remote keys)
	knownHostsFilePath = config.SSHClient.KnownHostsFile

	// Global for awareness (temp file to sftp as unprivileged user)
	tmpRemoteFilePath = config.SSHClient.RemoteTransferBuffer

	// Automatic Deployment via git post-commit hook or by user choice
	if CalledByGitHook {
		// Show progress to user
		fmt.Printf("==== Secure Configuration Management Pusher ====\n")
		fmt.Printf("Starting automatic deployment\n")

		// Run deployment
		Deployment(config, false, "", false, hostOverride, useAllRepoFilesFlag, configFilePath)
		fmt.Printf("================================================\n")
		os.Exit(0)
	}

	// Manual deployment if requested
	if manualDeployFlagExists {
		// Show progress to user
		fmt.Printf("==== Secure Configuration Management Pusher ====\n")
		fmt.Printf("Starting manual deployment for commit %s\n", manualCommitID)

		// Run deployment
		Deployment(config, manualDeployFlagExists, manualCommitID, useFailTracker, hostOverride, useAllRepoFilesFlag, configFilePath)
		fmt.Printf("================================================\n")
		os.Exit(0)
	}

	// Get user selected files from remote hosts to populate new local repository
	if seedRepoFiles {
		fmt.Printf("==== Secure Configuration Management Repository Seeding ====\n")
		seedRepositoryFiles(config, hostOverride)
		fmt.Printf("============================================================\n")
		os.Exit(0)
	}

	// Get version of remote host deployer binary if requested
	if checkDeployerVersions {
		fmt.Printf("==== Secure Configuration Management Deployer Version Check ====\n")

		// Run generic loop over config deployerendpoints or user choice
		deployerVersions := simpleLoopHosts(config, "", hostOverride, true)

		// Print the versions retrieved
		fmt.Printf("Deployer executable versions:\n%s", deployerVersions)

		fmt.Printf("================================================================\n")
		os.Exit(0)
	}

	// Push update file for deployer if requested
	if deployerUpdateFile != "" {
		fmt.Printf("==== Secure Configuration Management Deployer Updater  ====\n")
		fmt.Printf("Pushing update for deployer using new executable at %s\n", deployerUpdateFile)

		// Run generic loop over config deployerendpoints or user choice
		simpleLoopHosts(config, deployerUpdateFile, hostOverride, false)

		fmt.Printf("               COMPLETE: Updates Pushed\n")
		fmt.Printf(" Please wait for deployer services to auto-restart (1 min)\n")
		fmt.Printf("===========================================================\n")
		os.Exit(0)
	}

	// Exit program without any arguments
	fmt.Printf("No arguments specified! Use '-h' or '--help' to guide your way.\n")
}

// ###################################
//      DEPLOYMENT FUNCTION
// ###################################

func localSystemChecks(RepositoryPath string) (err error) {
	// Ensure current working directory is root of git repository from config
	pwd, err := os.Getwd()
	if err != nil {
		err = fmt.Errorf("failed to obtain current working directory: %v", err)
		return
	}

	// If current directory is not repo, change to it
	if filepath.Clean(pwd) != filepath.Clean(RepositoryPath) {
		err = os.Chdir(RepositoryPath)
		if err != nil {
			err = fmt.Errorf("failed to change directory to repository path: %v", err)
			return
		}
	}

	// Get list of local systems network interfaces
	systemNetInterfaces, err := net.Interfaces()
	if err != nil {
		err = fmt.Errorf("failed to obtain system network interfaces: %v", err)
		return
	}

	// Ensure system has an active network interface
	var noActiveNetInterface bool
	for _, iface := range systemNetInterfaces {
		// Net interface is up and not loopback
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			noActiveNetInterface = false
			break
		}
		noActiveNetInterface = true
	}
	if noActiveNetInterface {
		err = fmt.Errorf("no active network interfaces found, will not attempt network connections")
		return
	}

	// Check if known hosts file exists
	_, err = os.Stat(knownHostsFilePath)
	if os.IsNotExist(err) {
		var knownFile *os.File
		// Known hosts file does not exist, create it
		knownFile, err = os.Create(knownHostsFilePath)
		if err != nil {
			err = fmt.Errorf("failed to create known_hosts file at '%s'", knownHostsFilePath)
			return
		}
		defer knownFile.Close()
	} else if err != nil {
		err = fmt.Errorf("failed to create known_hosts file at %s", knownHostsFilePath)
		return
	}

	// Read in file
	knownHostFile, err := os.ReadFile(knownHostsFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read known_hosts file: %v", err)
		return
	}

	// Store all known_hosts as array
	knownhosts = strings.Split(string(knownHostFile), "\n")

	return
}

// Deployment prep function - AKA The Monolith
func Deployment(config Config, manualDeploy bool, commitID string, useFailTracker bool, hostOverride string, useAllRepoFiles bool, configFilePath string) {
	// Set global var for git rollback when parsing error and requested rollback
	RepositoryPath = config.Controller.RepositoryPath

	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while processing deployment", fmt.Errorf("%v", fatalError), true)
		}
	}()

	// Show progress to user
	fmt.Printf("Running local system checks... ")

	// Ensure local system is in a state that is able to deploy
	err := localSystemChecks(RepositoryPath)
	logError("Error in local system checks", err, true)

	// Get SSH Private Key from the supplied identity file
	PrivateKey, err := SSHIdentityToKey(config.SSHClient.SSHIdentityFile, config.SSHClient.UseSSHAgent)
	logError("Error retrieving SSH private key", err, true)

	// Regex Vars for validating user supplied commit id and received file hashes
	SHA256RegEx := regexp.MustCompile(`^[a-fA-F0-9]{64}`)
	SHA1RegEx := regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

	// Get the OS path separator for dealing with local files
	OSPathSeparator := string(os.PathSeparator)

	// If fail tracker use is requested, read in the fail tracker file for later usage
	var FailTrackerPath string
	var LastFailTracker string
	if useFailTracker {
		// Assume path to fail tracker from current yaml config and hard coded file name
		FailTrackerPath = config.Controller.RepositoryPath + OSPathSeparator + ".failtracker.meta"

		// Read in contents of fail tracker file
		LastFailTrackerBytes, err := os.ReadFile(FailTrackerPath)
		LastFailTracker = string(LastFailTrackerBytes)
		logError("Failed to read last fail tracker file", err, false)
	}

	// Show progress to user
	fmt.Printf("Complete.\n")
	fmt.Printf("Parsing committed files... ")

	// Open the repository
	repo, err := git.PlainOpen(config.Controller.RepositoryPath)
	logError("Failed to open repository", err, true)

	// Get the pointer to the HEAD commit
	ref, err := repo.Head()
	logError("Failed to get HEAD reference", err, true)
	headID := ref.Hash()

	// Figure out what commit ID to use for this deployment
	var commitHash plumbing.Hash
	if commitID != "" { // User supplied commit
		// Verify commit ID string content - only truly required when user specifies it - but verify anyways
		if !SHA1RegEx.MatchString(commitID) {
			logError("Error with supplied commit ID", fmt.Errorf("hash is not 40 characters and/or is not hexadecimal"), true)
		}

		// Set hash
		commitHash = plumbing.NewHash(commitID)
	} else if commitID != "" && useFailTracker { // User attempting to supply commit ID with failtrack - not allowed (user could potentially supply a commit id that doesnt have the correct files)
		// Return to main with error to user
		fmt.Printf("Refusing to use user supplied commit ID and fail tracker at the same time (commit ID is already in the fail tracker, do not specify it manually)\n")
		return
	} else if commitID == "" && useFailTracker { // Failtracker supplied commit
		// Regex to match commitid line from fail tracker
		FailCommitRegEx, err := regexp.Compile(`commitid:([0-9a-fA-F]+)\n`)
		logError("Failed to compile FailTracker CommitID regex patterns", err, true)

		// Use regex to extract commit hash from line in fail tracker (should be the first line)
		commitRegexMatches := FailCommitRegEx.FindStringSubmatch(LastFailTracker)

		// Save the retrieved ID to the string and the raw hash
		commitID = commitRegexMatches[1]
		commitHash = plumbing.NewHash(commitRegexMatches[1])

		// Remove commit line from the failtracker contents using the commit regex
		LastFailTracker = FailCommitRegEx.ReplaceAllString(LastFailTracker, "")
	} else if commitID == "" && !useFailTracker && manualDeploy { // User attempted manual deploy without commit id
		// Return to main with error to user
		fmt.Printf("Please specify a commit ID if you want to initiate a manual deployment\n")
		return
	} else { // Automatic deploy - using head commit
		commitHash = plumbing.NewHash(headID.String())
		// string version for fail tracker output
		commitID = headID.String()
	}

	// Get the commit
	commit, err := repo.CommitObject(commitHash)
	logError("Failed to get commit object", err, true)

	// Get the tree from the commit
	tree, err := commit.Tree()
	logError("Failed to get commit tree", err, true)

	// Get the parent commit
	parentCommit, err := commit.Parents().Next()
	logError("Failed to get the parent commit", err, true)

	// Get the diff between the commits
	patch, err := parentCommit.Patch(commit)
	logError("Failed to determine different between commits", err, true)

	// Map to store file paths and actions to be done on remote host
	HostsAndFiles := make(map[string]string)

	// Array to store hosts names valid for this commit that might be used for deployment
	var FilteredCommitHostNames []string

	// Determine what to do with each file in the commit
	// TODO: figure a better way to handle from and to with less duplication
	for _, file := range patch.FilePatches() {
		// Get the old file and new file info
		from, to := file.Files()

		// Pre declare
		var fromPath, toPath string
		var commitHostFileFrom string
		var commitHostFileTo string
		var commitFileFromType string
		var commitFileToType string
		var err error

		// Retrieve the paths and file types
		if from != nil {
			// Get the path
			fromPath = from.Path()

			if fromPath != "" {
				// Retrieve the host directory name for this file
				fileDirNames := strings.SplitN(toPath, OSPathSeparator, 2)
				commitHostFileFrom = fileDirNames[0]

				// Retrieve the type for this file
				commitFileFromType, err = determineFileType(fromPath)
				logError("Error determining commit file type", err, true)
			}
		}
		if to != nil {
			// Get the path
			toPath = to.Path()

			if toPath != "" {
				// Retrieve the host directory name for this file
				fileDirNames := strings.SplitN(toPath, OSPathSeparator, 2)
				commitHostFileTo = fileDirNames[0]

				// Retrieve the type for this file
				commitFileToType, err = determineFileType(toPath)
				logError("Error determining commit file type", err, true)
			}
		}

		// Skip unsupported file types
		if commitFileFromType == "unsupported" || commitFileToType == "unsupported" {
			continue
		}

		// Always ignore files in root of repository
		if !strings.ContainsRune(toPath, []rune(OSPathSeparator)[0]) && toPath != "" {
			continue
		} else if !strings.ContainsRune(fromPath, []rune(OSPathSeparator)[0]) && fromPath != "" {
			continue
		}

		// Add file to map depending on how it changed in this commit
		if from == nil {
			// Newly created files
			HostsAndFiles[toPath] = "create"
		} else if to == nil {
			// Deleted Files
			HostsAndFiles[fromPath] = "delete"
		} else if fromPath != toPath {
			// Changed Files - deleting if original was mv instead of cp
			_, err := os.Stat(fromPath)
			if os.IsNotExist(err) {
				HostsAndFiles[fromPath] = "delete"
			}
			HostsAndFiles[toPath] = "create"
		} else {
			// Anything else - usually editted in place files
			HostsAndFiles[fromPath] = "create"
		}

		// Check for sym links in commit and add correct tag for handling creation of sym links on target
		if commitFileFromType == "symlink" && HostsAndFiles[fromPath] == "create" {
			// Get the target path of the sym link target and ensure it is valid
			targetPath, err := ResolveLinkToTarget(fromPath, OSPathSeparator)
			logError("Failed to parse symbolic link in commit", err, true)

			// Add new action to this file that includes the expected target path for the link
			HostsAndFiles[fromPath] = "symlinkcreate to target " + targetPath
		}
		if commitFileToType == "symlink" && HostsAndFiles[toPath] == "create" {
			// Get the target path of the sym link target and ensure it is valid
			targetPath, err := ResolveLinkToTarget(toPath, OSPathSeparator)
			logError("Failed to parse symbolic link in commit", err, true)

			// Add new action to this file that includes the expected target path for the link
			HostsAndFiles[toPath] = "symlinkcreate to target " + targetPath
		}

		// Ensure the commit host directory name is a valid hostname in yaml config.DeployerEndpoints
		var configContainsCommitHost bool
		for availableHost := range config.DeployerEndpoints {
			if commitHostFileTo == availableHost || commitHostFileTo == config.TemplateDirectory {
				configContainsCommitHost = true
				break
			}
			if commitHostFileFrom == availableHost || commitHostFileTo == config.TemplateDirectory {
				configContainsCommitHost = true
				break
			}
			configContainsCommitHost = false
		}
		if !configContainsCommitHost {
			logError("Error processing host", fmt.Errorf("commit host directory %s %s has no matching DeployerEndpoints host in YAML config", commitHostFileFrom, commitHostFileTo), true)
		}

		// Add filtered target commit host to array depending on if from and to is split between host dirs
		if commitHostFileFrom == commitHostFileTo {
			FilteredCommitHostNames = append(FilteredCommitHostNames, commitHostFileFrom)
		} else {
			FilteredCommitHostNames = append(FilteredCommitHostNames, commitHostFileFrom)
			FilteredCommitHostNames = append(FilteredCommitHostNames, commitHostFileTo)
		}
	}

	// Get list of all files in repo tree
	repoFiles := tree.Files()

	// Record all files in repo to the all files map
	AllRepoFiles := make(map[string]string)
	// This might need to have the same logic as above for deleted files, moved files, sym links, ect
	for {
		// Go to next file in list
		repoFile, err := repoFiles.Next()

		// Break at end of list
		if err == io.EOF {
			err = nil
			break
		}

		// Fail if next file doesnt work
		if err != nil {
			logError("Error processing commit file", err, true)
		}

		// Always ignore files in root of repository
		if !strings.ContainsRune(repoFile.Name, []rune(OSPathSeparator)[0]) {
			continue
		}

		// Append the file path to the slice.
		AllRepoFiles[repoFile.Name] = "create"
	}

	// Parse out all files in repo for use in dedup'ing
	AllHostsAndFilesMap := make(map[string][]string)
	for filePath := range AllRepoFiles {
		// Isolate host and path
		commitSplit := strings.SplitN(filePath, OSPathSeparator, 2)
		commitHost := commitSplit[0]
		commitPath := commitSplit[1]

		// Create map
		AllHostsAndFilesMap[commitHost] = append(AllHostsAndFilesMap[commitHost], commitPath)
	}

	// Show progress to user
	fmt.Printf("Complete.\n")
	fmt.Printf("Filtering deployment hosts... ")

	// Override deployment for hosts for fail tracker use
	if useFailTracker {
		// empty hostOverride prior to reading in failed hosts
		hostOverride = ""

		// Create new map HostsAndFiles
		CommitFileOverride := make(map[string]string)
		FailLines := strings.Split(LastFailTracker, "\n")
		for _, fail := range FailLines {
			// Use global struct for errors json format
			var errorInfo ErrorInfo

			// Skip any empty lines
			if fail == "" {
				continue
			}

			// Unmarshal the line into vars
			err := json.Unmarshal([]byte(fail), &errorInfo)
			logError("Failed to unmarshal fail tracker json line", err, false)

			// Create new hostsandfiles array by only using the files that have matches in errorInfo
			for commitfile := range HostsAndFiles {
				// Separate parent dir from target file path
				commitSplit := strings.SplitN(commitfile, OSPathSeparator, 2)
				commitPath := OSPathSeparator + commitSplit[1]

				// Only add files that error'd out last time (add them all if an entire host failed - as evident by N/A)
				if errorInfo.FilePath == commitPath || errorInfo.FilePath == "N/A" {
					// Add it to array in the format of repo file paths
					CommitFileOverride[commitfile] = HostsAndFiles[commitfile]
				}
			}

			// Add failed hosts to override var to isolate host deployment loop to only failed hosts
			hostOverride = hostOverride + errorInfo.EndpointName + ","
		}

		// Overwrite new list of files to deploy (from that commit) to the HostsAndFiles array
		HostsAndFiles = CommitFileOverride
	}

	// Do not allow all files to be deployed to all hosts
	if useAllRepoFiles && hostOverride == "" {
		logError("Must specify hosts when deploying all repository files", fmt.Errorf("illegal: will not deploy every file to all remotes"), false)
	}

	// Counters for metrics
	var preDeploymentHosts int
	var preDeployedConfigs int

	// The Maps - by host (and extra for file processing)
	HostsAndFilePaths := make(map[string][]string)    // Map of hosts and their arrays of file paths
	HostsAndEndpointInfo := make(map[string][]string) // Map of hosts and their associated endpoint information ([0]=Socket, [1]=User)
	var targetEndpoints []string                      // Array of hosts to connect to
	var AllLocalFiles []string                        // Array of all files that will get deployed to all hosts

	// Loop hosts in config and prepare relevant host information for deployment
	for endpointName, endpointInfo := range config.DeployerEndpoints {
		// Used for fail tracker manual deployments - skip this host if not in override (if override was requested)
		SkipHost := checkForHostOverride(hostOverride, endpointName)
		if SkipHost {
			continue
		}

		// Ensure processing is only done for hosts which might have a config deployed - as identified by parse git commit function
		var noHostMatchFound bool
		for _, targetHost := range FilteredCommitHostNames {
			if endpointName == targetHost || targetHost == config.TemplateDirectory {
				noHostMatchFound = false
				break
			}
			noHostMatchFound = true
		}
		if noHostMatchFound {
			continue
		}

		// Extract var for endpoint information
		endpointUser := endpointInfo[2].EndpointUser

		// Network Pre-Checks and Parsing
		endpoint, err := ParseEndpointAddress(endpointInfo[0].Endpoint, endpointInfo[1].EndpointPort)
		logError(fmt.Sprintf("Error parsing host '%s' network address", endpointName), err, true)

		// Add endpoint info to The Maps
		HostsAndEndpointInfo[endpointName] = append(HostsAndEndpointInfo[endpointName], endpoint, endpointUser)

		// If the ignore index of the host is present, read in bool - for use in deduping
		var ignoreTemplates bool
		if len(endpointInfo) == 4 {
			ignoreTemplates = endpointInfo[3].IgnoreTemplates
		}

		// Find and remove duplicate conf files between Template and specific host directory, then exclude duplicates in the template directory from that host
		// IMPORTANT to ensure config files for certain hosts are not blown away by the equivalent template config, even if the specific host conf wasn't edited in this commit
		FilteredCommitFilePaths := deDupsHostsandTemplateCommits(HostsAndFiles, config.TemplateDirectory, AllHostsAndFilesMap, endpointName, OSPathSeparator, ignoreTemplates)

		// Re-check if there are any configs to deploy after dedup, skip this host if so
		if len(FilteredCommitFilePaths) == 0 {
			continue
		}

		// Add the filtered paths to host specific map and the combined all files map
		for _, commitPath := range FilteredCommitFilePaths {
			// Paths as is for local os for loading the files
			AllLocalFiles = append(AllLocalFiles, commitPath)

			// Paths in correct format for deployment to remote linux host
			hostCommitPath := strings.ReplaceAll(commitPath, OSPathSeparator, "/")
			HostsAndFilePaths[endpointName] = append(HostsAndFilePaths[endpointName], hostCommitPath)
		}

		// Add filtered endpoints to The Maps (array) - this is the main reference for which hosts will have a routine spawned
		targetEndpoints = append(targetEndpoints, endpointName)

		// Increment count of hosts to be deployed for metrics
		preDeploymentHosts++
	}

	// Show progress to user
	fmt.Printf("Complete.\n")
	fmt.Printf("Loading deployment files... ")

	// The Maps for target files - agnostic of host - keys are commit file paths (like host1/etc/resolv.conf)
	HostsAndFileData := make(map[string]string)                     // Map of target file paths and their associated content
	HostsAndFileMetadata := make(map[string]map[string]interface{}) // Map of target file paths and their associated extracted metadata
	HostsAndFileDataHashes := make(map[string]string)               // Map of target file paths and their associated content hashes
	HostsAndFileActions := make(map[string]string)                  // Map of target file paths and their associated file actions

	// Load file contents, metadata, hashes, and actions into their own maps
	for _, commitFilePath := range AllLocalFiles {
		// Ensure paths for deployment have correct separate for linux
		filePath := strings.ReplaceAll(commitFilePath, OSPathSeparator, "/")
		// As a reminder
		// filePath		should be identical to the full path of files in the repo except hard coded to forward slash path separators
		// commitFilePath	should be identical to the full path of files in the repo (meaning following the build OS file path separators)

		// Skip loading if file will be deleted
		if HostsAndFiles[commitFilePath] == "delete" {
			// But, add it to the deploy target files so it can be deleted during ssh
			HostsAndFileActions[filePath] = HostsAndFiles[commitFilePath]
			continue
		}

		// Skip loading if file is sym link
		if strings.Contains(HostsAndFiles[commitFilePath], "symlinkcreate") {
			// But, add it to the deploy target files so it can be ln'd during ssh
			HostsAndFileActions[filePath] = HostsAndFiles[commitFilePath]
			continue
		}

		// Get file from git tree
		file, err := tree.File(commitFilePath)
		if err != nil {
			logError("Error loading file contents when retrieving file from git tree", err, true)
		}

		// Open reader for file contents
		reader, err := file.Reader()
		logError("Error loading file contents when retrieving file reader", err, true)
		defer reader.Close()

		// Read file contents (as bytes)
		content, err := io.ReadAll(reader)
		logError("Error loading file contents when reading file content", err, true)

		// Grab metadata out of contents
		metadata, configContent, err := extractMetadata(string(content))
		logError("Error extracting metadata header from file contents", err, true)

		// SHA256 Hash the metadata-less contents
		contentBytes := []byte(configContent)
		hash := sha256.New()
		hash.Write(contentBytes)
		hashedBytes := hash.Sum(nil)

		// Parse JSON into a generic map
		var jsonMetadata MetaHeader
		err = json.Unmarshal([]byte(metadata), &jsonMetadata)
		logError(fmt.Sprintf("Error parsing JSON metadata header for %s", commitFilePath), err, true)

		// Initialize inner map for metadata
		HostsAndFileMetadata[filePath] = make(map[string]interface{})

		// Save metadata into its own map
		HostsAndFileMetadata[filePath]["FileOwnerGroup"] = jsonMetadata.TargetFileOwnerGroup
		HostsAndFileMetadata[filePath]["FilePermissions"] = jsonMetadata.TargetFilePermissions
		HostsAndFileMetadata[filePath]["ReloadRequired"] = jsonMetadata.ReloadRequired
		HostsAndFileMetadata[filePath]["Reload"] = jsonMetadata.ReloadCommands

		// Save Hashes into its own map
		HostsAndFileDataHashes[filePath] = hex.EncodeToString(hashedBytes)

		// Save content into its own map
		HostsAndFileData[filePath] = configContent

		// Increment config metric counter
		preDeployedConfigs++
	}

	// Show progress to user - using the metrics
	fmt.Printf("Complete.\n")
	fmt.Printf("Beginning deployment of %d configuration(s) to %d host(s)\n", preDeployedConfigs, preDeploymentHosts)

	// Semaphore to limit concurrency of host deployment go routines as specified in main config
	semaphore := make(chan struct{}, config.SSHClient.MaximumConcurrency)

	// Wait group for SSH host go routines
	var wg sync.WaitGroup

	// Start go routines for each remote host ssh
	for _, endpointName := range targetEndpoints {
		// Retrieve info for this host from The Maps
		commitFilePaths := HostsAndFilePaths[endpointName]
		endpointInfo := HostsAndEndpointInfo[endpointName]
		endpointSocket := endpointInfo[0]
		endpointUser := endpointInfo[1]

		// Start go routine for specific host
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker, git commit will NOT be rolled back
		wg.Add(1)
		go deployConfigs(&wg, semaphore, endpointName, commitFilePaths, endpointSocket, endpointUser, HostsAndFileData, HostsAndFileMetadata, HostsAndFileDataHashes, HostsAndFileActions, PrivateKey, config.SSHClient.SudoPassword, SHA256RegEx)
	}

	// Block until all SSH connections are finished
	wg.Wait()

	// Tell user about error and how to redeploy, writing fails to file in repo
	PathToExe := os.Args[0]
	if FailTracker != "" {
		fmt.Printf("\nPARTIAL COMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
		fmt.Printf("Failure(s) in deployment:\n")
		fmt.Printf("%s\n", FailTracker)
		fmt.Printf("Please fix the errors, then run the following command to redeploy (or create new commit if file corrections are needed):\n")
		fmt.Printf("%s -c %s --manual-deploy --use-failtracker-only\n", PathToExe, configFilePath)

		// Add FailTracker string to repo working directory as .failtracker.meta
		FailTrackerPath := config.Controller.RepositoryPath + OSPathSeparator + ".failtracker.meta"
		FailTrackerFile, err := os.Create(FailTrackerPath)
		if err != nil {
			fmt.Printf("Failed to create FailTracker File - manual redeploy using '--use-failtracker-only' will not work. Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A): %v\n", err)
			return
		}
		defer FailTrackerFile.Close()

		// Add commitid line to top of fail tracker
		FailTrackerAndCommit := "commitid:" + commitID + "\n" + FailTracker

		// Write string to file (overwrite old contents)
		_, err = FailTrackerFile.WriteString(FailTrackerAndCommit)
		if err != nil {
			fmt.Printf("Failed to write FailTracker to File - manual redeploy using '--use-failtracker-only' will not work. Please use the above errors to create a new commit with ONLY those failed files (or all per host if file is N/A): %v\n", err)
		}
		return
	}

	// Different exit if no hosts
	if postDeploymentHosts == 0 {
		fmt.Printf("\nINCOMPLETE: No hosts to deploy to\n")
		if useFailTracker {
			fmt.Printf("Better find out why there are none (this is probably a bug), then use this command to try again:\n")
			fmt.Printf("%s --manual-deploy --commitid %s --use-failtracker-only\n", PathToExe, commitID)
		}
		return
	}

	// Remove fail tracker file after successful redeployment - removal errors don't matter.
	if useFailTracker {
		os.Remove(FailTrackerPath)
	}

	// Show progress to user
	fmt.Printf("\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
}

// ###################################
//      HOST DEPLOYMENT HANDLING
// ###################################

func deployConfigs(wg *sync.WaitGroup, semaphore chan struct{}, endpointName string, commitFilePaths []string, endpointSocket string, endpointUser string, commitFileData map[string]string, commitFileMetadata map[string]map[string]interface{}, commitFileDataHashes map[string]string, commitFileActions map[string]string, PrivateKey ssh.Signer, SudoPassword string, SHA256RegEx *regexp.Regexp) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError(fmt.Sprintf("Controller panic during deployment to host '%s'", endpointName), fmt.Errorf("%v", fatalError), false)
		}
	}()

	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	// Connect to the SSH server
	client, err := connectToSSH(endpointSocket, endpointUser, PrivateKey)
	if err != nil {
		hostDeployFailCleanup(endpointName, "", fmt.Errorf("failed connect to SSH server %v", err))
		return
	}
	defer client.Close()

	// Loop through target files and deploy
	backupConfCreated := false
	for _, commitFilePath := range commitFilePaths {
		// Split repository host dir and config file path for obtaining the absolute target file path
		commitSplit := strings.SplitN(commitFilePath, "/", 2)
		commitPath := commitSplit[1]
		targetFilePath := "/" + commitPath
		// Reminder:
		// targetFilePath   should be the file path as expected on the remote system
		// commitFilePath   should be the local file path within the commit repository - is REQUIRED to reference keys in the global config information maps

		var command string
		var CommandOutput string
		targetFileAction := commitFileActions[commitFilePath]

		// If git file was deleted, attempt to delete file any empty folders above - failures here should not stop deployment to this host
		// Note: technically inefficient; if a file is moved within same directory, this will delete the file and parent dir(maybe)
		//                                then when deploying the moved file, it will recreate folder that was just deleted.
		if targetFileAction == "delete" {
			// Attempt remove file and any backup for that file
			command = "rm " + targetFilePath + " " + targetFilePath + ".old"
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				// Ignore specific error if one one isnt there but the other is
				if !strings.Contains(err.Error(), "No such file or directory") {
					fmt.Printf("Warning: Host %s: failed to remove file '%s': %v\n", endpointName, targetFilePath, err)
				}
			}
			// Danger Zone: Remove empty parent dirs
			targetPath := filepath.Dir(targetFilePath)
			maxLoopCount := 64001 // for safety - max ext4 sub dirs (but its sane enough for other fs which have super high limits)
			for i := 0; i < maxLoopCount; i++ {
				// Check for presence of anything in dir
				command = "ls -A " + targetPath
				CommandOutput, _ = RunSSHCommand(client, command, SudoPassword)

				// Empty stdout means empty dir
				if CommandOutput == "" {
					// Safe remove directory
					command = "rmdir " + targetPath
					_, err = RunSSHCommand(client, command, SudoPassword)
					if err != nil {
						// Error breaks loop
						fmt.Printf("Warning: Host %s: failed to remove empty parent directory '%s' for file '%s': %v\n", endpointName, targetPath, targetFilePath, err)
						break
					}

					// Set the next loop dir to be one above
					targetPath = filepath.Dir(targetPath)
					continue
				}

				// Leave loop when a parent dir has something in it
				break
			}

			// Next target file to deploy for this host
			continue
		}

		// Create symbolic link if requested
		if strings.Contains(targetFileAction, "symlinkcreate") {
			// Check if a file is already there - if so, error
			OldSymLinkExists, err := CheckRemoteFileExistence(client, targetFilePath, SudoPassword)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("error checking file existence before creating symbolic link: %v", err))
				continue
			}
			if OldSymLinkExists {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("error file already exists where symbolic link is supposed to be created"))
				continue
			}

			// Extract target path
			tgtActionSplitReady := strings.ReplaceAll(targetFileAction, " to target ", "?")
			targetActionArray := strings.SplitN(tgtActionSplitReady, "?", 2)
			symLinkTarget := targetActionArray[1]

			// Create symbolic link
			command = "ln -s " + symLinkTarget + " " + targetFilePath
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("error creating symbolic link: %v", err))
				continue
			}
			continue
		}

		// Parse out Metadata Map into vars
		TargetFileOwnerGroup := commitFileMetadata[commitFilePath]["FileOwnerGroup"].(string)
		TargetFilePermissions := commitFileMetadata[commitFilePath]["FilePermissions"].(int)
		ReloadRequired := commitFileMetadata[commitFilePath]["ReloadRequired"].(bool)
		ReloadCommands := commitFileMetadata[commitFilePath]["Reload"].([]string)

		// Find if target file exists on remote
		OldFileExists, err := CheckRemoteFileExistence(client, targetFilePath, SudoPassword)
		if err != nil {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("error checking file presence on remote host: %v", err))
			continue
		}

		// If file exists, Hash remote file
		var OldRemoteFileHash string
		if OldFileExists {
			// Get the SHA256 hash of the remote old conf file
			command = "sha256sum " + targetFilePath
			CommandOutput, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err))
				continue
			}

			// Parse hash command output to get just the hex
			OldRemoteFileHash = SHA256RegEx.FindString(CommandOutput)

			// Compare hashes and go to next file deployment if remote is same as local
			if OldRemoteFileHash == commitFileDataHashes[commitFilePath] {
				fmt.Printf("\rFile '%s' on Host '%s' identical to committed file... skipping deployment for this file\n", targetFilePath, endpointName)
				continue
			}

			// Backup old config
			command = "cp -p " + targetFilePath + " " + targetFilePath + ".old"
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("error making backup of old config file: %v", err))
				continue
			}

			// Ensure old restore only happens if a backup was created
			backupConfCreated = true
		}

		// Transfer local file to remote
		err = TransferFile(client, commitFileData[commitFilePath], targetFilePath, SudoPassword)
		if err != nil {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed SFTP config file transfer to remote host: %v", err))
			err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed Old Config Restoration: %v", err))
			}
			continue
		}

		// Check if deployed file is present on disk
		NewFileExists, err := CheckRemoteFileExistence(client, targetFilePath, SudoPassword)
		if err != nil {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("error checking deployed file presence on remote host: %v", err))
			err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed Old Config Restoration: %v", err))
			}
			continue
		}
		if !NewFileExists {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("deployed file on remote host is not present after file transfer"))
			err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		// Get Hash of new deployed conf file
		command = "sha256sum " + targetFilePath
		CommandOutput, err = RunSSHCommand(client, command, SudoPassword)
		if err != nil {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed SSH Command on host during hash of deployed file: %v", err))
			err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		// Parse hash command output to get just the hex
		NewRemoteFileHash := SHA256RegEx.FindString(CommandOutput)

		// Compare hashes and restore old conf if they dont match
		if NewRemoteFileHash != commitFileDataHashes[commitFilePath] {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("error: hash of config file post deployment does not match hash of pre deployment"))
			err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		command = "chown " + TargetFileOwnerGroup + " " + targetFilePath
		_, err = RunSSHCommand(client, command, SudoPassword)
		if err != nil {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed SSH Command on host during owner/group change: %v", err))
			err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		command = "chmod " + strconv.Itoa(TargetFilePermissions) + " " + targetFilePath
		_, err = RunSSHCommand(client, command, SudoPassword)
		if err != nil {
			hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed SSH Command on host during permissions change: %v", err))
			err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed old config restoration: %v", err))
			}
			continue
		}

		// No reload required, early return
		if !ReloadRequired {
			// Lock and write to metric var - increment suc configs by 1
			MetricCountMutex.Lock()
			postDeployedConfigs++
			MetricCountMutex.Unlock()
			continue
		}

		// Run all the commands required by new config file
		for _, command := range ReloadCommands {
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed SSH Command on host during reload command %s: %v", command, err))
				err := restoreOldConfig(client, targetFilePath, OldRemoteFileHash, SHA256RegEx, SudoPassword, backupConfCreated)
				if err != nil {
					hostDeployFailCleanup(endpointName, targetFilePath, fmt.Errorf("failed old config restoration: %v", err))
				}
				break
			}
		}

		// Lock and write to metric var - increment success configs by 1
		MetricCountMutex.Lock()
		postDeployedConfigs++
		MetricCountMutex.Unlock()
	}

	// Lock and write to metric var - increment success hosts by 1
	MetricCountMutex.Lock()
	postDeploymentHosts++
	MetricCountMutex.Unlock()
}
