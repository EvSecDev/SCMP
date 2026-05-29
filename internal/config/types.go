// Package for program-wide deployment configuration
package config

import (
	"scmp/internal/str"

	"golang.org/x/crypto/ssh"
)

// Per-user parsed config
type Config struct {
	HostInfo           map[str.RepoRootDir]EndpointInfo      // Hold some basic information about all the hosts
	KnownHostsFilePath string                                // Path to known server public keys - ~/.ssh/known_hosts
	AddAllUnknownHosts bool                                  // User option to always add unknown host keys
	KnownHosts         []string                              // Content of known server public keys - ~/.ssh/known_hosts
	RepositoryPath     string                                // Absolute path to git repository (based on current working dir)
	UniversalDirectory str.RepoRootDir                       // Universal config directory inside git repo
	AllUniversalGroups map[str.RepoRootDir][]str.RepoRootDir // Universal group config directory names and their respective hosts
	VaultFilePath      string                                // Path to password vault file
	Vault              map[str.RepoRootDir]Credential        // Password vault
}

type Credential struct {
	LoginUserPassword string `json:"loginUserPassword"` // For secrets vault
}

// Host-specific information/config
type EndpointInfo struct {
	DeploymentState string                       // Avoids deploying anything to host - so user can prevent deployments to otherwise up and health hosts
	IgnoreUniversal bool                         // Prevents deployments for this host to use anything from the primary Universal configs directory
	RequiresVault   bool                         // Direct match to the config option "PasswordRequired"
	UniversalGroups map[str.RepoRootDir]struct{} // Map to store the CSV for config option "GroupTags"
	EndpointName    str.RepoRootDir              // Name of host as it appears in config and in git repo top-level directory names
	Proxy           string                       // Name of the proxy host to use (if any)
	Endpoint        string                       // Address:port of the host
	EndpointUser    string                       // Login user name of the host
	IdentityFile    string                       // Key identity file path (private or public)
	PrivateKey      ssh.Signer                   // Actual private key contents
	KeyAlgo         string                       // Algorithm of the private key
	Password        string                       // Password for the EndpointUser
	ConnectTimeout  int                          // Timeout in seconds for connection to this host
}

// User supplied options
type Opts struct {
	MaxSSHConcurrency        int    // Maximum threads for ssh sessions
	MaxDeployConcurrency     int    // Maximum threads for file deployments per host
	DryRunEnabled            bool   // Tests deployment setup without connecting to remotes
	WetRunEnabled            bool   // Tests deployment on remotes without mutating anything
	RunAsUser                string // User to run commands as (not login user)
	DisableSudo              bool   // Disable using sudo for remote commands
	AllowDeletions           bool   // Allow deletions in local repo to delete files on remote hosts or vault entries
	DisableReloads           bool   // Disables all deployment reload commands for this deployment
	RunInstallCommands       bool   // Run the install command section of all relevant files metadata header section (within the given deployment)
	IgnoreDeploymentState    bool   // Ignore any deployment state for a host in the config
	RegexEnabled             bool   // Globally enable the use of regex for matching hosts/files
	ForceEnabled             bool   // Atomic mode
	DetailedSummaryRequested bool   // Generate a summary report of the deployment
	ExecutionTimeout         int    // Timeout in seconds for user-defined commands (Reloads,checks,exec,ect.)
}
