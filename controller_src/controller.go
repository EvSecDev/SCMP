// controller
package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"runtime"

	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/journal"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
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
		SSHIdentityFile    string `yaml:"SSHIdentityFile"`
		UseSSHAgent        bool   `yaml:"UseSSHAgent"`
		MaximumConcurrency int    `yaml:"MaximumConnectionsAtOnce"`
		SudoPassword       string `yaml:"SudoPassword"`
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
	ReloadCommands        []string `json:"Reload"`
}

// Fail tracker json line format
type ErrorInfo struct {
	EndpointName string `json:"endpointName"`
	FilePath     string `json:"filePath"`
	ErrorMessage string `json:"errorMessage"`
}

// Know if auto mode or manual mode
var CalledByGitHook bool

// Know if using journald logging
var LogToJournald bool

// Used for rolling back commit upon early failure
var RepositoryPath string

// Used for metrics - counting post deployment
var postDeployedConfigs int
var postDeploymentHosts int

// Lock for metric vars writing
var MetricCountMutex sync.Mutex

// Global to track failed go routines' hosts, files, and errors to be able to retry deployment on user request
var FailTracker string
var FailTrackerMutex sync.Mutex

// ###################################
//      EXCEPTION HANDLING
// ###################################

func logError(errorDescription string, errorMessage error, CleanupNeeded bool) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}
	// If requested, put error in journald
	if LogToJournald {
		err := CreateJournaldLog("", "", fmt.Sprintf("%s: %v", errorDescription, errorMessage))
		if err != nil {
			fmt.Printf("Failed to create journald entry: %v\n", err)
		}
	}
	// Only roll back commit if the program was started by a hook and if the commit rollback is requested
	if CalledByGitHook && CleanupNeeded {
		// Reset commit because the current commit should reflect what is deployed in the network
		fmt.Printf("Warning: Encountered processing error, rolling commit back 1 revision.\n")
		fmt.Printf("Working directory is **NOT** affected.\n")
		// Conceptually, equivalent of this command: git reset --soft HEAD~1
		repo, err := git.PlainOpen(RepositoryPath)
		if err != nil {
			fmt.Printf("Error rolling back commit. Failed to open repository: %v\n", err)
			os.Exit(1)
		}

		// Get the reference for HEAD
		headRef, err := repo.Head()
		if err != nil {
			fmt.Printf("Error rolling back commit. Failed to get HEAD: %v\n", err)
			os.Exit(1)
		}

		// Get the HEAD commit
		headCommit, err := repo.CommitObject(headRef.Hash())
		if err != nil {
			fmt.Printf("Error rolling back commit. Failed to get HEAD commit: %v\n", err)
			os.Exit(1)
		}

		// Find the parent commit of HEAD
		parentHashes := headCommit.ParentHashes
		if len(parentHashes) == 0 {
			fmt.Printf("Error rolling back commit. HEAD does not have a parent commit\n")
			os.Exit(1)
		}
		parentHash := headCommit.ParentHashes[0]

		err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, parentHash))
		if err != nil {
			fmt.Printf("Error rolling back commit. Failed to update HEAD reference: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Please fix the below error then add and commit to restart deployment process.\n")
	}
	fmt.Printf("\n%s: %v\n", errorDescription, errorMessage)
	fmt.Printf("================================================\n")
	os.Exit(1)
}

// Create log entry in journald
func CreateJournaldLog(endpointName string, filePath string, errorMessage string) error {
	// Send entry to journald
	err := journal.Send(errorMessage, journal.PriErr, map[string]string{
		"endpointName": endpointName,
		"filePath":     filePath,
	})
	if err != nil {
		return err
	}

	return nil
}

// Called from within go routines
func hostDeployFailCleanup(endpointName string, filePath string, errorMessage error) {
	if filePath == "" {
		filePath = "N/A"
	}

	// Ensure multiline error messages dont make their way into json
	Message := strings.ReplaceAll(errorMessage.Error(), "\n", " ")
	Message = strings.ReplaceAll(errorMessage.Error(), "\r", " ")

	// Send error to journald
	if LogToJournald {
		err := CreateJournaldLog(endpointName, filePath, Message)
		if err != nil {
			fmt.Printf("Failed to create journald entry: %v\n", err)
		}
	}

	// Parseable one line json for failures
	info := ErrorInfo{
		EndpointName: endpointName,
		FilePath:     filePath,
		ErrorMessage: Message,
	}

	// Marshal info string to a json format
	FailedInfo, err := json.Marshal(info)
	if err != nil {
		fmt.Printf("Failed to create Fail Tracker Entry for host %s file %s\n", endpointName, filePath)
		fmt.Printf("    Error: %s\n", Message)
		return
	}

	// Write (append) fail info for this go routine to global failures
	FailTrackerMutex.Lock()
	FailTracker += string(FailedInfo) + "\n"
	FailTrackerMutex.Unlock()
}

// ###################################
//	MAIN - START
// ###################################

func HelpMenu() {
	fmt.Printf("Usage: %s [OPTIONS]...\n%s", os.Args[0], usage)
}

const usage = `
Examples:
    controller --config </etc/scmpc.yaml> --manual-deploy --commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>
    controller --config </etc/scmpc.yaml> --manual-deploy --use-failtracker-only
    controller --config </etc/scmpc.yaml> --deploy-all --remote-hosts <www,proxy,db01> [--commitid <14a4187d22d2eb38b3ed8c292a180b805467f1f7>]
    controller --config </etc/scmpc.yaml> --deployer-versions [--remote-hosts <www,proxy,db01>]
    controller --config </etc/scmpc.yaml> --deployer-update-file <~/Downloads/deployer> [--remote-hosts <www,proxy,db01>]

Options:
    -c, --config </path/to/yaml>                    Path to the configuration file [default: scmpc.yaml]
    -a, --auto-deploy                               Use latest commit for deployment, normally used by git post-commit hook
    -m, --manual-deploy                             Use specified commit ID for deployment (Requires '--commitid')
    -d, --deploy-all                                Deploy all files in specified commit to specific hosts (Requires '--remote-hosts')
    -r, --remote-hosts <host1,host2,host3...>       Override hosts for deployment
    -C, --commitid <hash>                           Commit ID (hash) of the commit to deploy configurations from
    -f, --use-failtracker-only                      If previous deployment failed, use the failtracker to retry (Requires '--manual-deploy')
        --deployer-versions                         Query remote host deployer executable versions and print to stdout
        --deployer-update-file </path/to/binary>    Upload and update deployer executable with supplied signed ELF file
    -h, --help                                      Show this help menu
    -V, --version                                   Show version and packages
    -v, --versionid                                 Show only version number

Documentation: <https://github.com/EvSecDev/SCMPusher>
`

func main() {
	progVersion := "v1.1.0"

	// Program Argument Variables
	var configFilePath string
	var manualCommitID string
	var hostOverride string
	var deployerUpdateFile string
	var manualDeployFlagExists bool
	var useAllRepoFilesFlag bool
	var useFailTracker bool
	var checkDeployerVersions bool
	var versionFlagExists bool
	var versionNumberFlagExists bool

	// Read Program Arguments
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
	flag.BoolVar(&checkDeployerVersions, "deployer-versions", false, "")
	flag.StringVar(&deployerUpdateFile, "deployer-update-file", "", "")
	flag.BoolVar(&versionFlagExists, "V", false, "")
	flag.BoolVar(&versionFlagExists, "version", false, "")
	flag.BoolVar(&versionNumberFlagExists, "v", false, "")
	flag.BoolVar(&versionNumberFlagExists, "versionid", false, "")

	// Custom help menu
	flag.Usage = HelpMenu
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

	// Grab config file
	yamlConfigFile, err := os.ReadFile(configFilePath)
	logError("Error reading config file", err, false)

	// Parse all configuration options
	var config Config
	err = yaml.Unmarshal(yamlConfigFile, &config)
	logError("Error unmarshaling config file: %v", err, false)

	// Global for awareness (for log func)
	if config.Controller.LogtoJournald {
		LogToJournald = true
	}

	// Automatic Deployment via git post-commit hook
	if CalledByGitHook {
		// Run deployment
		fmt.Printf("==== Secure Configuration Management Pusher ====\n")
		fmt.Printf("Starting automatic deployment\n")

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

	// Get version of remote host deployer binary if requested
	if checkDeployerVersions {
		fmt.Printf("==== Secure Configuration Management Deployer Version Check ====\n")
		fmt.Printf("Deployer executable versions:\n")

		simpleLoopHosts(config, "", hostOverride, true)

		fmt.Printf("================================================================\n")
		os.Exit(0)
	}

	// Push update file for deployer if requested
	if deployerUpdateFile != "" {
		fmt.Printf("==== Secure Configuration Management Deployer Updater  ====\n")
		fmt.Printf("Pushing update for deployer using new executable at %s\n", deployerUpdateFile)

		simpleLoopHosts(config, deployerUpdateFile, hostOverride, false)

		fmt.Printf("               COMPLETE: Updates Pushed\n")
		fmt.Printf("===========================================================\n")
		os.Exit(0)
	}

	// Exit program without any arguments
	fmt.Printf("No arguments specified! Use '-h' or '--help' to guide your way.\n")
}

// ###################################
//      DEPLOYMENT FUNCTION
// ###################################

func Deployment(config Config, manualDeploy bool, commitID string, useFailTracker bool, hostOverride string, useAllRepoFiles bool, configFilePath string) {
	// For git rollback when parsing error
	RepositoryPath = config.Controller.RepositoryPath

	// Recover from panic
	defer func() {
		if r := recover(); r != nil {
			logError("Controller panic while processing deployment", fmt.Errorf("%v", r), true)
		}
	}()

	// Show progress to user
	fmt.Printf("Running local system checks... ")

	// Ensure current working directory is root of git repository from config
	pwd, err := os.Getwd()
	logError("failed to obtain current working directory", err, true)

	// If current dir is not repo, change to it
	if filepath.Clean(pwd) != filepath.Clean(RepositoryPath) {
		err := os.Chdir(RepositoryPath)
		logError("failed to change directory to repository path", err, true)
	}

	// Get list of system network interfaces
	systemNetInterfaces, err := net.Interfaces()
	logError("failed to obtain system network interfaces", err, false)

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
		logError("No active network interfaces found", fmt.Errorf("refusing to attempt configuration deployment"), false)
	}

	// Get SSH Private Key
	PrivateKey, err := SSHIdentityToKey(config.SSHClient.SSHIdentityFile, config.SSHClient.UseSSHAgent)
	logError("Error retrieving SSH private key", err, true)

	// Regex Vars
	SHA256RegEx := regexp.MustCompile(`^[a-fA-F0-9]{64}`)
	SHA1RegEx := regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

	// Get the OS path separator for parsing local config files
	OSPathSeparator := string(os.PathSeparator)

	// If fail tracker choice, read in the file for later usage
	var FailTrackerPath string
	var LastFailTracker string
	if useFailTracker {
		FailTrackerPath = config.Controller.RepositoryPath + OSPathSeparator + ".failtracker.meta"

		// Read in contents of fail tracker
		LastFailTrackerBytes, err := os.ReadFile(FailTrackerPath)
		LastFailTracker = string(LastFailTrackerBytes)
		logError("Failed to read last fail tracker file", err, false)
	}

	// Show progress to user
	fmt.Printf("Checks Complete.\n")
	fmt.Printf("Parsing committed configuration files... ")

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
		// Verify commit ID string content
		if !SHA1RegEx.MatchString(commitID) && !useFailTracker {
			logError("Error with supplied commit ID", fmt.Errorf("hash is not 40 characters and/or is not hexadecimal"), true)
		}

		// Set hash
		commitHash = plumbing.NewHash(commitID)
	} else if commitID == "" && useFailTracker { // Failtracker supplied commit
		// Regex to match commitid line from fail tracker
		FailCommitRegEx, err := regexp.Compile(`commitid:([0-9a-fA-F]+)\n`)
		logError("Failed to compile FailTracker CommitID regex patterns", err, true)

		// Use regex to extract commit hash from line in fail tracker
		commitRegexMatches := FailCommitRegEx.FindStringSubmatch(LastFailTracker)

		// Save the retrieved ID to the string and the raw hash
		commitID = commitRegexMatches[1]
		commitHash = plumbing.NewHash(commitRegexMatches[1])

		// Remove commit line from the failtracker contents
		LastFailTracker = FailCommitRegEx.ReplaceAllString(LastFailTracker, "")
	} else if commitID == "" && !useFailTracker && manualDeploy { // User attempted manual deploy without commit
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

	// Array of hosts in config
	var DeployerHosts []string
	for host := range config.DeployerEndpoints {
		DeployerHosts = append(DeployerHosts, host)
	}

	// Parse out git commit hosts and files
	HostsAndFiles, gitCommitTree, AllRepoFiles, FilteredCommitHostNames, err := parseGitCommit(commit, config.TemplateDirectory, DeployerHosts, OSPathSeparator)
	logError("Error parsing commit files", err, true)

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

	// Override deployment for hosts for fail tracker use
	var RemoteHostOverride []string
	if useFailTracker {
		// Create new HostsAndFiles
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

				// Only add files that error'd out last time (add them all if an entire host failed)
				if errorInfo.FilePath == commitPath || errorInfo.FilePath == "N/A" {
					// Add it to array in the format of repo file paths
					CommitFileOverride[commitfile] = HostsAndFiles[commitfile]
				}
			}

			// Add failed hosts to override var to isolate host deployment loop to only failed hosts
			RemoteHostOverride = append(RemoteHostOverride, errorInfo.EndpointName)
		}

		// Overwrite new list of files to deploy (from that commit) to the HostsAndFiles array
		HostsAndFiles = CommitFileOverride
	}

	// If user specified a host, overwrite choice to override
	if useAllRepoFiles {
		if hostOverride == "" {
			logError("Must specify hosts when deploying all repository files", fmt.Errorf("illegal: will not deploy every file to all remotes"), false)
		}

		// Set host override to user choice
		hostChoices := strings.Split(hostOverride, ",")
		for _, host := range hostChoices {
			RemoteHostOverride = append(RemoteHostOverride, host)
		}

		// Override deployment files with everything in repo
		HostsAndFiles = AllRepoFiles
	}

	// The Maps - All the information and data needed, parsed into maps per host
	var targetEndpoints []string                                               // Array of hosts to connect to
	HostsAndFilePaths := make(map[string]map[string]string)                    // Map of hosts and their arrays of target file paths and associated tagged actions
	HostsAndEndpointInfo := make(map[string][]string)                          // Map of hosts and their associated endpoint information ([0]=Socket, [1]=User)
	HostsAndFileData := make(map[string]map[string]string)                     // Map of hosts and their arrays of file paths and associated contents
	HostsAndFileMetadata := make(map[string]map[string]map[string]interface{}) // Map of hosts and their arrays of file paths and associated metadata headers
	HostsAndFileDataHashes := make(map[string]map[string]string)               // Map of hosts and their arrays of file paths and associated content hashes

	// Loop hosts in config and prepare all relevant information for deployment per host
	var preDeployedConfigs int
	var preDeploymentHosts int
	for endpointName, endpointInfo := range config.DeployerEndpoints {
		// Process host names

		// Used for fail tracker manual deployments - if host overrides exists
		if len(RemoteHostOverride) > 0 {
			var HostInOverride bool
			// Identify if host is in the override or not
			for _, overridehost := range RemoteHostOverride {
				if overridehost == endpointName {
					HostInOverride = true
					break
				}
				// No matches
				HostInOverride = false
			}
			// Next host deploy loop if host is not in overrides
			if !HostInOverride {
				continue
			}
		}

		// Ensure processing is only done for hosts which might have a config deployed
		var noHostMatchFound bool
		for _, targetHost := range FilteredCommitHostNames {
			noHostMatchFound = false
			if endpointName == targetHost || targetHost == config.TemplateDirectory {
				break
			}
			noHostMatchFound = true
		}
		if noHostMatchFound {
			continue
		}

		// Process network info

		// Extract var for endpoint information
		endpointUser := endpointInfo[2].EndpointUser

		// Network Pre-Checks and Setup
		endpoint, err := ParseEndpointAddress(endpointInfo[0].Endpoint, endpointInfo[1].EndpointPort)
		logError(fmt.Sprintf("Error parsing host '%s' network address", endpointName), err, true)

		// If the ignore index is present, read in bool
		var ignoreTemplates bool
		if len(endpointInfo) == 4 {
			ignoreTemplates = endpointInfo[3].IgnoreTemplates
		}

		// Add endpoint info to The Maps
		HostsAndEndpointInfo[endpointName] = append(HostsAndEndpointInfo[endpointName], endpoint, endpointUser)

		// Process configs

		// Find and remove duplicate conf files between Template and specific host directory, then exclude those dups in the template directory
		// IMPORTANT to ensure config files for certain hosts are not blown away by the equivalent template config, even if the specific host conf wasn't edited in this commit
		FilteredHostsAndFiles := deDupsHostsandTemplateCommits(HostsAndFiles, config.TemplateDirectory, AllHostsAndFilesMap, endpointName, OSPathSeparator)

		// Re-check if there are any configs to deploy after dedup, skip this host if so
		if len(FilteredHostsAndFiles) == 0 {
			continue
		}

		// Add filtered endpoints to The Maps (but its just an array)
		targetEndpoints = append(targetEndpoints, endpointName)

		// Init inner map for paths
		HostsAndFilePaths[endpointName] = make(map[string]string)

		// Load file contents into memory
		ConfigFileContents := make(map[string]string)
		for _, filePath := range FilteredHostsAndFiles {
			path := strings.ReplaceAll(filePath, OSPathSeparator, "/")
			commitSplit := strings.SplitN(path, "/", 2)
			commitPath := commitSplit[1]
			AbsolutePath := "/" + commitPath

			// Skip loading template dir files if configured
			if ignoreTemplates == true {
				continue
			}

			// Skip loading if file will be deleted
			if HostsAndFiles[filePath] == "delete" {
				// But, add it to the deploy target files so it can be deleted during ssh
				HostsAndFilePaths[endpointName][AbsolutePath] = HostsAndFiles[filePath]
				continue
			}

			// Skip loading if file is sym link
			if strings.Contains(HostsAndFiles[filePath], "symlinkcreate") {
				// But, add it to the deploy target files so it can be ln'd during ssh
				HostsAndFilePaths[endpointName][AbsolutePath] = HostsAndFiles[filePath]
				continue
			}

			// Get file from git tree
			file, err := gitCommitTree.File(filePath)
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

			// Store the content in the map
			ConfigFileContents[filePath] = string(content)
		}

		// Initialize inner maps for The Maps
		HostsAndFileData[endpointName] = make(map[string]string)
		HostsAndFileMetadata[endpointName] = make(map[string]map[string]interface{})
		HostsAndFileDataHashes[endpointName] = make(map[string]string)

		// Extract Absolute Paths, Metadata, Content, and Hash contents then store in The Maps
		for filePath, content := range ConfigFileContents {
			// Grab metadata out of contents
			metadata, configContent, err := extractMetadata(content)
			logError("Error extracting metadata header from file contents", err, true)

			// Make key in maps reference-able in other maps - using expected target paths (no longer using local os paths - hardcoded to /
			path := strings.ReplaceAll(filePath, OSPathSeparator, "/")
			commitSplit := strings.SplitN(path, "/", 2)
			commitPath := commitSplit[1]
			AbsolutePath := "/" + commitPath

			// Parse JSON into a generic map
			var jsonMetadata MetaHeader
			err = json.Unmarshal([]byte(metadata), &jsonMetadata)
			logError(fmt.Sprintf("Error parsing JSON metadata header for %s", path), err, true)

			// Initialize inner map
			HostsAndFileMetadata[endpointName][AbsolutePath] = make(map[string]interface{})
			// Save metadata into its own map inside The Maps
			HostsAndFileMetadata[endpointName][AbsolutePath]["FileOwnerGroup"] = jsonMetadata.TargetFileOwnerGroup
			HostsAndFileMetadata[endpointName][AbsolutePath]["FilePermissions"] = jsonMetadata.TargetFilePermissions
			HostsAndFileMetadata[endpointName][AbsolutePath]["ReloadRequired"] = jsonMetadata.ReloadRequired
			HostsAndFileMetadata[endpointName][AbsolutePath]["Reload"] = jsonMetadata.ReloadCommands

			// SHA256 Hash the contents and store in its own map inside The Maps
			contentBytes := []byte(configContent)
			hash := sha256.New()
			hash.Write(contentBytes)
			hashedBytes := hash.Sum(nil)
			HostsAndFileDataHashes[endpointName][AbsolutePath] = hex.EncodeToString(hashedBytes)

			// Overwrite file contents with metadataless content in its own map inside The Maps
			HostsAndFileData[endpointName][AbsolutePath] = configContent

			// Target file paths with the corresponding tag value
			HostsAndFilePaths[endpointName][AbsolutePath] = HostsAndFiles[filePath]

			// Increment config metric counter by 1
			preDeployedConfigs++
		}

		// Increment count of hosts to be deployed for metrics
		preDeploymentHosts++
	}

	// Show progress to user
	fmt.Printf("Parsing Complete.\n")
	fmt.Printf("Beginning deployment of %d configuration(s) to %d host(s)\n", preDeployedConfigs, preDeploymentHosts)

	// Semaphore to limit concurrency of host deployment go routines
	semaphore := make(chan struct{}, config.SSHClient.MaximumConcurrency)

	// Wait group for SSH host go routines
	var wg sync.WaitGroup

	// Start go routines for each remote host ssh
	for _, endpointName := range targetEndpoints {
		// Retrieve info for this host from The Maps
		TargetFilePaths := HostsAndFilePaths[endpointName]
		endpointInfo := HostsAndEndpointInfo[endpointName]
		endpointSocket := endpointInfo[0]
		endpointUser := endpointInfo[1]
		ConfigFileData := HostsAndFileData[endpointName]
		ConfigMetadata := HostsAndFileMetadata[endpointName]
		ConfigDataHashes := HostsAndFileDataHashes[endpointName]

		// Start go routine for specific host
		// All failures and errors from here on are soft stops - program will finish, errors are tracked with global FailTracker
		wg.Add(1)
		go deployConfigs(&wg, semaphore, endpointName, TargetFilePaths, endpointSocket, endpointUser, ConfigFileData, ConfigMetadata, ConfigDataHashes, PrivateKey, config.SSHClient.SudoPassword, SHA256RegEx)
	}

	// Block until all SSH connections are finished
	wg.Wait()

	// Tell user about error and how to redeploy, writing fails to file in repo
	PathToExe := os.Args[0]
	if FailTracker != "" {
		fmt.Printf("\nPARTIAL COMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
		fmt.Printf("Failure(s) in deployment:\n")
		fmt.Printf("%s\n", FailTracker)
		fmt.Printf("Please fix the errors, then run the following command to redeploy:\n")
		fmt.Printf("%s -c %s --manual-deploy --use-failtracker-only\n", PathToExe, configFilePath)

		// Add FailTracker string to repo working directory as .failtracker.meta
		FailTrackerPath := config.Controller.RepositoryPath + OSPathSeparator + ".failtracker.meta"
		FailTrackerFile, err := os.Create(FailTrackerPath)
		if err != nil {
			fmt.Printf("Failed to create FailTracker File - manual redeploy using '--use-failtracker-only' will not work. Please use the above errors to create a new commit with just those files: %v\n", err)
			return
		}
		defer FailTrackerFile.Close()

		// Add commitid line to top of fail tracker
		FailTrackerAndCommit := "commitid:" + commitID + "\n" + FailTracker

		// Write string to file (overwrite old contents)
		_, err = FailTrackerFile.WriteString(FailTrackerAndCommit)
		if err != nil {
			fmt.Printf("Failed to write FailTracker to file - manual redeploy using '--use-failtracker-only' will not work. Please use the above errors to create a new commit with just those files: %v\n", err)
		}
		return
	}

	// Different exit if no hosts
	if postDeploymentHosts == 0 {
		fmt.Printf("\nINCOMPLETE: No hosts to deploy to\n")
		if useFailTracker {
			fmt.Printf("Better find out why there are none, then use this command to try again:\n")
			fmt.Printf("%s --manual-deploy --commitid %s --use-failtracker-only\n", PathToExe, commitID)
		}
		return
	}

	// Remove fail tracker file after successful redeployment - removal errors don't matter.
	if useFailTracker {
		_ = os.Remove(FailTrackerPath)
	}
	fmt.Printf("\nCOMPLETE: %d configuration(s) deployed to %d host(s)\n", postDeployedConfigs, postDeploymentHosts)
}

// ###################################
//      HOST DEPLOYMENT HANDLING
// ###################################

func deployConfigs(wg *sync.WaitGroup, semaphore chan struct{}, endpointName string, TargetFilePaths map[string]string, endpointSocket string, endpointUser string, ConfigFileData map[string]string, ConfigMetadata map[string]map[string]interface{}, ConfigDataHashes map[string]string, PrivateKey ssh.Signer, SudoPassword string, SHA256RegEx *regexp.Regexp) {
	// Recover from panic
	defer func() {
		if r := recover(); r != nil {
			logError(fmt.Sprintf("Controller panic during deployment to host '%s'", endpointName), fmt.Errorf("%v", r), false)
		}
	}()

	// Signal routine is done after return
	defer wg.Done()

	// Acquire a token from the semaphore channel
	semaphore <- struct{}{}
	defer func() { <-semaphore }() // Release the token when the goroutine finishes

	// SSH Client Connect Conf
	SSHconfig := CreateSSHClientConfig(endpointUser, PrivateKey)

	// Connect to the SSH server
	// fix: retry connect if reason is no route to host
	client, err := ssh.Dial("tcp", endpointSocket, SSHconfig)
	if err != nil {
		hostDeployFailCleanup(endpointName, "", fmt.Errorf("failed connect to SSH server %v", err))
		return
	}
	defer client.Close()

	// Loop through target files and deploy
	backupConfCreated := false
	for targetFilePath, targetFileAction := range TargetFilePaths {
		var command string
		var CommandOutput string

		// If git file was deleted, attempt to delete file any empty folders above - failures here should not stop deployment to this host
		// Note: technically inefficient; if a file is moved within same directory, this will delete the file and parent dir(maybe)
		//                                then when deploying the moved file, it will recreate folder that was just deleted.
		if targetFileAction == "delete" {
			// Attempt remove file and any backup for that file
			command = "rm " + targetFilePath + " " + targetFilePath + ".old"
			CommandOutput, err = RunSSHCommand(client, command, SudoPassword)
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
						fmt.Printf("Warning: Host %s: failed to remove empty parent directory '%s' for file '%s': %v\n", targetPath, targetFilePath, err)
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
		TargetFileOwnerGroup := ConfigMetadata[targetFilePath]["FileOwnerGroup"].(string)
		TargetFilePermissions := ConfigMetadata[targetFilePath]["FilePermissions"].(int)
		ReloadRequired := ConfigMetadata[targetFilePath]["ReloadRequired"].(bool)
		ReloadCommands := ConfigMetadata[targetFilePath]["Reload"].([]string)

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
			if OldRemoteFileHash == ConfigDataHashes[targetFilePath] {
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
		err = TransferFile(client, ConfigFileData[targetFilePath], targetFilePath, SudoPassword)
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
		if NewRemoteFileHash != ConfigDataHashes[targetFilePath] {
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

// ###########################################
//      HOST DEPLOYMENT HANDLING FUNCTIONS
// ###########################################

func restoreOldConfig(client *ssh.Client, targetFilePath string, OldRemoteFileHash string, SHA256RegEx *regexp.Regexp, SudoPassword string, backupConfCreated bool) error {
	var command string
	var CommandOutput string
	var err error
	oldFilePath := targetFilePath + ".old"

	// Check if there is no backup to restore, return early
	if !backupConfCreated {
		return nil
	}

	// Move backup conf into place
	command = "mv " + oldFilePath + " " + targetFilePath
	_, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		return fmt.Errorf("failed SSH Command on host during restoration of old config file: %v", err)
	}

	// Check to make sure restore worked with hash
	command = "sha256sum " + targetFilePath
	CommandOutput, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		return fmt.Errorf("failed SSH Command on host during hash of old config file: %v", err)
	}

	RemoteFileHash := SHA256RegEx.FindString(CommandOutput)

	if OldRemoteFileHash != RemoteFileHash {
		return fmt.Errorf("restored file hash is different than its original hash")
	}

	return nil
}

func CheckRemoteFileExistence(client *ssh.Client, remoteFilePath string, SudoPassword string) (bool, error) {
	command := "ls " + remoteFilePath
	_, err := RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func TransferFile(client *ssh.Client, localFileContent string, remoteFilePath string, SudoPassword string) error {
	var err error
	var command string

	// Check if remote dir exists, if not create
	dir := filepath.Dir(remoteFilePath)
	command = "ls -d " + dir
	_, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			command = "mkdir -p " + dir
			_, err = RunSSHCommand(client, command, SudoPassword)
			if err != nil {
				return fmt.Errorf("failed to create directory: %v", err)
			}
		} else {
			return fmt.Errorf("error checking directory: %v", err)
		}
	}

	// temp file for unpriv sftp writing
	tmpRemoteFilePath := "/tmp/scmpdbuffer"

	// SFTP to temp file
	err = RunSFTP(client, []byte(localFileContent), tmpRemoteFilePath)
	if err != nil {
		return err
	}

	// Move file from tmp dir to actual deployment path
	command = "mv " + tmpRemoteFilePath + " " + remoteFilePath
	_, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		return fmt.Errorf("failed to move new file into place: %v", err)
	}

	return nil
}

// ###########################################
//      SSH/Connection HANDLING
// ###########################################

func SSHIdentityToKey(SSHIdentityFile string, UseSSHAgent bool) (ssh.Signer, error) {
	// Load SSH private key
	// Parse out which is which here and if pub key use as id for agent keychain
	var SSHKeyType string
	var PrivateKey ssh.Signer
	var PublicKey ssh.PublicKey

	// Load identity from file
	SSHIdentity, err := os.ReadFile(SSHIdentityFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read ssh identity file: %v", err)
	}

	// Determine key type
	_, err = ssh.ParsePrivateKey(SSHIdentity)
	if err == nil {
		SSHKeyType = "private"
	}

	_, _, _, _, err = ssh.ParseAuthorizedKey(SSHIdentity)
	if err == nil {
		SSHKeyType = "public"
	}

	// Load key from keyring if requested
	if UseSSHAgent {
		if SSHKeyType != "public" {
			return nil, fmt.Errorf("identity file is not a public key, cannot use agent without public key")
		}

		// Find auth socket for agent
		agentSock := os.Getenv("SSH_AUTH_SOCK")
		if agentSock == "" {
			return nil, fmt.Errorf("cannot use agent, '%s' environment variable is not set", agentSock)
		}

		// Connect to agent socket
		AgentConn, err := net.Dial("unix", agentSock)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to agent: %v", err)
		}

		// New Client to agent socket
		sshAgent := agent.NewClient(AgentConn)

		// Get list of keys in agent
		sshAgentKeys, err := sshAgent.List()
		if err != nil {
			return nil, fmt.Errorf("failed to get list of keys from agent: %v", err)
		}

		// Ensure keys are already loaded
		if len(sshAgentKeys) == 0 {
			return nil, fmt.Errorf("no keys found in agent")
		}

		PublicKey, _, _, _, err = ssh.ParseAuthorizedKey(SSHIdentity)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key from identity file: %v", err)
		}

		// Get signers
		signers, err := sshAgent.Signers()
		if err != nil {
			return nil, fmt.Errorf("failed to get signers from agent: %v", err)
		}

		// Find matching private key to local pub key
		for _, sshAgentKey := range signers {
			// Obtain public key from private key in keyring
			sshAgentPubKey := sshAgentKey.PublicKey()

			// Check against user supplied public key
			if bytes.Equal(sshAgentPubKey.Marshal(), PublicKey.Marshal()) {
				PrivateKey = sshAgentKey
				break
			}
		}
	} else {
		if SSHKeyType != "private" {
			return nil, fmt.Errorf("identity is not private key, you must use agent mode with a public key")
		}

		PrivateKey, err = ssh.ParsePrivateKey(SSHIdentity)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key from identity file: %v", err)
		}
	}

	return PrivateKey, nil
}

func ParseEndpointAddress(endpointIP string, endpointPort int) (string, error) {
	IPv4RegEx := regexp.MustCompile(`^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4}$`)

	// Verify endpoint Port
	if endpointPort <= 0 || endpointPort > 65535 {
		return "", fmt.Errorf("endpoint port number '%d' out of range", endpointPort)
	}

	// Verify IP address
	IPCheck := net.ParseIP(endpointIP)
	if IPCheck == nil && !IPv4RegEx.MatchString(endpointIP) {
		return "", fmt.Errorf("endpoint ip '%s' is not valid", endpointIP)
	}

	// Get endpoint socket by ipv6 or ipv4
	var endpoint string
	if strings.Contains(endpointIP, ":") {
		endpoint = "[" + endpointIP + "]" + ":" + strconv.Itoa(endpointPort)
	} else {
		endpoint = endpointIP + ":" + strconv.Itoa(endpointPort)
	}

	return endpoint, nil
}

func SSHEnvSetup() ssh.HostKeyCallback {
	// Get known_hosts from environment
	knownHostsPath := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	if knownHostsPath == "" {
		logError("Error known_hosts", fmt.Errorf("no path found from environment variable"), true)
	}

	// Read in file
	knownHostFile, err := os.ReadFile(knownHostsPath)
	logError("Error reading known_hosts", err, true)
	// Store as array
	knownhosts := strings.Split(string(knownHostFile), "\n")

	// Function when SSH is connecting during handshake
	hostKeyCallback := createCustomHostKeyCallback(knownHostsPath, knownhosts)
	return hostKeyCallback
}

func CreateSSHClientConfig(endpointUser string, PrivateKey ssh.Signer) *ssh.ClientConfig {
	// Setup host key callback function
	hostKeyCallback := SSHEnvSetup()

	// Setup config for client
	//      Need to only use a single key algorithm type to avoid getting wrong public key back from the server for the local known_hosts check
	//  Supposedly 'fixed' by allowing the client to specify which algo to use when connecting in https://github.com/golang/go/issues/11722
	//  Yeah bud, totally. Let me just create 3 connections per host just to try and find a match in known_hosts... fucking stupid.
	//  Its ed25519 for my env, change it if you want... Beware it must be the same algo used across all of your ssh servers
	SSHconfig := &ssh.ClientConfig{
		User: endpointUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(PrivateKey),
		},
		// Some IPS rules flag on GO's ssh client string
		ClientVersion: "SSH-2.0-OpenSSH_9.8p1",
		// Don't add multiple values here, you will experience handshake errors when verifying some server pub keys
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoED25519,
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	return SSHconfig
}

func RunSFTP(client *ssh.Client, localFileContent []byte, tmpRemoteFilePath string) error {
	// Open new session with ssh client
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create sftp session: %v", err)
	}
	defer sftpClient.Close()

	// Context for SFTP wait - add timeout
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Use default tmp if not provided
	if tmpRemoteFilePath == "" {
		tmpRemoteFilePath = "/tmp/scmpdbuffer"
	}

	// Wait for the file transfer
	errChannel := make(chan error)
	go func() {
		// Open remote file
		remoteTempFile, err := sftpClient.Create(tmpRemoteFilePath)
		if err != nil {
			errChannel <- err
			return
		}

		// Write file contents to remote file
		_, err = remoteTempFile.Write([]byte(localFileContent))
		if err != nil {
			errChannel <- err
			return
		}

		// Signal we are done transferring
		errChannel <- nil
	}()
	// Block until errChannel is done, then parse errors
	select {
	// Transfer finishes before timeout with error
	case err := <-errChannel:
		if err != nil {
			return fmt.Errorf("error with file transfer: %v", err)
		}
	// Timer finishes before transfer
	case <-ctx.Done():
		sftpClient.Close()
		return fmt.Errorf("closed ssh session, file transfer timed out")
	}

	return nil
}

func RunSSHCommand(client *ssh.Client, commandStr string, SudoPassword string) (string, error) {
	// Open new session
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Command output
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdout pipe: %v", err)
	}

	// Command Error
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stderr pipe: %v", err)
	}

	// Command stdin
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdin pipe: %v", err)
	}
	defer stdin.Close()

	// Add sudo to command
	command := "sudo -S " + commandStr

	// Start the command
	err = session.Start(command)
	if err != nil {
		return "", fmt.Errorf("failed to start command: %v", err)
	}

	// Write sudo password to stdin
	_, err = stdin.Write([]byte(SudoPassword))
	if err != nil {
		return "", fmt.Errorf("failed to write to command stdin: %v", err)
	}

	// Close stdin to signal no more writing
	err = stdin.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close stdin: %v", err)
	}

	// Context for command wait - 60 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Wait for the command to finish with timeout
	errChannel := make(chan error)
	go func() {
		errChannel <- session.Wait()
	}()
	// Block until errChannel is done, then parse errors
	select {
	// Command finishes before timeout with error
	case err := <-errChannel:
		if err != nil {
			// Return both exit status and stderr (readall errors are ignored as exit status will still be present)
			CommandError, _ := io.ReadAll(stderr)
			return "", fmt.Errorf("error with command '%s': %v : %s", commandStr, err, CommandError)
		}
	// Timer finishes before command
	case <-ctx.Done():
		session.Signal(ssh.SIGTERM)
		session.Close()
		return "", fmt.Errorf("closed ssh session, command %s timed out", commandStr)
	}

	CommandOutput, err := io.ReadAll(stdout)
	if err != nil {
		return "", fmt.Errorf("error reading from io.Reader: %v", err)
	}

	CommandError, err := io.ReadAll(stderr)
	if err != nil {
		return "", fmt.Errorf("error reading from io.Reader: %v", err)
	}

	// Only return the error if there is one
	if string(CommandError) != "" {
		return string(CommandOutput), fmt.Errorf("%s", CommandError)
	}

	return string(CommandOutput), nil
}

// Custom HostKeyCallback for checking known_hosts
func createCustomHostKeyCallback(knownHostsPath string, knownhosts []string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, PubKey ssh.PublicKey) error {
		// Turn remote address into format used with known_hosts file entries
		cleanHost, _, err := net.SplitHostPort(remote.String())
		if err != nil {
			return fmt.Errorf("error with ssh server key check: unable to determine hostname in address: %v", err)
		}

		// If the remote addr is IPv6, extract the address part (inside brackets)
		if strings.Contains(cleanHost, "]") {
			cleanHost = strings.TrimPrefix(cleanHost, "[")
			cleanHost = strings.TrimSuffix(cleanHost, "]")
		}

		// Convert line proto public key to known_hosts encoding
		remotePubKey := base64.StdEncoding.EncodeToString(PubKey.Marshal())

		// Find an entry that matches the host we are handshaking with
		for _, knownhostkey := range knownhosts {
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

			salt := knownHostsPart[1]
			hashedKnownHost := knownHostsPart[2]
			knownkeysPart := strings.Fields(knownhost[1])

			// Ensure Key section has at least algorithm and key fields
			if len(knownkeysPart) < 2 {
				continue
			}

			//keyAlgorithm := knownkeysPart[0]
			// Hash the cleaned host name with the salt
			saltBytes, err := base64.StdEncoding.DecodeString(salt)
			if err != nil {
				return fmt.Errorf("error decoding salt: %v", err)
			}

			// Create the HMAC-SHA1 using the salt as the key
			h := hmac.New(sha1.New, saltBytes)
			h.Write([]byte(cleanHost))
			hashed := h.Sum(nil)

			// Return the base64 encoded result
			hashedHost := base64.StdEncoding.EncodeToString(hashed)

			// Compare hashed values of host
			if hashedHost == hashedKnownHost {
				// Grab just the key part from known_hosts
				localPubKey := strings.Join(knownkeysPart[1:], " ")
				// Compare pub keys
				if localPubKey == remotePubKey {
					// nil means SSH is cleared to continue handshake
					return nil
				}
			}
		}

		// Ask to add key if not known
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Host %s not in known_hosts. Key: %s %s\nDo you want to add this key to known_hosts? [y/N]: \n", cleanHost, PubKey.Type(), remotePubKey)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if strings.ToLower(input) == "y" {
			//Get Salt and Hash to write to known_hosts
			salt := make([]byte, 20)
			_, err = rand.Read(salt)
			if err != nil {
				fmt.Printf("Error %v\n", err)
			}
			h := hmac.New(sha1.New, salt)
			h.Write([]byte(cleanHost))
			hashed := h.Sum(nil)

			// New line to be added
			newKnownHost := "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" + base64.StdEncoding.EncodeToString(hashed) + " " + PubKey.Type() + " " + remotePubKey

			fmt.Printf("Writing new host entry in known_hosts... ")
			knownHostsfile, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed: %v", err)
			}
			defer knownHostsfile.Close()

			// Write the new known host string followed by a newline
			if _, err := knownHostsfile.WriteString(newKnownHost + "\n"); err != nil {
				return fmt.Errorf("failed: %v", err)
			}
			fmt.Printf("Success")

			// nil means SSH is cleared to continue handshake
			return nil
		}
		return fmt.Errorf("not continuing with connection to %s", cleanHost)
	}
}

// ###################################
//      PARSING FUNCTIONS
// ###################################

func parseGitCommit(commit *object.Commit, TemplateDirectory string, DeployerHosts []string, OSPathSeparator string) (map[string]string, *object.Tree, map[string]string, []string, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get tree: %v", err)
	}

	// Get the parent commit
	parentCommit, err := commit.Parents().Next()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get parent commit: %v", err)
	}

	// Get the diff between the commits
	patch, err := parentCommit.Patch(commit)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get patch between commits: %v", err)
	}

	// New Array for only relevant endpoints for this commit
	var FilteredCommitHostNames []string

	// Main map of commit files
	HostsAndFiles := make(map[string]string)

	// Process changed files in commit
	// Lots of duplication.... sue me
	for _, file := range patch.FilePatches() {
		from, to := file.Files()
		var fromPath, toPath string
		var commitPathsArray []string
		var commitFileInfoFrom os.FileInfo
		var commitFileInfoTo os.FileInfo
		if from != nil {
			fromPath = from.Path()
			// Set path array if from path is present
			if fromPath != "" {
				commitPathsArray = strings.SplitN(fromPath, OSPathSeparator, 2)
			}

			// Get on disk file info if present
			_, err := os.Stat(toPath)
			if os.IsExist(err) {
				// Get file type on disk for filtering parsing actions
				commitFileInfoFrom, err = os.Lstat(fromPath)
				if err != nil {
					return nil, nil, nil, nil, err
				}

				// Skip special file types
				if commitFileInfoFrom.Mode()&os.ModeDevice != 0 {
					// like brw-rw----
					continue
				} else if commitFileInfoFrom.Mode()&os.ModeCharDevice != 0 {
					// like crw-rw----
					continue
				} else if commitFileInfoFrom.Mode()&os.ModeNamedPipe != 0 {
					// like prw-rw----
					continue
				} else if commitFileInfoFrom.Mode()&os.ModeSocket != 0 {
					// like Srw-rw----
					continue
				}
			}
		}
		if to != nil {
			toPath = to.Path()
			// Override path array with to path if present
			if toPath != "" {
				commitPathsArray = strings.SplitN(toPath, OSPathSeparator, 2)
			}

			// Get on disk file info if present
			_, err := os.Stat(toPath)
			if os.IsExist(err) {
				// Get file type on disk for filtering parsing actions
				commitFileInfoTo, err = os.Lstat(toPath)
				if err != nil {
					return nil, nil, nil, nil, err
				}

				// Skip special file types
				if commitFileInfoTo.Mode()&os.ModeDevice != 0 {
					// like brw-rw----
					continue
				} else if commitFileInfoTo.Mode()&os.ModeCharDevice != 0 {
					// like crw-rw----
					continue
				} else if commitFileInfoTo.Mode()&os.ModeNamedPipe != 0 {
					// like prw-rw----
					continue
				} else if commitFileInfoTo.Mode()&os.ModeSocket != 0 {
					// like Srw-rw----
					continue
				}
			}
		}

		// Get the host dir part of the commit path - for checking against main config
		commitHost := commitPathsArray[0]

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
		if commitFileInfoFrom != nil && commitFileInfoFrom.Mode()&os.ModeSymlink != 0 && HostsAndFiles[fromPath] == "create" {
			// Get link target path
			linkTarget, err := filepath.EvalSymlinks(fromPath)
			if err != nil {
				return nil, nil, nil, nil, err
			}

			// Get top directory for sym link and target for compare
			linkTargetPathArray := strings.SplitN(linkTarget, OSPathSeparator, 2)
			fromPathArray := strings.SplitN(fromPath, OSPathSeparator, 2)

			// Error if link is between hosts dirs
			if linkTargetPathArray[0] != fromPathArray[0] {
				return nil, nil, nil, nil, fmt.Errorf("illegal symbolic link, cannot have link between host directories")
			}

			// Add new tag for sym link itself - hard code / because these are target paths
			HostsAndFiles[fromPath] = "symlinkcreate to target " + "/" + linkTargetPathArray[1]
		} else if commitFileInfoTo != nil && commitFileInfoTo.Mode()&os.ModeSymlink != 0 && HostsAndFiles[toPath] == "create" {
			// Get link target path
			linkTarget, err := filepath.EvalSymlinks(toPath)
			if err != nil {
				return nil, nil, nil, nil, err
			}

			// Get top directory for sym link and target for compare
			linkTargetPathArray := strings.SplitN(linkTarget, OSPathSeparator, 2)
			toPathArray := strings.SplitN(toPath, OSPathSeparator, 2)

			// Error if link is between hosts dirs
			if linkTargetPathArray[0] != toPathArray[0] {
				return nil, nil, nil, nil, fmt.Errorf("illegal symbolic link, cannot have link between host directories")
			}

			//Add new tag for sym link itself - hard code / because these are target paths
			HostsAndFiles[toPath] = "symlinkcreate to target " + "/" + linkTargetPathArray[1]
		}

		// Ensure FilteredCommitHostNames are valid hostnames in config.DeployerEndpoints
		var configContainsCommitHost bool
		for _, availableHost := range DeployerHosts {
			if commitHost == availableHost || commitHost == TemplateDirectory {
				configContainsCommitHost = true
				break
			}
		}
		if !configContainsCommitHost {
			return nil, nil, nil, nil, fmt.Errorf("commit host directory '%s' has no matching DeployerEndpoints host in YAML config", commitHost)
		}

		// Add filtered target commit host to array
		FilteredCommitHostNames = append(FilteredCommitHostNames, commitHost)
	}

	// Var for all files in repo - used for dedup later
	AllRepoFiles := make(map[string]string)

	// Iterate over the tree entries.
	err = tree.Files().ForEach(func(f *object.File) error {
		// Always ignore files in root of repository
		if !strings.ContainsRune(f.Name, []rune(OSPathSeparator)[0]) {
			return nil
		}

		// Append the file path to the slice.
		AllRepoFiles[f.Name] = "create"
		return nil
	})
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error iterating over all files: %v", err)
	}

	return HostsAndFiles, tree, AllRepoFiles, FilteredCommitHostNames, nil
}

func removeValueFromMapSlice(HostsAndFilesMap map[string][]string, key, valueToRemove string) {
	if values, ok := HostsAndFilesMap[key]; ok {
		newValues := []string{}
		for _, value := range values {
			if value != valueToRemove {
				newValues = append(newValues, value)
			}
		}
		HostsAndFilesMap[key] = newValues
	}
}

func deDupsHostsandTemplateCommits(HostsAndFiles map[string]string, TemplateDirectory string, AllHostsAndFilesMap map[string][]string, endpointName string, OSPathSeparator string) []string {
	// Filter down committed files to only ones that are allowed for this host and create map for deduping
	HostsAndFilesMap := make(map[string][]string)
	for filePath := range HostsAndFiles {
		// Skip files in root of repository
		if !strings.ContainsRune(filePath, []rune(OSPathSeparator)[0]) {
			continue
		}

		commitSplit := strings.SplitN(filePath, OSPathSeparator, 2)
		commitHost := commitSplit[0]
		commitPath := commitSplit[1]

		if commitHost != endpointName && commitHost != TemplateDirectory {
			continue
		}

		HostsAndFilesMap[commitHost] = append(HostsAndFilesMap[commitHost], commitPath)
	}

	// Map to track duplicates
	confFileCount := make(map[string]int)

	// Count occurences of each conf file in entire repo
	for _, conffiles := range AllHostsAndFilesMap {
		for _, conf := range conffiles {
			confFileCount[conf]++
		}
	}

	// Remove duplicate confs for host in template dir
	for hostdir, conffiles := range AllHostsAndFilesMap {
		for _, conf := range conffiles {
			// Only remove if multiple same config paths AND the hostdir part is the template dir
			if confFileCount[conf] > 1 && hostdir == TemplateDirectory {
				removeValueFromMapSlice(AllHostsAndFilesMap, hostdir, conf)
			}
		}
	}

	// Compare the confs allowed to deploy in the repo with the confs in the actual commit
	hostFiles, hostExists := AllHostsAndFilesMap[endpointName]
	goldenFiles, templateExists := HostsAndFilesMap[TemplateDirectory]

	if hostExists && templateExists {
		// Create a map to track files in the host
		hostFileMap := make(map[string]struct{})
		for _, file := range hostFiles {
			hostFileMap[file] = struct{}{}
		}

		// Filter out files in the golden template that also exist in the host
		var newTemplateFiles []string
		for _, file := range goldenFiles {
			if _, exists := hostFileMap[file]; !exists {
				newTemplateFiles = append(newTemplateFiles, file)
			}
		}

		// Update the HostsAndFilesMap map with the filtered files
		HostsAndFilesMap[TemplateDirectory] = newTemplateFiles
	}

	// Convert Map back to array for loading file contents with git
	var FilteredHostsAndFiles []string
	for host, paths := range HostsAndFilesMap {
		for _, path := range paths {
			FilteredHostsAndFiles = append(FilteredHostsAndFiles, host+OSPathSeparator+path)
		}
	}
	return FilteredHostsAndFiles
}

// Function to extract and validate metadata JSON from file contents
func extractMetadata(fileContents string) (string, string, error) {
	// Define the delimiters
	StartDelimiter := "#|^^^|#"
	EndDelimiter := "#|^^^|#\n" // trims newline from actual file contents
	Delimiter := "#|^^^|#"

	// Find the start and end of the metadata section
	startIndex := strings.Index(fileContents, StartDelimiter)
	if startIndex == -1 {
		return "", "", fmt.Errorf("json start delimter missing")
	}
	startIndex += len(StartDelimiter)

	endIndex := strings.Index(fileContents[startIndex:], EndDelimiter)
	if endIndex == -1 {
		TestEndIndex := strings.Index(fileContents[startIndex:], Delimiter)
		if TestEndIndex == -1 {
			return "", "", fmt.Errorf("no newline after json end delimiter")
		}
		return "", "", fmt.Errorf("json end delimter missing ")
	}
	endIndex += startIndex

	// Extract the metadata section and remaining content into their own vars
	metadataSection := fileContents[startIndex:endIndex]
	remainingContent := fileContents[:startIndex-len(StartDelimiter)] + fileContents[endIndex+len(EndDelimiter):]

	return metadataSection, remainingContent, nil
}

// ###################################
//      UPDATE FUNCTIONS
// ###################################

func simpleLoopHosts(config Config, deployerUpdateFile string, hostOverride string, checkVersion bool) {
	// Load Binary if updating
	var deployerUpdateBinary []byte
	var err error
	if !checkVersion {
		// Load binary from file
		deployerUpdateBinary, err = os.ReadFile(deployerUpdateFile)
		logError("failed loading deployer executable file", err, true)
	}

	// Get SSH Private Key
	PrivateKey, err := SSHIdentityToKey(config.SSHClient.SSHIdentityFile, config.SSHClient.UseSSHAgent)
	logError("Error retrieving SSH private key", err, true)

	for endpointName, endpointInfo := range config.DeployerEndpoints {
		// Allow user override hosts
		var SkipHost bool
		if hostOverride != "" {
			hostChoices := strings.Split(hostOverride, ",")
			for _, host := range hostChoices {
				if host == endpointName {
					break
				}
				SkipHost = true
			}
		}
		if SkipHost {
			continue
		}

		// Extract vars for endpoint information
		endpointIP := endpointInfo[0].Endpoint
		endpointPort := endpointInfo[1].EndpointPort
		endpointUser := endpointInfo[2].EndpointUser

		// Run update
		returnedData, err := DeployerUpdater(deployerUpdateBinary, PrivateKey, config.SSHClient.SudoPassword, checkVersion, endpointUser, endpointIP, endpointPort)
		if err != nil {
			logError(fmt.Sprintf("Error: host '%s'", endpointName), err, true)
			continue
		}

		// If just checking version, Print
		if checkVersion {
			fmt.Printf("%s:%s\n", endpointName, returnedData)
		}
	}
}

func DeployerUpdater(deployerUpdateBinary []byte, PrivateKey ssh.Signer, SudoPassword string, checkVersion bool, endpointUser string, endpointIP string, endpointPort int) (string, error) {
	// Set client configuration
	SSHconfig := CreateSSHClientConfig(endpointUser, PrivateKey)

	// Network info checks
	endpointSocket, err := ParseEndpointAddress(endpointIP, endpointPort)
	if err != nil {
		return "", fmt.Errorf("failed to parse network address: %v", err)
	}

	// Connect to the SSH server
	// TODO: retry connect if reason is no route to host
	client, err := ssh.Dial("tcp", endpointSocket, SSHconfig)
	if err != nil {
		return "", fmt.Errorf("failed connect: %v", err)
	}
	defer client.Close()

	if checkVersion {
		// Get remote host deployer version
		deployerSSHVersion := string(client.Conn.ServerVersion())
		deployerVersion := strings.Replace(deployerSSHVersion, "SSH-2.0-OpenSSH_", "", 1)
		return deployerVersion, nil
	}

	// SFTP to default temp file
	err = RunSFTP(client, deployerUpdateBinary, "")
	if err != nil {
		return "", err
	}

	// Open new session
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Set custom request and payload
	requestType := "update"
	wantReply := true
	reqAccepted, err := session.SendRequest(requestType, wantReply, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create update session: %v", err)
	}
	if !reqAccepted {
		return "", fmt.Errorf("server did not accept request type '%s'", requestType)
	}

	// Command stdin
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdin pipe: %v", err)
	}
	defer stdin.Close()

	// Write sudo password to stdin
	_, err = stdin.Write([]byte(SudoPassword))
	if err != nil {
		return "", fmt.Errorf("failed to write to command stdin: %v", err)
	}

	// Close stdin to signal no more writing
	err = stdin.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close stdin: %v", err)
	}

	return "", nil
}
