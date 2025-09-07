// controller
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type DeploymentList struct {
	files             []string            // Ordered list of files to deploy together
	reloadIDtoFile    map[string][]string // Lookup of file list by reload ID
	fileToReloadID    map[string]string   // Lookup of a files reload ID
	reloadIDfileCount map[string]int      // Total files in reload group
	reloadIDcommands  map[string][]string // Ordered list of reload commands
}

// Struct for metadata json in config files
type MetaHeader struct {
	TargetFileOwnerGroup    string   `json:"FileOwnerGroup"`
	TargetFilePermissions   int      `json:"FilePermissions"`
	ExternalContentLocation string   `json:"ExternalContentLocation,omitempty"`
	SymbolicLinkTarget      string   `json:"SymbolicLinkTarget,omitempty"`
	Dependencies            []string `json:"Dependencies,omitempty"`
	PreDeployCommands       []string `json:"PreDeploy,omitempty"`
	InstallCommands         []string `json:"Install,omitempty"`
	CheckCommands           []string `json:"Checks,omitempty"`
	ReloadCommands          []string `json:"Reload,omitempty"`
	ReloadGroup             string   `json:"ReloadGroup,omitempty"`
}

// Struct for deployment file metadata
type FileInfo struct {
	hash              string // Pointer (key) to file data map (for deduplication)
	targetFilePath    string // Expected remote file path
	action            string
	ownerGroup        string
	permissions       int
	fileSize          int
	linkTarget        string
	dependencies      []string // List of files required by this file
	predeployRequired bool
	predeploy         []string // Command list
	installOptional   bool
	install           []string // Command list
	checksRequired    bool
	checks            []string // Command list
	reloadRequired    bool
	reload            []string // Command list
	reloadGroup       string   // Named string defined by user to manually group files together
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

func entryDeploy(commandname string, args []string) {
	availCommands := map[string]func(string, string, string, string){
		"all":      deploy,
		"diff":     deploy,
		"failures": deploy,
	}
	var commandList []string
	for cmd := range availCommands {
		commandList = append(commandList, cmd)
	}

	var commitID string
	var hostOverride string
	var localFileOverride string
	var testConfig bool

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	setDeployConfArguments(commandFlags)
	commandFlags.StringVar(&hostOverride, "r", "", "Override hosts for deployment")
	commandFlags.StringVar(&hostOverride, "remote-hosts", "", "Override hosts for deployment")
	commandFlags.StringVar(&localFileOverride, "l", "", "Override file(s) for deployment")
	commandFlags.StringVar(&localFileOverride, "local-files", "", "Override file(s) for deployment")
	commandFlags.StringVar(&commitID, "C", "", "Commit ID (hash) to deploy from")
	commandFlags.StringVar(&commitID, "commitid", "", "Commit ID (hash) to deploy from")
	commandFlags.IntVar(&config.options.maxDeployConcurrency, "M", 5, "Maximum simultaneous file deployments per host (1 disables threading)")
	commandFlags.IntVar(&config.options.maxDeployConcurrency, "max-deploy-threads", 5, "Maximum simultaneous file deployments per host (1 disables threading)")
	commandFlags.BoolVar(&config.options.runInstallCommands, "install", false, "Run installation commands during deployment")
	commandFlags.BoolVar(&config.options.disableReloads, "disable-reloads", false, "Disables running any reload commands")
	commandFlags.BoolVar(&config.options.ignoreDeploymentState, "ignore-deployment-state", false, "Ignores deployment state in configuration file")
	commandFlags.BoolVar(&testConfig, "t", false, "Test configuration syntax and option validity")
	commandFlags.BoolVar(&testConfig, "test-config", false, "Test configuration syntax and option validity")
	commandFlags.BoolVar(&config.options.regexEnabled, "regex", false, "Enables regular expression parsing for file/host overrides")
	setGlobalArguments(commandFlags)
	setSSHArguments(commandFlags)

	commandFlags.Usage = func() {
		printHelpMenu(commandFlags, commandname, commandList, "", false)
	}
	if len(args) < 1 {
		printHelpMenu(commandFlags, commandname, commandList, "", false)
		os.Exit(1)
	}
	commandFlags.Parse(args[1:])

	err := config.extractOptions(config.filePath)
	logError("Error in controller configuration", err, true)

	if testConfig {
		printMessage(verbosityStandard, "controller: configuration file %s test is successful\n", config.filePath)
		return
	}

	subcommand := args[0]

	entryFunc, validCommand := availCommands[subcommand]
	if validCommand {
		entryFunc(subcommand, commitID, hostOverride, localFileOverride)
	} else {
		printHelpMenu(commandFlags, commandname, commandList, "", false)
		os.Exit(1)
	}
}

// Parses and prepares deployment information
func deploy(deployMode string, commitID string, hostOverride string, fileOverride string) {
	// Pull contents of out file URIs
	hostOverride, err := retrieveURIFile(hostOverride)
	logError("Failed to parse remove-hosts URI", err, true)
	fileOverride, err = retrieveURIFile(fileOverride)
	logError("Failed to parse local-files URI", err, true)

	err = retrieveGitRepoPath()
	logError("Repository Error", err, false)

	// Override commitID with one from failtracker if redeploy requested
	var lastDeploymentSummary DeploymentSummary
	if deployMode == "failures" {
		commitID, lastDeploymentSummary, err = getFailTrackerCommit()
		logError("Failed to extract commitID/failures from failtracker file", err, false)
	}

	// Open repo and get details - using HEAD commit if commitID is empty
	// Pass by reference to ensure commitID can be used later if user did not specify one
	tree, commit, err := getCommit(&commitID)
	logError("Error retrieving commit details", err, true)

	var commitFiles map[string]string

	switch deployMode {
	case "diff":
		changedFiles, lerr := getChangedFiles(commit)
		logError("Failed to retrieve changed files", lerr, true)

		commitFiles = parseChangedFiles(changedFiles, fileOverride)
	case "all":
		commitFiles, err = getRepoFiles(tree, fileOverride)
	case "failures":
		commitFiles, hostOverride, err = lastDeploymentSummary.getFailures(fileOverride)
	default:
		logError("Unknown deployment mode", fmt.Errorf("mode must be diff, all, or failures"), false)
	}

	logError("Failed to retrieve files", err, false)

	if len(commitFiles) == 0 {
		// Non-error - can happen under normal operations: When committing files outside of host directories
		printMessage(verbosityStandard, "No files available for deployment.\n")
		return
	}

	allHostsFiles, universalFiles, err := parseAllRepoFiles(tree)
	logError("Failed to track files by host/universal directory", err, true)

	deniedUniversalFiles := mapDeniedUniversalFiles(allHostsFiles, universalFiles)

	allDeploymentHosts, allDeploymentFiles, hostDeploymentFiles := filterHostsAndFiles(deniedUniversalFiles, commitFiles, hostOverride)
	if len(allDeploymentFiles) == 0 || len(allDeploymentHosts) == 0 {
		// Non-error - can happen under normal operations: if user specifies change deploy mode with a host that didn't have any changes in the specified commit
		printMessage(verbosityStandard, "No deployment files for available hosts.\n")
		return
	}

	rawFileContent, err := loadGitFileContent(allDeploymentFiles, tree)
	logError("Error loading files", err, true)

	allFileMeta, allFileData, err := parseFileContent(allDeploymentFiles, rawFileContent)
	logError("Error parsing loaded files", err, true)

	config.hostInfo, err = sortFiles(config.hostInfo, hostDeploymentFiles, allFileMeta)
	logError("Failed sorting deployment files", err, true)

	err = localSystemChecks()
	logError("Error in local system checks", err, true)

	printMessage(verbosityStandard, "Deploying %d item(s) to %d host(s)\n", len(allFileMeta), len(allDeploymentHosts))

	if config.options.dryRunEnabled {
		printDeploymentInformation(allFileMeta, allDeploymentHosts)
		return
	}

	// Retrieve keys and passwords for any hosts that require it
	for _, endpointName := range allDeploymentHosts {
		// Retrieve host secrets
		config.hostInfo[endpointName], err = retrieveHostSecrets(config.hostInfo[endpointName])
		logError("Error retrieving host secrets", err, true)

		// Retrieve proxy secrets (if proxy is needed)
		proxyName := config.hostInfo[endpointName].proxy
		if proxyName != "" {
			config.hostInfo[proxyName], err = retrieveHostSecrets(config.hostInfo[proxyName])
			logError("Error retrieving proxy secrets", err, true)
		}
	}

	// Metric collection
	deployMetrics := &DeploymentMetrics{}
	deployMetrics.hostFiles = make(map[string][]string)
	deployMetrics.hostBytes = make(map[string]int)
	deployMetrics.fileErr = make(map[string]string)
	deployMetrics.hostErr = make(map[string]string)
	deployMetrics.fileAction = make(map[string]string)
	deployMetrics.startTime = time.Now().UnixMilli()

	// Start SSH Deployments
	// All failures and errors from here on are soft stops - program will finish, errors are tracked within deployment metrics, git commit will NOT be rolled back
	var wg sync.WaitGroup
	connLimiter := make(chan struct{}, config.options.maxSSHConcurrency)
	for _, endpointName := range allDeploymentHosts {
		hostInfo := config.hostInfo[endpointName]
		proxyInfo := config.hostInfo[config.hostInfo[endpointName].proxy]

		wg.Add(1)
		if config.options.maxSSHConcurrency > 1 {
			go sshDeploy(&wg, connLimiter, hostInfo, proxyInfo, allFileMeta, allFileData, deployMetrics)
		} else {
			// Max conns of 1 disables using go routine
			sshDeploy(&wg, connLimiter, hostInfo, proxyInfo, allFileMeta, allFileData, deployMetrics)

			// Don't continue to the next host on errors
			if len(deployMetrics.fileErr) > 0 {
				break
			}
		}
	}
	wg.Wait()

	deployMetrics.endTime = time.Now().UnixMilli()
	deploymentSummary := deployMetrics.createReport(commitID)

	if config.options.wetRunEnabled {
		printMessage(verbosityStandard, "Wet-run enabled. No mutating actions taken, theoretical deployment summary:\n")
	}

	// Show user what was done during deployment
	if config.options.detailedSummaryRequested {
		// Detailed Summary
		deploymentSummaryJson, err := json.MarshalIndent(deploymentSummary, "", " ")
		logError("Failed to marshal detailed deployment summary JSON", err, false)

		printMessage(verbosityStandard, "%s\n", string(deploymentSummaryJson))
	} else {
		printMessage(verbosityStandard,
			"Status: %s. Deployed %d item(s) (%s) to %d host(s). Deployment took %s\n",
			deploymentSummary.Status,
			deploymentSummary.Counters.CompletedItems,
			deploymentSummary.TransferredData,
			deploymentSummary.Counters.CompletedHosts,
			deploymentSummary.ElapsedTime,
		)

		err = deploymentSummary.printFailures()
		logError("Error in printing deployment failures", err, false)
	}

	err = deploymentSummary.saveReport()
	logError("Error in recording deployment failures", err, false)

	if len(deployMetrics.fileErr) > 0 {
		// Remove fail tracker file after successful redeployment - best effort
		err = os.Remove(config.failTrackerFilePath)
		if err != nil {
			if os.IsNotExist(err) {
				// No warning if the file doesn't exist
			} else {
				logError("Failed removing failtracker file", err, false)
			}
		}
	}
}
