// Package for SSH-specific configuration loading and parsing
package sshconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/internal/config"
	"scmp/internal/fsops"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

func Set(ctx context.Context, configFilePath string) (newCtx context.Context, err error) {
	var cfg config.Config
	newCtx = ctx

	configFilePath, err = fsops.ExpandHomeDirectory(configFilePath)
	if err != nil {
		err = fmt.Errorf("resolving config file path failed: %w", err)
		return
	}

	// Load Config File
	sshConfigFile, err := os.ReadFile(configFilePath)
	if err != nil {
		err = fmt.Errorf("reading config failed: %w", err)
		return
	}
	sshConfigContents := string(sshConfigFile)

	// Retrieve SSH Config file options
	sshConfig, err := ssh_config.Decode(strings.NewReader(sshConfigContents))
	if err != nil {
		err = fmt.Errorf("failed decoding config file: %w", err)
		return
	}

	// Do everything with relative paths inside repository
	cfg.RepositoryPath, err = gitinternal.RetrieveRepoPath(ctx)
	if err != nil {
		err = fmt.Errorf("failed retrieving local repository path: %w", err)
		return
	}
	err = os.Chdir(cfg.RepositoryPath)
	if err != nil {
		err = fmt.Errorf("failed decoding config file: %w", err)
		return
	}

	// Retrieve known_hosts file path
	cfg.KnownHostsFilePath, _ = sshConfig.Get("*", "UserKnownHostsFile")
	if cfg.KnownHostsFilePath == "" {
		sshConfDir := filepath.Dir(configFilePath)
		cfg.KnownHostsFilePath = filepath.Join(sshConfDir, sshinternal.KnownHostsFile)
	}

	// Format known_hosts path correctly
	cfg.KnownHostsFilePath, err = fsops.ExpandHomeDirectory(cfg.KnownHostsFilePath)
	if err != nil {
		err = fmt.Errorf("failed to resolve absolute path to '%s': %w", cfg.KnownHostsFilePath, err)
		return
	}

	// Ensure known_hosts file exists, if not create it
	_, err = os.Stat(cfg.KnownHostsFilePath)
	if os.IsNotExist(err) {
		var knownHostsFile *os.File
		knownHostsFile, err = os.Create(cfg.KnownHostsFilePath)
		if err != nil {
			return
		}
		_ = knownHostsFile.Close()
	} else if err != nil {
		return
	}

	// Read in file
	knownHostFile, err := os.ReadFile(cfg.KnownHostsFilePath)
	if err != nil {
		err = fmt.Errorf("unable to read known_hosts file: %w", err)
		return
	}

	// Store all known_hosts as array
	cfg.KnownHosts = strings.Split(string(knownHostFile), "\n")

	// All config dir names in repo
	universalDir, _ := sshConfig.Get("", "UniversalDirectory")
	if strings.Contains(universalDir, string(os.PathSeparator)) {
		err = fmt.Errorf("UniversalDirectory should be a relative path from the root of repository")
		return
	}
	cfg.UniversalDirectory = str.RepoRootDir(universalDir)

	// Password vault file
	vaultRelPath, _ := sshConfig.Get("", "PasswordVault")
	cfg.VaultFilePath, err = fsops.ExpandHomeDirectory(vaultRelPath)
	if err != nil {
		err = fmt.Errorf("failed to resolve absolute path to '%s': %w", vaultRelPath, err)
		return
	}

	// Initialize vault map
	cfg.Vault = make(map[str.RepoRootDir]config.Credential)

	// Array of Hosts and their info
	cfg.HostInfo = make(map[str.RepoRootDir]config.EndpointInfo)
	cfg.AllUniversalGroups = make(map[str.RepoRootDir][]str.RepoRootDir)
	var hostInfo config.EndpointInfo
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

		hostDir := str.RepoRootDir(hostPattern)

		// Save hostname into info map
		hostInfo.EndpointName = hostDir

		// Save user into info map
		hostInfo.EndpointUser, _ = sshConfig.Get(hostPattern, "User")

		// First item must be present
		endpointAddr, _ := sshConfig.Get(hostPattern, "Hostname")

		// Get port from endpoint
		endpointPort, _ := sshConfig.Get(hostPattern, "Port")

		// Network Address Parsing - only if address
		if endpointAddr != "" && endpointPort != "" {
			hostInfo.Endpoint, err = sshinternal.ParseEndpointAddress(endpointAddr, endpointPort)
			if err != nil {
				err = fmt.Errorf("failed parsing network address: %w", err)
				return
			}
		}

		// Get timeout value if present
		connectTimeout, _ := sshConfig.Get(hostPattern, "ConnectTimeout")
		if connectTimeout != "" {
			hostInfo.ConnectTimeout, err = strconv.Atoi(connectTimeout)
			if err != nil {
				err = fmt.Errorf("failed parsing connect timeout value: %w", err)
				return
			}
		}

		// Get proxy
		hostInfo.Proxy, _ = sshConfig.Get(hostPattern, "ProxyJump")

		// Get identity file path
		hostInfo.IdentityFile, _ = sshConfig.Get(hostPattern, "IdentityFile")
		hostInfo.IdentityFile, err = fsops.ExpandHomeDirectory(hostInfo.IdentityFile)
		if err != nil {
			err = fmt.Errorf("failed to resolve absolute path to '%s': %w", hostInfo.IdentityFile, err)
			return
		}

		// Create list of hosts that would need vault access
		passwordRequired, _ := sshConfig.Get(hostPattern, "PasswordRequired")
		if strings.ToLower(passwordRequired) == "yes" {
			hostInfo.RequiresVault = true
		} else {
			hostInfo.RequiresVault = false
		}

		// Save deployment state of this host
		hostInfo.DeploymentState, _ = sshConfig.Get(hostPattern, "DeploymentState")

		// Get all groups this host is a part of
		universalGroupsCSV, _ := sshConfig.Get(hostPattern, "GroupTags")

		// Get yes/no if host ignores main universal
		ignoreUniversalString, _ := sshConfig.Get(hostPattern, "IgnoreUniversal")

		// Parse config host groups into necessary global/host variables
		hostInfo.IgnoreUniversal, hostInfo.UniversalGroups = filterHostGroups(cfg, hostDir, universalGroupsCSV, ignoreUniversalString)

		// write into config
		cfg.HostInfo[hostDir] = hostInfo
	}

	newCtx = context.WithValue(ctx, global.ConfKey, cfg)
	return
}

// Creates two maps relating to host groups
// First map: key'd on group and contains only groups that the host is a part of (values are empty)
func filterHostGroups(cfg config.Config, endpointName str.RepoRootDir, universalGroupsCSV string, ignoreUniversalString string) (hostIgnoresUniversal bool, hostUniversalGroups map[str.RepoRootDir]struct{}) {
	// Convert CSV of host groups to array
	universalGroupsList := strings.Split(universalGroupsCSV, ",")

	// If host ignores universal configs
	if strings.ToLower(ignoreUniversalString) == "yes" {
		hostIgnoresUniversal = true
	} else {
		hostIgnoresUniversal = false

		// Not ignoring, make this host a part of the universal group
		universalGroupsList = append(universalGroupsList, string(cfg.UniversalDirectory))
	}

	// Get universal groups this host is a part of
	hostUniversalGroups = make(map[str.RepoRootDir]struct{})
	for _, group := range universalGroupsList {
		universalGroup := str.RepoRootDir(group)

		// Skip empty hosts' group
		if universalGroup == "" {
			continue
		}

		// Map of groups that this host is a part of
		hostUniversalGroups[universalGroup] = struct{}{}

		// Add this hosts name to the global universal map for groups this host is a part of
		cfg.AllUniversalGroups[universalGroup] = append(cfg.AllUniversalGroups[universalGroup], endpointName)
	}

	return
}
