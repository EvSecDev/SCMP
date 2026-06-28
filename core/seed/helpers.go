package seed

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/deployment/remote"
	"scmp/core/filesystem/content"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/input"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Takes a full ls -lA from a directory and extracts:
// - Array of all file names
// - a map keyed on file name of each files metadata
// - the maximum file name length encountered
func parseDirEntries(lsDirOutput string) (dirList []string, maxNameLenght int) {
	// Create array of files in the directory from the ls output
	directoryListFiles := strings.SplitSeq(lsDirOutput, "\n")

	// Extract information from the ls output
	for fileName := range directoryListFiles {
		if fileName == "" {
			continue
		}

		// Determine column spacing from longest file name
		if length := len(fileName); length > maxNameLenght {
			maxNameLenght = length
		}

		// Add file names to their own index - for selection reference
		dirList = append(dirList, fileName)
	}

	return
}

// Walks directory tree above file and retrieves its metadata and writes metadata files to repo if it differs from standard system umask
func writeNewDirectoryTreeMetadata(ctx context.Context, endpointName string, remoteFilePath string, client *ssh.Client, SudoPassword string) (err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "options", global.OpsKey, "config.Opts")

	// Directory permissions to ignore
	const defaultOwner string = "root"
	const defaultGroup string = "root"
	const defaultPermissions int = 755

	// Path stack (init without filename)
	remoteDirPath := filepath.Dir(remoteFilePath)

	// Searching for non-default directories
	for range global.MaxDirectoryLoopCount {
		// Break if no more parent dirs
		if remoteDirPath == "." || len(remoteDirPath) < 2 {
			break
		}

		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  File '%s': Retrieving metadata for parent directory '%s'\n", remoteFilePath, remoteDirPath)

		command := sshinternal.BuildStat(str.RemotePath(remoteDirPath))
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser

		var directoryMetadata string
		directoryMetadata, err = command.SSHexec(ctx, client, SudoPassword)
		if err != nil {
			err = fmt.Errorf("ssh command failure: %w", err)
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  File '%s': Parsing metadata for parent directory '%s'\n", remoteFilePath, remoteDirPath)

		var metadata sshinternal.RemoteFileInfo
		metadata, err = sshinternal.ExtractMetadataFromStat(directoryMetadata)
		if err != nil {
			return
		}
		if metadata.FsType != remote.DirType {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.WarnLog, "Expected remote path to be directory, but got type '%s' instead", metadata.FsType)
			continue
		}

		localDirPath := str.LocalRepoPath(filepath.Join(endpointName, remoteDirPath))

		// Save metadata to map if not the default
		if metadata.Owner != defaultOwner || metadata.Group != defaultGroup || metadata.Permissions != defaultPermissions {
			logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  File '%s': Parent directory '%s' has non-standard metadata, saving\n", remoteFilePath, remoteDirPath)
			err = content.WriteNewDirectoryMetadata(ctx, localDirPath, metadata)
			if err != nil {
				logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.WarnLog, "unique directory save failed: %v\n", err)
				continue
			}
		}

		// Move up one directory for next loop iteration
		remoteDirPath = filepath.Dir(remoteDirPath)
	}
	return
}

func handleNewReloadCommands(ctx context.Context, remoteFilePath string, localFilePath string, optCache *RepoUserChoiceCache) (reloadCmds []string, err error) {
	// Recommended reload commands for known configuration files
	// If user wants reloads, they will be prompted to use the reloads below if the file has the prefix of a map key (reloads are optional)
	// names surrounded by '??' indicate sections that should be filled in with relevant info from user selected files
	var DefaultReloadCommands = map[string][]string{
		"/etc/apparmor.d/":         {"apparmor_parser -r <scmp://_local@repo.file.path>"},
		"/etc/crontab":             {"crontab -n /etc/crontab"},
		"/etc/network/":            {"ifup -n -a", "systemctl restart networking.service", "systemctl is-active networking.service"},
		"/etc/nftables":            {"nft -f /etc/nftables.conf -c", "systemctl restart nftables.service", "systemctl is-active nftables.service"},
		"/etc/nginx":               {"nginx -s reload"},
		"/etc/rsyslog":             {"rsyslogd -N1 -f /etc/rsyslog.conf", "systemctl restart rsyslog.service", "systemctl is-active rsyslog.service"},
		"/etc/ssh/sshd":            {"sshd -t", "systemctl restart ssh.service", "systemctl is-active ssh.service"},
		"/etc/sysctl":              {"sysctl -p --dry-run", "sysctl -p"},
		"/etc/systemd/system/":     {"systemd-analyze verify <scmp://_local@repo.file.path>", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/lib/systemd/system/":     {"systemd-analyze verify <scmp://_local@repo.file.path>", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/usr/lib/systemd/system/": {"systemd-analyze verify <scmp://_local@repo.file.path>", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/etc/zabbix":              {"zabbix_agent2 -T -c /etc/zabbix/zabbix_agent2.conf", "systemctl restart zabbix-agent2.service", "systemctl is-active zabbix-agent2.service"},
		"/etc/squid-deb-proxy":     {"squid -f /etc/squid-deb-proxy/squid-deb-proxy.conf -k check", "systemctl restart squid-deb-proxy.service", "systemctl is-active squid-deb-proxy.service"},
		"/etc/squid/":              {"squid -f /etc/squid/squid.conf -k check", "systemctl restart squid.service", "systemctl is-active squid.service"},
		"/etc/syslog-ng":           {"syslog-ng -s", "systemctl restart syslog-ng", "systemctl is-active syslog-ng"},
		"/etc/chrony":              {"chronyd -f /etc/chrony/chrony.conf -p", "systemctl restart chrony.service", "systemctl is-active chrony.service"},
		"/etc/postfix":             {"postfix check", "postfix reload"},
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  File '%s': Retrieving reload command information from user\n", remoteFilePath)

	reloadWanted, err := input.AskUser(ctx, fmt.Sprintf("Does file '%s' need reload commands? [y/N]", localFilePath), "")
	if err != nil {
		return
	}

	reloadWanted = strings.TrimSpace(reloadWanted)
	reloadWanted = strings.ToLower(reloadWanted)

	if reloadWanted != "y" {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  User did not want reload commands, skipping reload selection logic\n")
		return
	}

	// Repetitive reloads - find most reused to suggest to user
	var mostReusedReload string
	var highestNum int
	for reloadStr, reloadRepeatCnt := range optCache.ReloadCnt {
		if reloadRepeatCnt < 2 {
			continue
		}

		if reloadRepeatCnt > highestNum {
			highestNum = reloadRepeatCnt
		}

		mostReusedReload = reloadStr
	}

	// Setup metadata depending on user choice
	// Search known files for a match
	var userDoesNotWantDefaults, fileHasNoDefaults bool
	for filePathPrefix, defaultReloadCommandArray := range DefaultReloadCommands {
		if !strings.Contains(remoteFilePath, filePathPrefix) {
			// Target file path does not match any defaults, skipping file
			fileHasNoDefaults = true
			continue
		}
		fileHasNoDefaults = false

		// Show user available commands and ask for confirmation
		fmt.Printf("Selected file has default reload commands available.\n")
		for index, command := range defaultReloadCommandArray {
			// Replace placeholders in default commands with collected information
			if strings.Contains(command, "??") {
				command = strings.ReplaceAll(command, "??baseDirName??", filepath.Base(remoteFilePath))
				defaultReloadCommandArray[index] = command
			}

			// Print command on its own line
			fmt.Printf("  - %s\n", command)
		}
		var userConfirmation string
		userConfirmation, err = input.AskUser(ctx, "Do you want to use these? [y/N]", "")
		if err != nil {
			return
		}
		userConfirmation = strings.TrimSpace(userConfirmation)
		userConfirmation = strings.ToLower(userConfirmation)

		// User did not say yes, skip using default reload commands
		if userConfirmation != "y" {
			userDoesNotWantDefaults = true
			fileHasNoDefaults = false
			break
		}

		// User does want default commands, save to array and stop default search
		reloadCmds = defaultReloadCommandArray
		break
	}

	// Get array of commands from user
	if userDoesNotWantDefaults || fileHasNoDefaults {
		fmt.Printf("Enter reload commands (press Enter after each command, leave an empty line to finish):\n")
		if mostReusedReload != "" {
			fmt.Printf("Default (press enter): '%v'\n", optCache.ReloadCmd[mostReusedReload])
		}

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			cmd := scanner.Text()
			if cmd == "" { // Done once empty line
				var userConfirmation string
				userConfirmation, err = input.AskUser(ctx, "Are these commands correct? [Y/n]", "")
				if err != nil {
					return
				}
				userConfirmation = strings.TrimSpace(userConfirmation)
				userConfirmation = strings.ToLower(userConfirmation)
				if userConfirmation == "y" {
					break
				}
				// Reset array of commands
				reloadCmds = nil
				fmt.Printf("Enter reload commands (press Enter after each command, leave an empty line to finish):\n")
				continue
			}
			reloadCmds = append(reloadCmds, cmd)

		}
		if len(reloadCmds) == 0 {
			reloadCmds = optCache.ReloadCmd[mostReusedReload]
		}

		optCache.ReloadCmd[fmt.Sprintf("%v", reloadCmds)] = reloadCmds
		optCache.ReloadCnt[fmt.Sprintf("%v", reloadCmds)]++
	}

	return
}
