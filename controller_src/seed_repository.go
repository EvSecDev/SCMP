// controller
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ###################################
//  SEED REPO FILES FUNCTIONS
// ###################################

// Entry point for user to select remote files to download and format into local repository
func seedRepositoryFiles(hostOverride string, remoteFileOverride string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while seeding repository files", fmt.Errorf("%v", fatalError), false)
		}
	}()

	// Refused seeding without specific hosts specified
	if hostOverride == "" {
		logError("Invalid arguments", fmt.Errorf("remote-hosts cannot be empty when seeding the repository"), false)
	} else if strings.Contains(hostOverride, "*") {
		// TODO: allow wildcards - I'm just being lazy for now
		logError("Invalid arguments", fmt.Errorf("remote-hosts cannot contain wildcards for repository seeding"), false)
	}

	printMessage(VerbosityStandard, "==== Secure Configuration Management Repository Seeding ====\n")

	// Check local system
	err := localSystemChecks()
	logError("Error in system checks", err, false)

	// Check working dir for git repo
	err = retrieveGitRepoPath()
	logError("Repository Error", err, false)

	if dryRunRequested {
		// Notify user that program is in dry run mode
		printMessage(VerbosityStandard, "Requested dry-run, aborting deployment\n")
		if globalVerbosityLevel < 2 {
			// If not running with higher verbosity, no need to collect deployment information
			return
		}
		printMessage(VerbosityProgress, "Outputting information collected for deployment:\n")
	}

	// Retrieve user host choices and put into array
	userHostChoices := strings.Split(hostOverride, ",")

	// Loop hosts chosen by user and prepare relevant host information for deployment
	for _, endpointName := range userHostChoices {
		// Ensure user choice has an entry in the config
		_, configHostFromUserChoice := config.HostInfo[endpointName]
		if !configHostFromUserChoice {
			logError("Invalid host choice", fmt.Errorf("host %s does not exist in config", endpointName), false)
		}

		// Retrieve host secrests (keys,passwords)
		err = retrieveHostSecrets(endpointName)
		logError("Error retrieving host secrets", err, true)

		// Retrieve most current global host config
		hostInfo := config.HostInfo[endpointName]

		// If user requested dry run - print host information and abort connections
		if dryRunRequested {
			printHostInformation(hostInfo)
			continue
		}

		// Connect to the SSH server
		client, err := connectToSSH(hostInfo.Endpoint, hostInfo.EndpointUser, hostInfo.PrivateKey, hostInfo.KeyAlgo)
		logError("Failed connect to SSH server", err, false)
		defer client.Close()

		// Run menu for user to select desired files or direct download
		selectedFiles := make(map[string][]string)
		if remoteFileOverride == "" {
			selectedFiles, err = runSelection(endpointName, client, hostInfo.Password)
			logError("Error retrieving remote file list", err, false)
		} else {
			// Get remote file metadata
			remoteFiles := strings.Split(remoteFileOverride, ",")
			for _, remoteFile := range remoteFiles {
				// Ls the remote file for metadata information
				command := "ls -lA " + remoteFile
				var fileLS string
				fileLS, err = RunSSHCommand(client, command, "", true, hostInfo.Password)
				logError("Failed to retrieve remote file information", err, false)

				// Split ls output into fields for this file
				fileInfo := strings.Fields(fileLS)

				// Skip misc ls output
				if len(fileInfo) < 9 {
					continue
				}

				// Split out permissions and check for directory or regular file
				fileType := string(fileInfo[0][0])

				// Identify if file is directory
				if fileType == "d" {
					// Skip file if dir
					continue
				} else if fileType != "-" {
					// Skip other non-files
					continue
				}

				// Filtering file metadata
				fileName := string(fileInfo[8])
				permissions := string(fileInfo[0][1:])
				fileOwner := string(fileInfo[2])
				fileGroup := string(fileInfo[3])

				// Add file info to map
				selectedFiles[fileName] = append(selectedFiles[fileName], permissions)
				selectedFiles[fileName] = append(selectedFiles[fileName], fileOwner)
				selectedFiles[fileName] = append(selectedFiles[fileName], fileGroup)
			}
		}

		// Initialize buffer file (with random byte) - ensures ownership of buffer stays correct when retrieving remote files
		err = SCPUpload(client, []byte{12}, hostInfo.RemoteTransferBuffer)
		logError(fmt.Sprintf("Failed to initialize buffer file on remote host %s", endpointName), err, false)

		// Download user file choices to local repo and format
		for targetFilePath, fileInfo := range selectedFiles {
			err = retrieveSelectedFile(targetFilePath, fileInfo, endpointName, client, hostInfo.Password, hostInfo.RemoteTransferBuffer)
			logError("Error seeding repository", err, false)
		}
	}

	printMessage(VerbosityStandard, "============================================================\n")
}

// Runs the CLI-based menu that user will use to select which files to download
func runSelection(endpointName string, client *ssh.Client, SudoPassword string) (selectedFiles map[string][]string, err error) {
	// Start selection at root of filesystem - '/'
	directory := "/"
	directoryStack := []string{"/"}

	// Initialize return value
	selectedFiles = make(map[string][]string)

	// Loop until user is done selecting
	for {
		// Get file names and info for the directory
		command := "ls -lA " + directory
		var directoryList string
		directoryList, err = RunSSHCommand(client, command, "", true, SudoPassword)
		if err != nil {
			// All errors except permission denied exits selection menu
			if !strings.Contains(err.Error(), "Permission denied") {
				return
			}

			// Exit menu if it failed reading the first directory after ssh connection (i.e. "/")
			if directory == "/" {
				err = fmt.Errorf("permission denied when reading '/'")
				return
			}

			// Show progress to user
			fmt.Printf("Error: unable to read '%s'\n", directory)

			// Set next loop directory to parent directory
			directory = directoryStack[len(directoryStack)-2]

			// Remove current directory from the stack
			directoryStack = directoryStack[:len(directoryStack)-1]
			continue
		}

		// Create array of files in the directory from the ls output
		directoryListFiles := strings.Split(directoryList, "\n")

		// Initialize vars for holding file information
		var dirList []string
		filesInfo := make(map[string][]string)
		isDir := make(map[string]bool)
		maxLength := 0

		// Extract information from the ls output
		for _, file := range directoryListFiles {
			// Split ls output into fields for this file
			fileInfo := strings.Fields(file)

			// Skip misc ls output
			if len(fileInfo) < 9 {
				continue
			}

			// Determine column spacing from longest file name
			if length := len(fileInfo[8]); length > maxLength {
				maxLength = length
			}

			// Split out permissions and check for directory or regular file
			fileType := string(fileInfo[0][0])

			// Add file names to their own index - for selection reference
			dirList = append(dirList, fileInfo[8])

			// Identify if file is directory
			if fileType == "d" {
				// Skip further processing of directories
				isDir[fileInfo[8]] = true
				continue
			} else if fileType == "-" {
				isDir[fileInfo[8]] = false
			}

			// Filtering file metadata
			permissions := string(fileInfo[0][1:])
			fileOwner := string(fileInfo[2])
			fileGroup := string(fileInfo[3])

			// Add file info to map
			filesInfo[fileInfo[8]] = append(filesInfo[fileInfo[8]], permissions)
			filesInfo[fileInfo[8]] = append(filesInfo[fileInfo[8]], fileOwner)
			filesInfo[fileInfo[8]] = append(filesInfo[fileInfo[8]], fileGroup)
		}

		// Use the length of dir list after filtering
		numberOfDirEntries := len(dirList)

		// Show Menu - Print the directory contents in columns
		fmt.Printf("============================================================\n")
		numberOfColumns := 4
		maxRows := (numberOfDirEntries + numberOfColumns - 1) / numberOfColumns
		columnWidth := maxLength + 4
		for row := 0; row < maxRows; row++ {
			for column := 0; column < numberOfColumns; column++ {
				// Calculate index based on fixed column and row count
				index := row + column*maxRows
				if index >= numberOfDirEntries {
					continue
				}

				// Get file name at current index
				name := dirList[index]

				// Add slash to dir names
				if isDir[name] {
					name += "/"
				}

				// Print the file name
				fmt.Printf("%-4d %-*s", index+1, columnWidth, name)
			}
			// Newline before next row
			fmt.Printf("\n")
		}
		// User prompt
		fmt.Printf("\n============================================================\n")
		fmt.Printf("         Select File     Change Dir ^v   Exit\n")
		fmt.Printf("         [ # # ## ### ]  [ c0 ] [ c# ]   [ ! ]\n")
		fmt.Printf("%s:%s# Type your selections: ", endpointName, directory)

		// Read user input
		reader := bufio.NewReader(os.Stdin)
		userInput, _ := reader.ReadString('\n')

		// Split input into individual selections separated by spaces
		selections := strings.Fields(userInput)

		// Clear menu rows - add to row count to account for the prompts
		maxRows += 6
		for maxRows > 0 {
			fmt.Printf("\033[A\033[K")
			maxRows--
		}

		// Parse user selections for this directory
		var exitRequested bool
		for _, selection := range selections {
			// Convert selection to integer
			dirIndex, err := strconv.Atoi(selection)

			if selection == "!" {
				// Exit menu only after processing selections
				exitRequested = true
			} else if strings.HasPrefix(selection, "c") { // Find which directory to move to
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
					directory = directoryStack[len(directoryStack)-2]

					// Remove current directory from the stack
					directoryStack = directoryStack[:len(directoryStack)-1]
				} else if dirIndex >= 1 && dirIndex <= numberOfDirEntries {
					// Set next loop directory to chosen dir
					directory = filepath.Join(directory, dirList[dirIndex-1])

					// Add chosen dir to the stack
					directoryStack = append(directoryStack, directory)
				}
			} else if err == nil && dirIndex > 0 && dirIndex <= numberOfDirEntries { // Select file by number
				// Get file name from user selection number
				name := dirList[dirIndex-1]

				// Skip dirs if selected
				if isDir[name] {
					continue
				}

				// Format into absolute path
				absolutePath := filepath.Join(directory, name)

				// Save file and relevant metadata into map
				selectedFiles[absolutePath] = append(selectedFiles[absolutePath], filesInfo[name][0])
				selectedFiles[absolutePath] = append(selectedFiles[absolutePath], filesInfo[name][1])
				selectedFiles[absolutePath] = append(selectedFiles[absolutePath], filesInfo[name][2])
			}
		}

		// Exit selection if user requested
		if exitRequested {
			break
		}
	}

	return
}

// Downloads user selected files from remote host
// Adds metadata header
// Recreates directory structure of remote host in the local repository
func retrieveSelectedFile(targetFilePath string, fileInfo []string, endpointName string, client *ssh.Client, SudoPassword string, tmpRemoteFilePath string) (err error) {
	// Recommended reload commands for known configuration files
	// If user wants reloads, they will be prompted to use the reloads below if the file has the prefix of a map key (reloads are optional)
	// names surrounded by '??' indicate sections that should be filled in with relevant info from user selected files
	var DefaultReloadCommands = map[string][]string{
		"/etc/apparmor.d/":     {"apparmor_parser -r /etc/apparmor.d/??baseDirName??"},
		"/etc/crontab":         {"crontab -n /etc/crontab"},
		"/etc/network/":        {"ifup -s -a", "systemctl restart networking.service", "systemctl is-active networking.service"},
		"/etc/nftables":        {"nft -f /etc/nftables.conf -c", "systemctl restart nftables.service", "systemctl is-active nftables.service"},
		"/etc/nginx":           {"nginx -t", "systemctl restart nginx.service", "systemctl is-active nginx.service"},
		"/etc/rsyslog":         {"rsyslogd -N1 -f /etc/rsyslog.conf", "systemctl restart rsyslog.service", "systemctl is-active rsyslog.service"},
		"/etc/ssh/sshd":        {"sshd -t", "systemctl restart ssh.service", "systemctl is-active ssh.service"},
		"/etc/sysctl":          {"sysctl -p --dry-run", "sysctl -p"},
		"/etc/systemd/system/": {"systemd-analyze verify /etc/systemd/system/??baseDirName??", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/etc/zabbix":          {"zabbix_agent2 -T -c /etc/zabbix/zabbix_agent2.conf", "systemctl restart zabbix-agent2.service", "systemctl is-active zabbix-agent2.service"},
		"/etc/squid-deb-proxy": {"squid -f /etc/squid-deb-proxy/squid-deb-proxy.conf -k check", "systemctl restart squid-deb-proxy.service", "systemctl is-active squid-deb-proxy.service"},
		"/etc/squid/":          {"squid -f /etc/squid/squid.conf -k check", "systemctl restart squid.service", "systemctl is-active squid.service"},
		"/etc/syslog-ng":       {"syslog-ng -s", "systemctl restart syslog-ng", "systemctl is-active syslog-ng"},
	}

	// Copy desired file to buffer location
	command := "cp " + targetFilePath + " " + tmpRemoteFilePath
	_, err = RunSSHCommand(client, command, "", true, SudoPassword)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	// Ensure buffer file can be read and then deleted later
	command = "chmod 666 " + tmpRemoteFilePath
	_, err = RunSSHCommand(client, command, "", true, SudoPassword)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	// Download remote file contents
	fileContents, err := SCPDownload(client, tmpRemoteFilePath)
	if err != nil {
		return
	}

	// Replace target path separators with local os ones
	hostFilePath := strings.ReplaceAll(targetFilePath, "/", config.OSPathSeparator)

	// Use target file path and hosts name for repo file location
	configFilePath := endpointName + hostFilePath

	// Convert permissions string to number format
	numberPermissions := permissionsSymbolicToNumeric(fileInfo[0])

	// Put metadata into JSON format
	var metadataHeader MetaHeader
	metadataHeader.TargetFileOwnerGroup = fileInfo[1] + ":" + fileInfo[2]
	metadataHeader.TargetFilePermissions = numberPermissions

	// Ask user for confirmation to use reloads
	reloadWanted, err := promptUser("Does file '%s' need reload commands? [y/N]: ", configFilePath)
	if err != nil {
		return
	}

	// Format user choice
	reloadWanted = strings.TrimSpace(reloadWanted)
	reloadWanted = strings.ToLower(reloadWanted)

	// Setup metadata depending on user choice
	if reloadWanted == "y" {
		var reloadCmds []string

		// Search known files for a match
		var userDoesNotWantDefaults, fileHasNoDefaults bool
		for filePathPrefix, defaultReloadCommandArray := range DefaultReloadCommands {
			if !strings.Contains(targetFilePath, filePathPrefix) {
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
					command = strings.Replace(command, "??baseDirName??", filepath.Base(targetFilePath), -1)
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
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				cmd := scanner.Text()
				if cmd == "" { // Done once empty line
					// Get confirmation of input
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
		}

		// Write user supplied command array to metadata header
		metadataHeader.ReloadCommands = reloadCmds
	}

	printMessage(VerbosityProgress, "Adding JSON metadata header to file %s\n", configFilePath)

	// Marshal metadata JSON
	metadata, errNoFatal := json.MarshalIndent(metadataHeader, "", "  ")
	if errNoFatal != nil {
		printMessage(VerbosityStandard, "Failed to marshal metadata header into JSON format for file %s: %v\n", configFilePath, errNoFatal)
		return
	}

	// Add header to file contents
	configFile := Delimiter + "\n" + string(metadata) + "\n" + Delimiter + "\n" + fileContents

	printMessage(VerbosityProgress, "Writing file %s to repository\n", configFilePath)

	// Create any missing directories in repository
	configParentDirs := filepath.Dir(configFilePath)
	errNoFatal = os.MkdirAll(configParentDirs, os.ModePerm)
	if errNoFatal != nil {
		printMessage(VerbosityStandard, "Failed to create missing directories in local repository for file '%s': %v\n", configFilePath, errNoFatal)
		return
	}

	// Write config to file in repository
	errNoFatal = os.WriteFile(configFilePath, []byte(configFile), 0600)
	if errNoFatal != nil {
		printMessage(VerbosityStandard, "Failed to write file '%s' to local repository: %v\n", configFilePath, errNoFatal)
		return
	}

	return
}

// Converts symbolic linux permission to numeric representation
// Like rwxr-x-rx -> 755
func permissionsSymbolicToNumeric(permissions string) (perm int) {
	var bits string
	// Loop permission fields
	for _, field := range []string{permissions[:3], permissions[3:6], permissions[6:]} {
		bit := 0
		// Read
		if strings.Contains(field, "r") {
			bit += 4
		}
		// Write
		if strings.Contains(field, "w") {
			bit += 2
		}
		// Execute
		if strings.Contains(field, "x") {
			bit += 1
		}
		// Convert sum'd bits to string to concat with other loop iterations
		bits = bits + strconv.Itoa(bit)
	}

	// Convert back to integer (ignore error, we control all input values)
	perm, _ = strconv.Atoi(bits)
	return
}
