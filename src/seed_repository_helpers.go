// controller
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// Prints out table-like menu for a directory listing
// Prompts the user to supply their choices of files/directories and returns array of choices (in user chosen order)
func dirListMenu(endpointName string, maxNameLenght int, dirList []string, currentDirectory string) (userSelections []string) {
	// Menu (Table) sizing
	const numberOfColumns int = 4
	numberOfDirEntries := len(dirList)
	maxRows := (numberOfDirEntries + numberOfColumns - 1) / numberOfColumns
	columnWidth := maxNameLenght + 4

	// Populate table items
	printMessage(verbosityStandard, "============================================================\n")
	for row := range maxRows {
		for column := range numberOfColumns {
			// Calculate index based on fixed column and row count
			index := row + column*maxRows
			if index >= numberOfDirEntries {
				continue
			}

			fmt.Printf("%-4d %-*s", index+1, columnWidth, dirList[index])
		}
		fmt.Println()
	}
	// User prompt
	printMessage(verbosityStandard, "============================================================\n")
	fmt.Printf("     Select File     Change Dir ^/v   Recursive   Exit\n")
	fmt.Printf("     [ # # ## ### ]  [ c0 ]  [ c# ]    [ #r ]     [ ! ]\n")
	fmt.Printf("%s:%s# Type your selections: ", endpointName, currentDirectory)

	reader := bufio.NewReader(os.Stdin)
	userInput, err := reader.ReadString('\n')
	if err != nil {
		printMessage(verbosityStandard, "\nWarning: could not read user input\n")
	}

	// Split input into individual selections separated by spaces
	userSelections = strings.Fields(userInput)

	if len(userSelections) == 0 {
		printMessage(verbosityProgress, "\nDid not receive any selections!\n")
	}

	// Clear menu rows - add to row count to account for the prompts (only for standard verbosity)
	if globalVerbosityLevel < 2 {
		maxRows += 6
		for maxRows > 0 {
			fmt.Printf("\033[A\033[K")
			maxRows--
		}
	}

	return
}

// Takes user selection array and parses options
// Handles saving file/director choices, changing directories, and exiting selection
func parseUserSelections(userSelections []string, dirList []string, directoryState DirectoryState, host HostMeta) (userRequestedExit bool, selectedFiles []string, directoryStateNew DirectoryState) {
	// Sync current directory state to return value
	directoryStateNew = directoryState

	printMessage(verbosityProgress, "\nParsing Selections for Current Directory: '%s'\n", directoryState.current)

	for _, selection := range userSelections {
		printMessage(verbosityData, "  Selection: '%s'\n", selection)

		dirIndex, err := strconv.Atoi(selection)

		if selection == "!" { // Exit menu only after processing selections

			userRequestedExit = true
			printMessage(verbosityData, "  Requested exit: will exit selections after parsing current selection\n")

		} else if strings.HasSuffix(selection, "r") { // Recurse directory and grab all files

			printMessage(verbosityData, "  Requested recursive selection\n")

			// Remove suffix for recursive
			selection = strings.TrimSuffix(selection, "r")

			// Convert and ensure theres an integer after 'c'
			dirIndex, err = strconv.Atoi(selection)
			if err != nil {
				continue
			}

			// Get file name from user selection number
			name := dirList[dirIndex-1]

			// Only allow recursion for directories
			if !strings.HasSuffix(name, "/") {
				continue
			}

			// Format into absolute path
			absolutePath := filepath.Join(directoryState.current, name)

			printMessage(verbosityData, "  Recursing into directory '%s' for all files\n", absolutePath)

			command := RemoteCommand{"find '" + absolutePath + "' -type f"}
			findOutput, err := command.SSHexec(host.sshClient, config.options.runAsUser, config.options.disableSudo, host.password, 120)
			if err != nil {
				return
			}

			// Ensure empty lines are not fed into selection
			var filteredSelectedFiles []string
			for file := range strings.SplitSeq(findOutput, "\n") {
				if file != "" {
					filteredSelectedFiles = append(filteredSelectedFiles, file)
				}
			}

			// Save all recursively found files to selection
			selectedFiles = append(selectedFiles, filteredSelectedFiles...)

		} else if strings.HasPrefix(selection, "c") { // Find which directory to move to

			printMessage(verbosityData, "  Requested directory change\n")

			// Get the number after 'c'
			changeDirIndex := selection[1:]

			// Convert and ensure theres an integer after 'c'
			dirIndex, err = strconv.Atoi(changeDirIndex)
			if err != nil {
				continue
			}

			// Move directory up or down (0 = up, # = down)
			if dirIndex == 0 {
				// Set next loop directory to dir name above current dir
				directoryStateNew.current = directoryState.stack[len(directoryState.stack)-1]

				printMessage(verbosityData, "    Moving up from '%s' to '%s'\n", directoryState.current, directoryStateNew.current)

				// Remove current directory from the stack
				directoryStateNew.stack = directoryState.stack[:len(directoryState.stack)-1]
			} else if dirIndex >= 1 && dirIndex <= len(dirList) {
				// If selection is not directory, dont cd into anything
				name := dirList[dirIndex-1] // Get file name from user selection number
				if !strings.HasSuffix(name, "/") {
					continue
				}

				// Set next loop directory to chosen dir
				directoryStateNew.current = filepath.Join(directoryState.current, dirList[dirIndex-1])

				printMessage(verbosityData, "    Moving down from '%s' to '%s'\n", directoryState.current, directoryStateNew.current)

				// Add chosen dir to the stack
				directoryStateNew.stack = append(directoryState.stack, directoryState.current)
			}
		} else if err == nil && dirIndex > 0 && dirIndex <= len(dirList) { // Select file by number

			printMessage(verbosityData, "  Requested individual item\n")

			// Get file name from user selection number
			name := dirList[dirIndex-1]

			absolutePath := filepath.Join(directoryState.current, name)

			printMessage(verbosityData, "    Marking item '%s' for retrieval\n", absolutePath)
			selectedFiles = append(selectedFiles, absolutePath)
		} else {
			printMessage(verbosityStandard, "Warning: unknown option '%s'\n", selection)
		}
	}

	return
}

// Walks directory tree above file and retireves its metadata and writes metadata files to repo if it differs from standard system umask
func writeNewDirectoryTreeMetadata(endpointName string, remoteFilePath string, client *ssh.Client, SudoPassword string) (err error) {
	// Directory permissions to ignore
	const defaultOwner string = "root"
	const defaultGroup string = "root"
	const defaultPermissions int = 755

	// Path stack (init without filename)
	remoteDirPath := filepath.Dir(remoteFilePath)

	// Searching for non-default directories
	for range maxDirectoryLoopCount {
		// Break if no more parent dirs
		if remoteDirPath == "." || len(remoteDirPath) < 2 {
			break
		}

		printMessage(verbosityProgress, "  File '%s': Retrieving metadata for parent directory '%s'\n", remoteFilePath, remoteDirPath)

		command := buildStat(remoteDirPath)
		var directoryMetadata string
		directoryMetadata, err = command.SSHexec(client, config.options.runAsUser, config.options.disableSudo, SudoPassword, 10)
		if err != nil {
			err = fmt.Errorf("ssh command failure: %v", err)
			return
		}

		printMessage(verbosityProgress, "  File '%s': Parsing metadata for parent directory '%s'\n", remoteFilePath, remoteDirPath)

		var metadata RemoteFileInfo
		metadata, err = extractMetadataFromStat(directoryMetadata)
		if err != nil {
			return
		}
		if metadata.fsType != dirType {
			printMessage(verbosityProgress, "Warning: expected remote path to be directory, but got type '%s' instead", metadata.fsType)
			continue
		}

		localDirPath := filepath.Join(endpointName, remoteDirPath)

		// Save metadata to map if not the default
		if metadata.owner != defaultOwner || metadata.group != defaultGroup || metadata.permissions != defaultPermissions {
			printMessage(verbosityProgress, "  File '%s': Parent directory '%s' has non-standard metadata, saving\n", remoteFilePath, remoteDirPath)
			err = writeNewDirectoryMetadata(localDirPath, metadata)
			if err != nil {
				printMessage(verbosityProgress, "Warning: unique directory save failed: %v\n", err)
				continue
			}
		}

		// Move up one directory for next loop iteration
		remoteDirPath = filepath.Dir(remoteDirPath)
	}
	return
}

func handleNewReloadCommands(remoteFilePath string, localFilePath string, optCache *SeedRepoUserChoiceCache) (reloadCmds []string, err error) {
	// Recommended reload commands for known configuration files
	// If user wants reloads, they will be prompted to use the reloads below if the file has the prefix of a map key (reloads are optional)
	// names surrounded by '??' indicate sections that should be filled in with relevant info from user selected files
	var DefaultReloadCommands = map[string][]string{
		"/etc/apparmor.d/":         {"apparmor_parser -r /etc/apparmor.d/??baseDirName??"},
		"/etc/crontab":             {"crontab -n /etc/crontab"},
		"/etc/network/":            {"ifup -n -a", "systemctl restart networking.service", "systemctl is-active networking.service"},
		"/etc/nftables":            {"nft -f /etc/nftables.conf -c", "systemctl restart nftables.service", "systemctl is-active nftables.service"},
		"/etc/nginx":               {"nginx -s reload"},
		"/etc/rsyslog":             {"rsyslogd -N1 -f /etc/rsyslog.conf", "systemctl restart rsyslog.service", "systemctl is-active rsyslog.service"},
		"/etc/ssh/sshd":            {"sshd -t", "systemctl restart ssh.service", "systemctl is-active ssh.service"},
		"/etc/sysctl":              {"sysctl -p --dry-run", "sysctl -p"},
		"/etc/systemd/system/":     {"systemd-analyze verify /etc/systemd/system/??baseDirName??", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/lib/systemd/system/":     {"systemd-analyze verify /lib/systemd/system/??baseDirName??", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/usr/lib/systemd/system/": {"systemd-analyze verify /lib/systemd/system/??baseDirName??", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/etc/zabbix":              {"zabbix_agent2 -T -c /etc/zabbix/zabbix_agent2.conf", "systemctl restart zabbix-agent2.service", "systemctl is-active zabbix-agent2.service"},
		"/etc/squid-deb-proxy":     {"squid -f /etc/squid-deb-proxy/squid-deb-proxy.conf -k check", "systemctl restart squid-deb-proxy.service", "systemctl is-active squid-deb-proxy.service"},
		"/etc/squid/":              {"squid -f /etc/squid/squid.conf -k check", "systemctl restart squid.service", "systemctl is-active squid.service"},
		"/etc/syslog-ng":           {"syslog-ng -s", "systemctl restart syslog-ng", "systemctl is-active syslog-ng"},
		"/etc/chrony":              {"chronyd -f /etc/chrony/chrony.conf -p", "systemctl restart chrony.service", "systemctl is-active chrony.service"},
		"/etc/postfix":             {"postfix check", "postfix reload"},
	}

	printMessage(verbosityProgress, "  File '%s': Retrieving reload command information from user\n", remoteFilePath)

	reloadWanted, err := promptUser("Does file '%s' need reload commands? [y/N]: ", localFilePath)
	if err != nil {
		return
	}

	reloadWanted = strings.TrimSpace(reloadWanted)
	reloadWanted = strings.ToLower(reloadWanted)

	if reloadWanted != "y" {
		printMessage(verbosityProgress, "  User did not want reload commands, skipping reload selection logic\n")
		return
	}

	// Repetitive reloads - find most reused to suggest to user
	var mostReusedReload string
	var highestNum int
	for reloadStr, reloadRepeatCnt := range optCache.reloadCnt {
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
			// Target file path does not match any defauts, skipping file
			fileHasNoDefaults = true
			continue
		}
		fileHasNoDefaults = false

		// Show user available commands and ask for confirmation
		fmt.Printf("Selected file has default reload commands available.\n")
		for index, command := range defaultReloadCommandArray {
			// Replace placeholders in default commands with collected information
			if strings.Contains(command, "??") {
				command = strings.Replace(command, "??baseDirName??", filepath.Base(remoteFilePath), -1)
				defaultReloadCommandArray[index] = command
			}

			// Print command on its own line
			fmt.Printf("  - %s\n", command)
		}
		var userConfirmation string
		userConfirmation, err = promptUser("Do you want to use these? [y/N]: ")
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
			fmt.Printf("Default (press enter): '%v'\n", optCache.reloadCmd[mostReusedReload])
		}

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			cmd := scanner.Text()
			if cmd == "" { // Done once empty line
				var userConfirmation string
				userConfirmation, err = promptUser("Are these commands correct? [Y/n]: ")
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
			reloadCmds = optCache.reloadCmd[mostReusedReload]
		}

		optCache.reloadCmd[fmt.Sprintf("%v", reloadCmds)] = reloadCmds
		optCache.reloadCnt[fmt.Sprintf("%v", reloadCmds)]++
	}

	return
}
