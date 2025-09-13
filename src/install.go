// controller
package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Read in installation static files at compile time
//
//go:embed static-files/configurations/apparmor-profile.config
//go:embed static-files/configurations/default-ssh.config
//go:embed static-files/configurations/autocomplete.sh
var installationConfigs embed.FS

func entryInstall(commandname string, args []string) {
	var installAAProf bool
	var installDefaultConfig bool
	var installBashAutoComplete bool
	var newRepoBranch string
	var newRepoPath string

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	commandFlags.StringVar(&newRepoPath, "repository-path", "", "Path to repository")
	commandFlags.StringVar(&newRepoBranch, "repository-branch-name", "main", "Initial branch new for new repository")
	commandFlags.BoolVar(&installDefaultConfig, "default-config", false, "Write default SSH configuration file")
	commandFlags.BoolVar(&installBashAutoComplete, "bash-autocomplete", false, "Setup BASH autocompletion function")
	commandFlags.BoolVar(&installAAProf, "apparmor-profile", false, "Enable apparmor profile if supported")
	setGlobalArguments(commandFlags)

	commandFlags.Usage = func() {
		printHelpMenu(commandFlags, commandname, allCmdOpts)
	}
	if len(args) < 1 {
		printHelpMenu(commandFlags, commandname, allCmdOpts)
		os.Exit(1)
	}
	commandFlags.Parse(args[0:])

	if installAAProf {
		installAAProfile(newRepoPath)
	} else if installDefaultConfig {
		installDefaultSSHConfig()
	} else if installBashAutoComplete {
		installBashAutocomplete()
	} else if newRepoPath != "" {
		createNewRepository(newRepoPath, newRepoBranch)
	} else {
		printHelpMenu(commandFlags, commandname, allCmdOpts)
		os.Exit(1)
	}
}

// Sets up new git repository based on controller-expected directory format
// Also creates initial commit so the first deployment will have something to compare against
func createNewRepository(repoPath string, initialBranchName string) {
	const autoCommitUserName string = "SCMPController"
	const autoCommitUserEmail string = "scmpc@localhost"
	config.osPathSeparator = string(os.PathSeparator)

	// Only take absolute paths from user choice
	absoluteRepoPath, err := filepath.Abs(repoPath)
	logError("Failed to get absolute path to new repository", err, false)

	printMessage(verbosityProgress, "Creating new repository at %s\n", absoluteRepoPath)

	// Get individual dir names
	pathDirs := strings.Split(absoluteRepoPath, config.osPathSeparator)

	// Error if it already exists
	_, err = os.Stat(absoluteRepoPath)
	if !os.IsNotExist(err) {
		logError("Failed to create new repository", fmt.Errorf("directory '%s' already exists", absoluteRepoPath), false)
	}

	// Create repository directories if missing
	repoPath = ""
	for _, pathDir := range pathDirs {
		// Skip empty
		if pathDir == "" {
			continue
		}

		// Save current dir to main path
		repoPath = repoPath + config.osPathSeparator + pathDir

		// Check existence
		_, err := os.Stat(repoPath)
		if os.IsNotExist(err) {
			// Create if not exist
			err := os.Mkdir(repoPath, 0750)
			logError("Failed to create missing directory in repository path", err, false)
		}

		// Go to next dir in array
		pathDirs = pathDirs[:len(pathDirs)-1]
	}

	// Move into new repo directory
	err = os.Chdir(repoPath)
	logError("Failed to change into new repository directory", err, false)

	printMessage(verbosityProgress, "Setting initial branch name to %s\n", initialBranchName)

	// Format branch name
	if initialBranchName != "refs/heads/"+initialBranchName {
		initialBranchName = "refs/heads/" + initialBranchName
	}

	// Set initial branch
	initialBranch := plumbing.ReferenceName(initialBranchName)
	initOptions := &git.InitOptions{
		DefaultBranch: initialBranch,
	}

	// Set git initial options
	plainInitOptions := &git.PlainInitOptions{
		InitOptions: *initOptions,
		Bare:        false,
	}

	printMessage(verbosityProgress, "Initializing git repository\n")

	// Create git repo
	repo, err := git.PlainInitWithOptions(repoPath, plainInitOptions)
	logError("Failed to init git repository", err, false)

	// Read existing config options
	gitConfigPath := repoPath + "/.git/config"
	gitConfigFileBytes, err := os.ReadFile(gitConfigPath)
	logError("Failed to read git config file", err, false)

	printMessage(verbosityProgress, "Setting initial git repository configuration options\n")

	// Write options to config file if no garbage collection section
	if !strings.Contains(string(gitConfigFileBytes), "[gc]") {
		// Open git config file - APPEND
		gitConfigFile, err := os.OpenFile(gitConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		logError("Failed to open git config file", err, false)
		defer gitConfigFile.Close()

		// Define garbage collection section and options
		repoGCOptions := `[gc]
        auto = 10
        reflogExpire = 8.days
        reflogExpireUnreachable = 8.days
        pruneExpire = 16.days
`

		// Write (append) string
		_, err = gitConfigFile.WriteString(repoGCOptions + "\n")
		logError("Failed to write git garbage collection options", err, false)
		gitConfigFile.Close()
	}

	printMessage(verbosityProgress, "Adding example config metadata header files\n")

	// Create a working tree
	worktree, err := repo.Worktree()
	logError("Failed to create new git tree", err, false)

	// Example file
	const exampleFile string = ".example-metadata-header.txt"
	writeTemplateFile(exampleFile, true)

	// Stage the universal files
	_, err = worktree.Add(exampleFile)
	logError("Failed to add universal file", err, false)

	printMessage(verbosityProgress, "Creating an initial commit in repository\n")

	// Create initial commit
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  autoCommitUserName,
			Email: autoCommitUserEmail,
		},
	})
	logError("Failed to create first commit", err, false)

	printMessage(verbosityStandard, "Successfully created new git repository in %s\n", repoPath)
}

func installBashAutocomplete() {
	const sysAutocompleteDir string = "/usr/share/bash-completion/completions"
	autoCompleteFunc, err := installationConfigs.ReadFile("static-files/configurations/autocomplete.sh")
	if err != nil {
		printMessage(verbosityStandard, "Unable to retrieve autocomplete file from embedded filesystem: %v\n", err)
		return
	}

	executablePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		printMessage(verbosityStandard, "Failed to retrieve absolute executable path for profile installation: %v\n", err)
		return
	}
	executableName := filepath.Base(executablePath)

	// Inject actual executable name into completion script
	autoCompletion := strings.Replace(string(autoCompleteFunc), "_controller()", "_"+executableName+"()", 1)
	autoCompletion = strings.Replace(autoCompletion, "complete -F _controller controller", "complete -F _"+executableName+" "+executableName, 1)
	autoCompleteFunc = []byte(autoCompletion)

	// Write to system, or fallback to users home
	var autoCompleteFilePath string
	_, err = os.Stat(sysAutocompleteDir)
	if err == nil {
		autoCompleteFilePath = filepath.Join(sysAutocompleteDir, executableName)
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			printMessage(verbosityStandard, "Failed to find user home directory: %v\n", err)
			return
		}
		userDir := filepath.Join(homeDir, ".bash_completion.d")
		err = os.MkdirAll(userDir, 0750)
		if err != nil {
			printMessage(verbosityStandard, "Failed to create user autocomplete dir: %v\n", err)
			return
		}
		err = nil

		autoCompleteFilePath = filepath.Join(userDir, executableName)
		printMessage(verbosityStandard, "System completion dir missing, installing bash completion at %s\n", autoCompleteFilePath)
		printMessage(verbosityStandard, "Make sure ~/.bashrc sources ~/.bash_completion and ~/.bash_completion.d/*\n")
	}

	err = os.WriteFile(autoCompleteFilePath, autoCompleteFunc, 0644)
	if err != nil {
		printMessage(verbosityStandard, "Failed to write autocompletion file: %v\n", err)
		return
	}
}

// Install sample SSH config if it doesn't already exist
func installDefaultSSHConfig() {
	configPath := expandHomeDirectory(defaultConfigPath)
	defaultConfig, err := installationConfigs.ReadFile("static-files/configurations/default-ssh-config")
	if err != nil {
		printMessage(verbosityStandard, "Unable to retrieve configuration file from embedded filesystem: %v\n", err)
		return
	}

	// Check if config already exists
	_, err = os.Stat(configPath)
	if !os.IsNotExist(err) {
		printMessage(verbosityStandard, "SSH Config file already exists, not overwriting it. Please configure manually.\n")
		return
	} else if os.IsNotExist(err) {
		err = nil // create the file
	} else if err != nil {
		printMessage(verbosityStandard, "Unable to check if SSH config file already exists: %v\n", err)
		return
	}

	err = os.WriteFile(configPath, defaultConfig, 0640)
	if err != nil {
		printMessage(verbosityStandard, "Failed to write sample SSH config: %v\n", err)
		return
	}

	printMessage(verbosityStandard, "Successfully created new example config in %s\n", configPath)
}

// If apparmor LSM is available on this system and running as root, auto install the profile - failures are not printed under normal verbosity
func installAAProfile(repositoryPath string) {
	if repositoryPath == "" {
		printMessage(verbosityStandard, "Unable to install apparmor profile: missing repository-path\n")
		return
	}

	const appArmorProfilePath string = "/etc/apparmor.d/scmp-controller"
	appArmorProfile, err := installationConfigs.ReadFile("static-files/configurations/apparmor-profile")
	if err != nil {
		printMessage(verbosityStandard, "Unable to retrieve configuration file from embedded filesystem: %v\n", err)
		return
	}

	executablePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		printMessage(verbosityStandard, "Failed to retrieve absolute executable path for profile installation: %v\n", err)
		return
	}

	// Inject variables into config
	newaaProf := strings.Replace(string(appArmorProfile), "=$executablePath", "="+executablePath, 1)
	newaaProf = strings.Replace(newaaProf, "=$repositoryPath", "="+repositoryPath, 1)
	newaaProf = strings.Replace(newaaProf, "=$aaProfPath", "="+appArmorProfilePath, 1)
	appArmorProfile = []byte(newaaProf)

	// Can't install apparmor profile without root/sudo
	if os.Geteuid() > 0 {
		printMessage(verbosityStandard, "Need root permissions to install apparmor profile\n")
		return
	}

	// Check if apparmor /sys path exists
	systemAAPath := "/sys/kernel/security/apparmor/profiles"
	_, err = os.Stat(systemAAPath)
	if os.IsNotExist(err) {
		printMessage(verbosityStandard, "AppArmor not supported by this system\n")
		return
	} else if err != nil {
		printMessage(verbosityStandard, "Unable to check if AppArmor is supported by this system: %v\n", err)
		return
	}

	// Write Apparmor Profile to /etc
	err = os.WriteFile(appArmorProfilePath, appArmorProfile, 0644)
	if err != nil {
		printMessage(verbosityStandard, "Failed to write apparmor profile: %v\n", err)
		return
	}

	// Enact Profile
	command := exec.Command("apparmor_parser", "-r", appArmorProfilePath)
	_, err = command.CombinedOutput()
	if err != nil {
		printMessage(verbosityStandard, "Failed to reload apparmor profile: %v\n", err)
		return
	}

	printMessage(verbosityStandard, "Successfully installed AppArmor Profile\n")
}
