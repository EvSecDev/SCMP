// controller
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kevinburke/ssh_config"
	"golang.org/x/term"
)

// Argument Groups

func setGlobalArguments(fs *flag.FlagSet) {
	fs.BoolVar(&config.options.detailedSummaryRequested, "with-summary", false, "Generate JSON summary of actions")
	fs.StringVar(&config.logFilePath, "log-file", "", "Write events to log file (using --verbose level)")
	fs.BoolVar(&config.options.forceEnabled, "force", false, "Do not exit/abort on failures")
	fs.BoolVar(&config.options.allowDeletions, "allow-deletions", false, "Permits deletions of files/entries")
	fs.BoolVar(&config.options.dryRunEnabled, "T", false, "Conducts non-mutating actions (no remote actions)")
	fs.BoolVar(&config.options.dryRunEnabled, "dry-run", false, "Conducts non-mutating actions (no remote actions)")
	fs.BoolVar(&config.options.wetRunEnabled, "w", false, "Conducts non-mutating actions (including remote actions)")
	fs.BoolVar(&config.options.wetRunEnabled, "wet-run", false, "Conducts non-mutating actions (including remote actions)")
	fs.IntVar(&globalVerbosityLevel, "v", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")
	fs.IntVar(&globalVerbosityLevel, "verbosity", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")
}

func setDeployConfArguments(fs *flag.FlagSet) {
	fs.StringVar(&config.filePath, "c", defaultConfigPath, "Path to the configuration file")
	fs.StringVar(&config.filePath, "config", defaultConfigPath, "Path to the configuration file")
}

func setSSHArguments(fs *flag.FlagSet) {
	fs.StringVar(&config.options.runAsUser, "u", "root", "User name to run sudo commands as")
	fs.StringVar(&config.options.runAsUser, "run-as-user", "root", "User name to run sudo commands as")
	fs.BoolVar(&config.options.disableSudo, "disable-privilege-escalation", false, "Disables use of sudo when executing commands remotely")
	fs.IntVar(&config.options.executionTimeout, "execution-timeout", 180, "Timeout in seconds for user-defined commands")
	fs.IntVar(&config.options.maxSSHConcurrency, "m", 10, "Maximum simultaneous SSH connections (1 disables threading)")
	fs.IntVar(&config.options.maxSSHConcurrency, "max-conns", 10, "Maximum simultaneous SSH connections (1 disables threading)")
}

// Print message to stdout
// Message will only print if the global verbosity level is equal to or smaller than requiredVerbosityLevel
// Can directly take variables as values to print just like fmt.Printf
func printMessage(requiredVerbosityLevel int, message string, vars ...interface{}) {
	// No output for verbosity level 0
	if globalVerbosityLevel == 0 {
		return
	}

	// Add timestamps to verbosity levels 2 and up (but only when the timestamp will get printed)
	if globalVerbosityLevel >= 2 && requiredVerbosityLevel <= globalVerbosityLevel {
		currentTime := time.Now()
		timestamp := currentTime.Format("15:04:05.000000")
		message = timestamp + ": " + message
	}

	// Required stdout message verbosity level is equal to or less than global verbosity level
	if requiredVerbosityLevel <= globalVerbosityLevel {
		fmt.Printf(message, vars...)
	}

	// Only use global buffer when log file is present
	if config.logFile != nil && requiredVerbosityLevel <= globalVerbosityLevel {
		// Append message to global log
		config.eventLogMutex.Lock()
		config.eventLog = append(config.eventLog, fmt.Sprintf(message, vars...))
		config.eventLogMutex.Unlock()
	}
}

// Defines all commands/subcommands and their relationships and descriptions
func defineOptions() (cmdOpts map[string]commandSet) {
	cmdOpts = make(map[string]commandSet)

	// Root level
	rootCmd := commandSet{
		description:     "Secure Configuration Management Program (SCMP)",
		fullDescription: "  Deploy configuration files from a git repository to Linux servers via SSH\n  Deploy ad-hoc commands and scripts to Linux servers via SSH",
		parentCommand:   "",
		childCommands:   []string{"deploy", "seed", "file", "header", "exec", "scp", "git", "secrets", "install", "version"},
	}
	cmdOpts["root"] = rootCmd

	// Deployment
	deployCmd := commandSet{
		description:     "Deploy configurations",
		fullDescription: "Takes configuration files from local repository, transfers them to remote servers, and reloads associated services",
		parentCommand:   "root",
		childCommands:   []string{"all", "diff", "failures"},
	}
	cmdOpts["deploy"] = deployCmd

	deployAllCmd := commandSet{
		description:     "Deploy Current Configurations",
		fullDescription: "Deploy configurations from HEAD commit or specified commit regardless of commit difference",
		parentCommand:   "deploy",
		childCommands:   []string{},
	}
	cmdOpts["all"] = deployAllCmd

	deployDiffCmd := commandSet{
		description:     "Deploy Configurations from Commit Diff",
		fullDescription: "Deploy the difference in configurations from the given commit",
		parentCommand:   "deploy",
		childCommands:   []string{},
	}
	cmdOpts["diff"] = deployDiffCmd

	deployFailuresCmd := commandSet{
		description:     "Deploy Configurations from last Complete Deployment Failure",
		fullDescription: "Deploy failed configurations from last total failed deployment using local cached failure file",
		parentCommand:   "deploy",
		childCommands:   []string{},
	}
	cmdOpts["failures"] = deployFailuresCmd

	// Repo Seeding
	seedCmd := commandSet{
		description:     "Download Remote Configurations",
		fullDescription: "Retrieve existing remote configurations and associated metadata and store in local repository",
		parentCommand:   "root",
		childCommands:   []string{},
	}
	cmdOpts["seed"] = seedCmd

	// Local file data handling
	fileCmd := commandSet{
		description:     "Modify Local Data",
		fullDescription: "Manipulate local repository files and their data",
		parentCommand:   "root",
		childCommands:   []string{"new", "replace-data"},
	}
	cmdOpts["file"] = fileCmd

	fileNewCmd := commandSet{
		description:     "Create File with Template Metadata",
		fullDescription: "Makes file at specified path with example metadata and data",
		parentCommand:   "file",
		usageOption:     "<file path>",
		childCommands:   []string{},
	}
	cmdOpts["new"] = fileNewCmd

	fileReplCmd := commandSet{
		description:     "Replace File Data",
		fullDescription: "Replace Chosen File's Data with Given File's Data",
		parentCommand:   "file",
		usageOption:     "<source file> <destination file>",
		childCommands:   []string{},
	}
	cmdOpts["replace-data"] = fileReplCmd

	// Local file metadata handling
	headerCmd := commandSet{
		description:     "Modify File Headers",
		fullDescription: "Manipulate local file JSON metadata headers",
		parentCommand:   "root",
		childCommands:   []string{"edit", "strip", "insert", "read", "verify"},
	}
	cmdOpts["header"] = headerCmd

	headerModCmd := commandSet{
		description:     "Change Metadata Header Values",
		fullDescription: "Modify values in the existing JSON header via direct input JSON or via interactive prompts",
		usageOption:     "<file path>",
		parentCommand:   "header",
		childCommands:   []string{},
	}
	cmdOpts["edit"] = headerModCmd

	headerStripCmd := commandSet{
		description:     "Remove Metadata Header",
		fullDescription: "Deletes the JSON header from the given file",
		usageOption:     "<file path>",
		parentCommand:   "header",
		childCommands:   []string{},
	}
	cmdOpts["strip"] = headerStripCmd

	headerAddCmd := commandSet{
		description:     "Add Metadata Header to Existing File",
		fullDescription: "Use provided JSON to add metadata header to a file that does not have one",
		usageOption:     "<file path>",
		parentCommand:   "header",
		childCommands:   []string{},
	}
	cmdOpts["insert"] = headerAddCmd

	headerReadCmd := commandSet{
		description:     "Print Metadata Header from File",
		fullDescription: "Extract JSON header from file and format",
		usageOption:     "<file path>",
		parentCommand:   "header",
		childCommands:   []string{},
	}
	cmdOpts["read"] = headerReadCmd

	headerVerifyCmd := commandSet{
		description:     "Test Metadata Header Validity",
		fullDescription: "Tests the extraction of file header and the syntax validity of the JSON",
		usageOption:     "<file path>",
		parentCommand:   "header",
		childCommands:   []string{},
	}
	cmdOpts["verify"] = headerVerifyCmd

	// Executions
	execCmd := commandSet{
		description:     "Execute Remote Commands",
		fullDescription: "Execute remote commands and scripts on remote hosts and universal groups",
		parentCommand:   "root",
		usageOption:     "<remote command | file://local-script>",
		childCommands:   []string{},
	}
	cmdOpts["exec"] = execCmd

	// File transfers
	scpCmd := commandSet{
		description:     "Transfer Files",
		fullDescription: "Transfer local files to remote hosts and universal groups",
		parentCommand:   "root",
		usageOption:     "[src host:]<src path> [dst host:]<dst path>",
		childCommands:   []string{},
	}
	cmdOpts["scp"] = scpCmd

	// Repository
	gitCmd := commandSet{
		description:     "Repository Actions",
		fullDescription: "Standard git repository manipulations and support for artifact file tracking",
		parentCommand:   "root",
		childCommands:   []string{"add", "status", "commit"},
	}
	cmdOpts["git"] = gitCmd

	gitAddCmd := commandSet{
		description:     "Add file(s)/dir(s) to the worktree",
		fullDescription: "Add files and/or directories by exact path or glob matches to the working tree",
		usageOption:     "<path|glob>",
		parentCommand:   "git",
		childCommands:   []string{},
	}
	cmdOpts["add"] = gitAddCmd

	gitStatusCmd := commandSet{
		description:     "Show Current Worktree Status",
		fullDescription: "Display status of files and/or directories both in the worktree and not tracked",
		parentCommand:   "git",
		childCommands:   []string{},
	}
	cmdOpts["status"] = gitStatusCmd

	gitCommitCmd := commandSet{
		description:     "Commit Changes to Repository",
		fullDescription: "Commit any tracked changes in the worktree to the repository",
		parentCommand:   "git",
		childCommands:   []string{},
	}
	cmdOpts["commit"] = gitCommitCmd

	// Secrets
	secretsCmd := commandSet{
		description:     "Modify Vault",
		fullDescription: "Add/Modify/Delete entries in the local password vault",
		parentCommand:   "root",
		childCommands:   []string{},
	}
	cmdOpts["secrets"] = secretsCmd

	// Controller installation
	installCmd := commandSet{
		description:     "Initial Setups",
		fullDescription: "Install default configurations for apparmor and SSH and setup new repositories",
		parentCommand:   "root",
		childCommands:   []string{},
	}
	cmdOpts["install"] = installCmd

	// Version Info
	versionCmd := commandSet{
		description:     "Show Version Information",
		fullDescription: "Display meta information about program",
		parentCommand:   "root",
		childCommands:   []string{},
	}
	cmdOpts["version"] = versionCmd

	return
}

const baseIndentSpaces int = 2 // like "[  ]-t, --test  Some usage text"

// Full standardized help menu (wraps option printer as well)
func printHelpMenu(fs *flag.FlagSet, commandname string, allCmdOpts map[string]commandSet) {
	curCmdSet := allCmdOpts[commandname]

	// Usage Overview Line
	usage := os.Args[0]

	// Build usage commands
	func(commandname string) {
		// Temporary slice to collect the commands in reverse order
		var commands []string

		// Traverse the command hierarchy and collect commands
		for commandname != "" && commandname != "root" {
			commands = append(commands, commandname) // Collect commands
			// Get the parent command for the next iteration
			command := allCmdOpts[commandname]
			commandname = command.parentCommand
		}

		// Now reverse the slice and append each command to usage
		for i := len(commands) - 1; i >= 0; i-- {
			usage += " " + commands[i]
		}
	}(commandname)

	if len(curCmdSet.childCommands) == 1 {
		usage += " " + curCmdSet.childCommands[0]
	} else if len(curCmdSet.childCommands) > 1 {
		usage += " [subcommand]"
	}
	usage += " [arguments]..."
	if curCmdSet.usageOption != "" {
		usage += " " + curCmdSet.usageOption
	}

	fmt.Printf("Usage: %s\n\n", usage)

	// Description
	if curCmdSet.parentCommand == "" {
		// Full nice title at top level
		fmt.Println(curCmdSet.description)
		fmt.Println(curCmdSet.fullDescription)
		fmt.Println()
	} else if curCmdSet.fullDescription != "" {
		fmt.Printf("  Description:\n")
		fmt.Printf("    %s\n\n", curCmdSet.fullDescription)
	}

	// Calculate longest subcommand length for offset to description
	maxCmdLen := 0
	for _, subcommand := range curCmdSet.childCommands {
		if len(subcommand) > maxCmdLen {
			maxCmdLen = len(subcommand)
		}
	}

	// Available Sub-commands
	if len(curCmdSet.childCommands) > 0 {
		indent := strings.Repeat(" ", baseIndentSpaces)

		fmt.Printf("%sSubcommands:\n", indent)

		cmdIndent := strings.Repeat(" ", baseIndentSpaces+2)

		// Fixed ordering (in case a map was source of info)
		sort.Strings(curCmdSet.childCommands)

		cmdToDescSpaces := 2
		for _, subcommand := range curCmdSet.childCommands {
			subCmdSet := allCmdOpts[subcommand]
			if subCmdSet.description != "" {
				paddingLen := maxCmdLen - len(subcommand) + cmdToDescSpaces
				padding := strings.Repeat(" ", paddingLen)
				subcommand = subcommand + padding + " - " + subCmdSet.description
			}

			fmt.Printf("%s%s\n", cmdIndent, subcommand)
		}
		fmt.Println()
	}

	// Available Arguments
	printFlagOptions(fs)

	// Trailer at top level
	if curCmdSet.parentCommand == "" {
		fmt.Print(helpMenuTrailer)
	}
}

// Custom printer to deduplicate short/long usages and indent automatically
func printFlagOptions(fs *flag.FlagSet) {
	const shortArgPrefix string = "-"      // like "  [-]t, --test  Some usage text"
	const shortLongArgJoiner string = ", " // like "  -t[, ]--test  Some usage text"
	const longArgPrefix string = "--"      // like "  -t, [--]test  Some usage text"
	const argToUsageSpaces int = 2         // like "  -t, --test[  ]Some usage text"

	type optInfo struct {
		names      []string
		usage      string
		defaultVal string
		hasShort   bool
	}

	seen := make(map[string]*optInfo)

	// Deduplicate usages by exact usage text match
	fs.VisitAll(func(arg *flag.Flag) {
		name := arg.Name
		var shortArgName, longArgName string
		if len(name) == 1 {
			shortArgName = name
		} else {
			longArgName = name
		}

		usageText := arg.Usage

		hasShort := shortArgName != ""

		// Add formatted arg text
		usage, seenUsage := seen[usageText]
		if seenUsage {
			if shortArgName != "" {
				usage.names = append(usage.names, shortArgPrefix+shortArgName)
				usage.hasShort = true
			}
			if longArgName != "" {
				usage.names = append(usage.names, longArgPrefix+longArgName)
			}
		} else {
			names := []string{}
			if shortArgName != "" {
				names = append(names, shortArgPrefix+shortArgName)
			}
			if longArgName != "" {
				names = append(names, longArgPrefix+longArgName)
			}
			seen[usageText] = &optInfo{
				names:      names,
				usage:      arg.Usage,
				defaultVal: arg.DefValue,
				hasShort:   hasShort,
			}
		}
	})

	// Deduplicated option list
	opts := []*optInfo{}
	for _, opt := range seen {
		opts = append(opts, opt)
	}

	// Ensure short args come before long args
	for _, opt := range seen {
		if len(opt.names) <= 1 {
			continue
		}

		sort.Slice(opt.names, func(indexA, indexB int) bool {
			flagNameA := opt.names[indexA]
			flagNameB := opt.names[indexB]

			return len(flagNameA) < len(flagNameB)
		})
	}

	// Sort list to group long/short args
	sort.Slice(opts, func(indexA, indexB int) bool {
		flagA := opts[indexA]
		flagB := opts[indexB]

		firstNameA := strings.ToLower(flagA.names[0])
		firstNameB := strings.ToLower(flagB.names[0])

		return firstNameA < firstNameB
	})

	// accounts for short arg prefix length, short arg default len (1), and joiner length
	longShortArgOffset := len(shortLongArgJoiner) + len(shortArgPrefix) + 1

	// Calculate max length flags for alignment
	maxLen := 0
	for _, opt := range opts {
		left := strings.Join(opt.names, shortLongArgJoiner)
		if !opt.hasShort {
			leftLen := len(left) + longShortArgOffset
			if leftLen > maxLen {
				maxLen = leftLen
			}
		} else {
			if len(left) > maxLen {
				maxLen = len(left)
			}
		}
	}

	// Print option list
	fmt.Printf("%sOptions:\n", strings.Repeat(" ", baseIndentSpaces))
	for _, opt := range opts {
		left := strings.Join(opt.names, shortLongArgJoiner)

		// Indent based on short/long
		indentSpaces := baseIndentSpaces
		if !opt.hasShort {
			indentSpaces += longShortArgOffset
		}
		indent := strings.Repeat(" ", indentSpaces)

		// Padding for this line to offset usage text
		leftLen := len(left) + (0)
		if !opt.hasShort {
			leftLen += longShortArgOffset
		}
		paddingSpaces := maxLen - leftLen + argToUsageSpaces
		if paddingSpaces < argToUsageSpaces {
			paddingSpaces = argToUsageSpaces
		}
		padding := strings.Repeat(" ", paddingSpaces)

		// Skip printing any "empty" defaults
		desc := opt.usage
		if opt.defaultVal != "" && opt.defaultVal != "false" && opt.defaultVal != "0" {
			desc += fmt.Sprintf(" [default: %s]", opt.defaultVal)
		}

		fmt.Printf("%s%s%s%s\n", indent, left, padding, desc)
	}

}

// Parse out options from config file into global
func (config *Config) extractOptions(configFilePath string) (err error) {
	const failTrackerFile string = ".scmp-last-deployment-summary.json" // file name for recording deployment summary details

	// Config agnostic configuration options
	config.osPathSeparator = string(os.PathSeparator)
	config.userHomeDirectory, err = os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("unable to find home directory: %v", err)
		return
	}

	// Load Config File
	config.filePath = expandHomeDirectory(configFilePath)
	sshConfigFile, err := os.ReadFile(config.filePath)
	if err != nil {
		err = fmt.Errorf("reading config failed: %v", err)
		return
	}
	sshConfigContents := string(sshConfigFile)

	// Retrieve SSH Config file options
	sshConfig, err := ssh_config.Decode(strings.NewReader(sshConfigContents))
	if err != nil {
		err = fmt.Errorf("failed decoding config file: %v", err)
		return
	}

	// Set globals - see global section at top for descriptions

	// Open log file
	if config.logFilePath != "" {
		config.logFile, err = os.OpenFile(config.logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
	}

	// Set path to failtracker file (in config directory)
	configDirectory := filepath.Dir(config.filePath)
	config.failTrackerFilePath = filepath.Join(configDirectory, failTrackerFile)

	// Retrieve known_hosts file path
	config.knownHostsFilePath, _ = sshConfig.Get("*", "UserKnownHostsFile")
	if config.knownHostsFilePath == "" {
		err = fmt.Errorf("known_hosts file path must be present")
		return
	}

	// Format known_hosts path correctly
	config.knownHostsFilePath = expandHomeDirectory(config.knownHostsFilePath)

	// Ensure known_hosts file exists, if not create it
	_, err = os.Stat(config.knownHostsFilePath)
	if os.IsNotExist(err) {
		var knownHostsFile *os.File
		knownHostsFile, err = os.Create(config.knownHostsFilePath)
		if err != nil {
			return
		}
		knownHostsFile.Close()
	} else if err != nil {
		return
	}

	// Read in file
	knownHostFile, err := os.ReadFile(config.knownHostsFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read known_hosts file: %v", err)
		return
	}

	// Store all known_hosts as array
	config.knownHosts = strings.Split(string(knownHostFile), "\n")

	// All config dir names in repo
	config.universalDirectory, _ = sshConfig.Get("", "UniversalDirectory")
	if strings.Contains(config.universalDirectory, config.osPathSeparator) {
		err = fmt.Errorf("UniversalDirectory should be a relative path from the root of repository")
		return
	}

	// Ignored Dirs in repo
	ignoreDirectoryNames, _ := sshConfig.Get("", "IgnoreDirectories")
	config.ignoreDirectories = strings.Split(ignoreDirectoryNames, ",")
	if strings.Contains(ignoreDirectoryNames, config.osPathSeparator) {
		err = fmt.Errorf("IgnoreDirectories should be relative paths from the root of repository")
		return
	}

	// If max conns is not set, default to no concurrency
	if config.options.maxSSHConcurrency == 0 {
		config.options.maxSSHConcurrency = 1
	}

	// Password vault file
	vaultRelPath, _ := sshConfig.Get("", "PasswordVault")
	config.vaultFilePath = expandHomeDirectory(vaultRelPath)

	// Initialize vault map
	config.vault = make(map[string]Credential)

	// Array of Hosts and their info
	config.hostInfo = make(map[string]EndpointInfo)
	config.allUniversalGroups = make(map[string][]string)
	var hostInfo EndpointInfo
	for _, host := range sshConfig.Hosts {
		// Skip host patterns with more than one pattern
		if len(host.Patterns) != 1 {
			continue
		}

		// Convert host pattern to string
		hostPattern := host.Patterns[0].String()

		// If a wildcard pattern, skip
		if strings.Contains(hostPattern, "*") {
			continue
		}

		// Save hostname into info map
		hostInfo.endpointName = hostPattern

		// Save user into info map
		hostInfo.endpointUser, _ = sshConfig.Get(hostPattern, "User")

		// First item must be present
		endpointAddr, _ := sshConfig.Get(hostPattern, "Hostname")

		// Get port from endpoint
		endpointPort, _ := sshConfig.Get(hostPattern, "Port")

		// Network Address Parsing - only if address
		if endpointAddr != "" && endpointPort != "" {
			hostInfo.endpoint, err = parseEndpointAddress(endpointAddr, endpointPort)
			if err != nil {
				err = fmt.Errorf("failed parsing network address: %v", err)
				return
			}
		}

		// Get timeout value if present
		connectTimeout, _ := sshConfig.Get(hostPattern, "ConnectTimeout")
		if connectTimeout != "" {
			hostInfo.connectTimeout, err = strconv.Atoi(connectTimeout)
			if err != nil {
				err = fmt.Errorf("failed parsing connect timeout value: %v", err)
				return
			}
		}

		// Get proxy
		hostInfo.proxy, _ = sshConfig.Get(hostPattern, "ProxyJump")

		// Get identity file path
		hostInfo.identityFile, _ = sshConfig.Get(hostPattern, "IdentityFile")
		hostInfo.identityFile = expandHomeDirectory(hostInfo.identityFile)

		// Create list of hosts that would need vault access
		passwordRequired, _ := sshConfig.Get(hostPattern, "PasswordRequired")
		if strings.ToLower(passwordRequired) == "yes" {
			hostInfo.requiresVault = true
		} else {
			hostInfo.requiresVault = false
		}

		// Save deployment state of this host
		hostInfo.deploymentState, _ = sshConfig.Get(hostPattern, "DeploymentState")

		// Get all groups this host is a part of
		universalGroupsCSV, _ := sshConfig.Get(hostPattern, "GroupTags")

		// Get yes/no if host ignores main universal
		ignoreUniversalString, _ := sshConfig.Get(hostPattern, "IgnoreUniversal")

		// Parse config host groups into necessary global/host variables
		hostInfo.ignoreUniversal, hostInfo.universalGroups = filterHostGroups(hostPattern, universalGroupsCSV, ignoreUniversalString)

		// write into config
		config.hostInfo[hostPattern] = hostInfo
	}

	return
}

// Creates two maps relating to host groups
// First map: key'd on group and contains only groups that the host is a part of (values are empty)
// Second map: global key'd on group and contains array of hosts belonging to that group
func filterHostGroups(endpointName string, universalGroupsCSV string, ignoreUniversalString string) (hostIgnoresUniversal bool, hostUniversalGroups map[string]struct{}) {
	// Convert CSV of host groups to array
	universalGroupsList := strings.Split(universalGroupsCSV, ",")

	// If host ignores universal configs
	if strings.ToLower(ignoreUniversalString) == "yes" {
		hostIgnoresUniversal = true
	} else {
		hostIgnoresUniversal = false

		// Not ignoring, make this host a part of the universal group
		universalGroupsList = append(universalGroupsList, config.universalDirectory)
	}

	// Get universal groups this host is a part of
	hostUniversalGroups = make(map[string]struct{})
	for _, universalGroup := range universalGroupsList {
		// Skip empty hosts' group
		if universalGroup == "" {
			continue
		}

		// Map of groups that this host is a part of
		hostUniversalGroups[universalGroup] = struct{}{}

		// Add this hosts name to the global universal map for groups this host is a part of
		config.allUniversalGroups[universalGroup] = append(config.allUniversalGroups[universalGroup], endpointName)
	}

	return
}

// Ensures variables that contains paths do not have '~/' and is replaced with absolute path
func expandHomeDirectory(path string) (absolutePath string) {
	path = strings.Trim(path, `"`)
	path = strings.Trim(path, `'`)

	// Return early if path doesn't have '~/' prefix
	if !strings.HasPrefix(path, "~/") {
		absolutePath = path
		return
	}

	// Remove '~/' prefixes
	path = strings.TrimPrefix(path, "~/")

	// Combine Users home directory path with the input path
	absolutePath = filepath.Join(config.userHomeDirectory, path)
	return
}

// Prompts user to enter something
func promptUser(userPrompt string, printVars ...interface{}) (userResponse string, err error) {
	// Throw error if not in terminal - stdin not available outside terminal for users
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	fmt.Printf(userPrompt, printVars...)
	fmt.Scanln(&userResponse)
	userResponse = strings.ToLower(userResponse)
	return
}

// Prompts user for a secret value (does not echo back entered text)
func promptUserForSecret(userPrompt string, printVars ...interface{}) (userResponse []byte, err error) {
	fd := int(os.Stdin.Fd())

	// Throw error if not in terminal - stdin not available outside terminal for users
	if !term.IsTerminal(fd) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	// Save old terminal state
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		err = fmt.Errorf("failed to set terminal raw mode: %v", err)
		return
	}
	defer func() {
		// Restore terminal state upon program exit
		_ = term.Restore(fd, oldState)
		fmt.Println()
	}()

	// Catch signals to ensure cleanup occurs prior to exit
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		_ = term.Restore(fd, oldState)
		fmt.Println()
		os.Exit(1)
	}()

	// Print prompt
	fmt.Printf(userPrompt, printVars...)

	// Read secret input from user
	userResponse, err = term.ReadPassword(fd)
	if err != nil {
		err = fmt.Errorf("error reading password: %v", err)
		return
	}

	return
}
