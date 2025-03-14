// controller
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Sets up new git repository based on controller-expected directory format
// Also creates intial commit so the first deployment will have something to compare against
func createNewRepository(newRepoInfo string) {
	// Split user choices
	userRepoChoices := strings.Split(newRepoInfo, ":")

	// Default repo values
	repoPath := expandHomeDirectory("~/SCMPGit")
	initialBranchName := "main" // Default to "main"

	// Set user choice if provided
	switch len(userRepoChoices) {
	case 0:
		// Both repoPath and initialBranchName are set to defaults
	case 1:
		// User specified only the repo path
		repoPath = userRepoChoices[0]
		// Default branch name is already set to "main"
	case 2:
		// User specified both repo path and branch name
		repoPath = userRepoChoices[0]
		initialBranchName = userRepoChoices[1]
	default:
		logError("Invalid Argument", fmt.Errorf("invalid new repository option length"), false)
	}

	// Local os separator char
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

	// Example files
	exampleFiles := []string{".example-metadata-header.txt", ".example-metadata-header-noreload.txt"}

	// Create and add example files to repository
	for _, exampleFile := range exampleFiles {
		var metadataHeader MetaHeader

		// Populate metadata JSON with examples
		metadataHeader.TargetFileOwnerGroup = "root:root"
		metadataHeader.TargetFilePermissions = 640

		// Add reloads/checks or dont depending on example file name
		if !strings.Contains(exampleFile, "noreload") {
			metadataHeader.ReloadCommands = []string{"ls /var/log/custom.log", "ping -W2 -c1 syslog.example.com >/dev/null"}
			metadataHeader.CheckCommands = []string{"systemctl restart rsyslog.service", "systemctl is-active rsyslog"}
		}

		// Create example metadata header files
		metadata, err := json.MarshalIndent(metadataHeader, "", "  ")
		logError("Failed to marshal example metadata JSON", err, false)

		// Add full header to string
		exampleHeader := metaDelimiter + "\n" + string(metadata) + "\n" + metaDelimiter + "\n"

		// Write example file to repo
		err = os.WriteFile(exampleFile, []byte(exampleHeader), 0640)
		logError("Failed to write example metadata file", err, false)

		// Stage the universal files
		_, err = worktree.Add(exampleFile)
		logError("Failed to add universal file", err, false)
	}

	printMessage(verbosityProgress, "Creating an initial commit in repository\n")

	// Create initial commit
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  autoCommitUserName,
			Email: autoCommitUserEmail,
		},
	})
	logError("Failed to create first commit", err, false)
}

func installDefaultSSHConfig() {
	configPath := expandHomeDirectory(defaultConfigPath)
	const defaultConfig string = `
##########################
# Global Config Settings #
##########################
#  Ignore SCMP Host Configuration Options
IgnoreUnknown           PasswordVault,PasswordRequired,DeploymentState,IgnoreTemplates,RemoteBackupDir,RemoteTransferBuffer,UniversalDirectory,GroupDirs,GroupTags,IgnoreDirectories
#  Store any login/sudo passwords in an encrypted file here
PasswordVault           ~/.ssh/scmpc.vault
#  Directory Name that contains files relevant to all hosts
UniversalDirectory      "UniversalConfs"
#  Group Directory Names for Universal Configs to be deployed to all hosts tagged with that group
#GroupDirs              UniversalConfs_NGINX,UniversalConfs_MONAGENT
#  Directory Names to ignore in deployment git repository
IgnoreDirectories       Templates,Extras
#
################# EXAMPLE HOSTS CONFIGURATION
#
#Host Web01
#        Hostname       192.168.10.2
#       GroupTags       UniversalConfs_NGINX,UniversalConfs_MONAGENT
#       DeploymentState offline
#Host Proxy01
#       Hostname        192.168.10.3
#       GroupTags       UniversalConfs_MONAGENT
#Host DNS01
#        Hostname       ns1.domain.com
#Host PBX
#        Hostname       192.168.10.4
#       User            root
#       DeploymentState offline
#Host WWW
#        Hostname       192.168.20.15
#       Port            2222
#       GroupTags       UniversalConfs_MONAGENT
#Host Mail
#        Hostname       mx01.domain.com
#       IdentityFile    ~/.ssh/appservers.key
#Host Squid
#        Hostname       192.168.20.22
#       RemoteBackupDir         /var/tmp/.scmpbackups
#       RemoteTransferBuffer    /var/tmp/.scmpbuffer
#Host DB01
#        Hostname       psql01.domain.com
#       Port            2202
#       StrictHostKeyChecking no
#       DeploymentState offline
#Host SSO
#        Hostname       sso.domain.com
#       PasswordRequired yes
#       IgnoreUniversal yes
##########################
# Global Device Settings #
##########################
Host * 
        # Host Settings
        User                            deployer
        Port                            22
        PreferredAuthentications        password,keyboard-interactive,publickey
        IdentityFile                    "~/.ssh/example.key"
        IdentitiesOnly                  yes
        #  General Settings
        UserKnownHostsFile              ~/.ssh/known_hosts
        StrictHostKeyChecking           ask
        ForwardX11                      no
        ForwardX11Trusted               no
        Tunnel                          no
        ForwardAgent                    no
        GSSAPIAuthentication            no
        HostbasedAuthentication         no
        #  SCMP Global Settings
        RemoteBackupDir                 /tmp/.scmpbackups
        RemoteTransferBuffer            /tmp/.scmpbuffer
	`

	// Check if config already exists
	_, err := os.Stat(configPath)
	if !os.IsNotExist(err) {
		printMessage(verbosityProgress, "SSH Config file already exists, not overwritting it. Please configure manually.\n")
		return
	} else if err != nil {
		printMessage(verbosityProgress, "Unable to check if SSH config file already exists: %v\n", err)
		return
	}

	// Write example config to default location
	err = os.WriteFile(configPath, []byte(defaultConfig), 0640)
	if err != nil {
		printMessage(verbosityProgress, "Failed to write sample SSH config: %v\n", err)
		return
	}
}

// If apparmor LSM is available on this system and running as root, auto install the profile
func installAAProfile() {
	const AppArmorProfilePath string = "/etc/apparmor.d/scmpcontroller"
	const AppArmorProfile = `### Apparmor Profile for the Secure Configuration Management Controller
## This is a very locked down profile made for Debian systems
## Variables - add to if required
@{exelocation}=$executablePath
@{repolocation}=$RepositoryPath
@{configdir}=~/.ssh/config

@{profilelocation}=$ApparmorProfilePath
@{pid}={[1-9],[1-9][0-9],[1-9][0-9][0-9],[1-9][0-9][0-9][0-9],[1-9][0-9][0-9][0-9][0-9],[1-9][0-9][0-9][0-9][0-9][0-9],[1-4][0-9][0-9][0-9][0-9][0-9][0-9]}
@{home}={/root,/home/*}

## Profile Begin
profile SCMController @{exelocation} flags=(enforce) {
  # Receive signals
  signal receive set=(stop term kill quit int urg),
  # Send signals to self
  signal send set=(urg int) peer=SCMController,

  # Capabilities
  network netlink raw,
  network inet stream,
  network inet6 stream,
  unix (create) type=stream,
  unix (create) type=dgram,

  ## Startup Configurations needed
  @{configdir}/** rw,

  ## Program Accesses
  /sys/kernel/mm/transparent_hugepage/hpage_pmd_size r,
  /usr/share/zoneinfo/** r,

  ## Repository access
  # allow read/write for files in repository (write is needed for seeding operations)
  @{repolocation}/{,**} rw,
  # allow locking in git's directory (for commit rollback on early error)
  @{repolocation}/.git/** k,
}
`

	// Can't install apparmor profile without root/sudo
	if os.Geteuid() > 0 {
		printMessage(verbosityStandard, "Need root permissions to install apparmor profile\n")
		return
	}

	// Check if apparmor /sys path exists
	systemAAPath := "/sys/kernel/security/apparmor/profiles"
	_, err := os.Stat(systemAAPath)
	if os.IsNotExist(err) {
		printMessage(verbosityProgress, "AppArmor not supported by this system\n")
		return
	} else if err != nil {
		printMessage(verbosityProgress, "Unable to check if AppArmor is supported by this system: %v\n", err)
		return
	}

	// Write Apparmor Profile to /etc
	err = os.WriteFile(AppArmorProfilePath, []byte(AppArmorProfile), 0644)
	if err != nil {
		printMessage(verbosityProgress, "Failed to write apparmor profile: %v\n", err)
		return
	}

	// Enact Profile
	command := exec.Command("apparmor_parser", "-r", AppArmorProfilePath)
	_, err = command.CombinedOutput()
	if err != nil {
		printMessage(verbosityProgress, "Failed to reload apparmor profile: %v\n", err)
		return
	}

	printMessage(verbosityStandard, "Successfully installed AppArmor Profile\n")
}
