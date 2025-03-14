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

	printMessage(verbosityStandard, "==== Secure Configuration Management Repository Seeding ====\n")

	// Check local system
	err := localSystemChecks()
	logError("Error in system checks", err, false)

	// Check working dir for git repo
	err = retrieveGitRepoPath()
	logError("Repository Error", err, false)

	if dryRunRequested {
		// Notify user that program is in dry run mode
		printMessage(verbosityStandard, "Requested dry-run, aborting deployment\n")
		if globalVerbosityLevel < 2 {
			// If not running with higher verbosity, no need to collect deployment information
			return
		}
		printMessage(verbosityProgress, "Outputting information collected for deployment:\n")
	}

	// Loop hosts chosen by user and prepare relevant host information for deployment
	for endpointName, hostInfo := range config.hostInfo {
		skipHost := checkForOverride(hostOverride, endpointName)
		if skipHost {
			printMessage(verbosityProgress, "  Skipping host %s, not desired\n", endpointName)
			continue
		}

		// Retrieve host secrests (keys,passwords)
		err = retrieveHostSecrets(endpointName)
		logError("Error retrieving host secrets", err, true)

		// Retrieve most current global host config
		hostInfo = config.hostInfo[endpointName]

		// If user requested dry run - print host information and abort connections
		if dryRunRequested {
			printHostInformation(hostInfo)
			continue
		}

		// Connect to the SSH server
		client, err := connectToSSH(hostInfo.endpointName, hostInfo.endpoint, hostInfo.endpointUser, hostInfo.password, hostInfo.privateKey, hostInfo.keyAlgo)
		logError("Failed connect to SSH server", err, false)
		defer client.Close()

		// Run menu for user to select desired files or direct download
		selectedFiles := make(map[string][]string)
		if remoteFileOverride == "" {
			selectedFiles, err = runSelection(endpointName, client, hostInfo.password)
			logError("Error retrieving remote file list", err, false)
		} else {
			// Get remote file metadata
			remoteFiles := strings.Split(remoteFileOverride, ",")
			for _, remoteFile := range remoteFiles {
				// Ls the remote file for metadata information
				command := "ls -lA " + remoteFile
				var fileLS string
				fileLS, err = runSSHCommand(client, command, "", config.disableSudo, hostInfo.password, 30)
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
		err = SCPUpload(client, []byte{12}, hostInfo.remoteTransferBuffer)
		logError(fmt.Sprintf("Failed to initialize buffer file on remote host %s", endpointName), err, false)

		// Download user file choices to local repo and format
		for targetFilePath, fileInfo := range selectedFiles {
			err = retrieveSelectedFile(targetFilePath, fileInfo, endpointName, client, hostInfo.password, hostInfo.remoteTransferBuffer)
			logError("Error seeding repository", err, false)
		}
	}

	printMessage(verbosityStandard, "============================================================\n")
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
		directoryList, err = runSSHCommand(client, command, "", config.disableSudo, SudoPassword, 30)
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
			// Retrieve File metadata
			fileType, permissions, fileOwner, fileGroup, _, fileName, errLocal := extractMetadataFromLS(file)
			if errLocal != nil {
				// Only error for this function is incomplete ls, which we skip
				continue
			}

			// Determine column spacing from longest file name
			if length := len(fileName); length > maxLength {
				maxLength = length
			}

			// Add file names to their own index - for selection reference
			dirList = append(dirList, fileName)

			// Identify if file is directory
			if fileType == "d" {
				// Skip further processing of directories
				isDir[fileName] = true
				continue
			} else if fileType == "-" {
				isDir[fileName] = false
			}

			// Add file info to map
			filesInfo[fileName] = append(filesInfo[fileName], permissions)
			filesInfo[fileName] = append(filesInfo[fileName], fileOwner)
			filesInfo[fileName] = append(filesInfo[fileName], fileGroup)
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
		"/etc/network/":        {"ifup -n -a", "systemctl restart networking.service", "systemctl is-active networking.service"},
		"/etc/nftables":        {"nft -f /etc/nftables.conf -c", "systemctl restart nftables.service", "systemctl is-active nftables.service"},
		"/etc/nginx":           {"nginx -s reload"},
		"/etc/rsyslog":         {"rsyslogd -N1 -f /etc/rsyslog.conf", "systemctl restart rsyslog.service", "systemctl is-active rsyslog.service"},
		"/etc/ssh/sshd":        {"sshd -t", "systemctl restart ssh.service", "systemctl is-active ssh.service"},
		"/etc/sysctl":          {"sysctl -p --dry-run", "sysctl -p"},
		"/etc/systemd/system/": {"systemd-analyze verify /etc/systemd/system/??baseDirName??", "systemctl daemon-reload", "systemctl restart ??baseDirName??", "systemctl is-active ??baseDirName??"},
		"/etc/zabbix":          {"zabbix_agent2 -T -c /etc/zabbix/zabbix_agent2.conf", "systemctl restart zabbix-agent2.service", "systemctl is-active zabbix-agent2.service"},
		"/etc/squid-deb-proxy": {"squid -f /etc/squid-deb-proxy/squid-deb-proxy.conf -k check", "systemctl restart squid-deb-proxy.service", "systemctl is-active squid-deb-proxy.service"},
		"/etc/squid/":          {"squid -f /etc/squid/squid.conf -k check", "systemctl restart squid.service", "systemctl is-active squid.service"},
		"/etc/syslog-ng":       {"syslog-ng -s", "systemctl restart syslog-ng", "systemctl is-active syslog-ng"},
		"/etc/chrony":          {"chronyd -f /etc/chrony/chrony.conf -p", "systemctl restart chrony.service", "systemctl is-active chrony.service"},
	}

	// Default Directory permissions (to ignore)
	const defaultOwnerGroup string = "root:root"
	const defaultPermissions int = 755

	// Map to hold metadata for each directory
	directoryTreeMetadata := make(map[string]MetaHeader)

	// Loop over directories in path and retrieve metadata information that is different than linux default
	directory := filepath.Dir(targetFilePath)
	for i := 0; i < maxDirectoryLoopCount; i++ {
		// Break if no more parent dirs
		if directory == "." || len(directory) < 2 {
			break
		}

		printMessage(verbosityProgress, "  File '%s': Retrieving metadata for parent directory '%s'\n", targetFilePath, directory)

		// Retrieve metadata
		command := "ls -ld " + directory
		var directoryMetadata string
		directoryMetadata, err = runSSHCommand(client, command, "root", config.disableSudo, SudoPassword, 10)
		if err != nil {
			err = fmt.Errorf("ssh command failure: %v", err)
			return
		}

		printMessage(verbosityProgress, "  File '%s': Parsing metadata for parent directory '%s'\n", targetFilePath, directory)

		// Extract ls information
		fileType, permissionsSymbolic, owner, group, _, _, lerr := extractMetadataFromLS(directoryMetadata)
		if lerr != nil {
			return
		}
		if fileType != "d" {
			printMessage(verbosityProgress, "Warning: expected remote path to be directory, but got type '%s' instead", fileType)
			continue
		}

		// Metadata
		var dirMetadata MetaHeader
		dirMetadata.TargetFileOwnerGroup = owner + ":" + group
		dirMetadata.TargetFilePermissions = permissionsSymbolicToNumeric(permissionsSymbolic)

		printMessage(verbosityData, "  File '%s': Metadata for parent directory '%s': %s %s\n", targetFilePath, directory, permissionsSymbolic, dirMetadata.TargetFileOwnerGroup)

		// Save metadata to map if not the default
		if dirMetadata.TargetFileOwnerGroup != defaultOwnerGroup || dirMetadata.TargetFilePermissions != defaultPermissions {
			printMessage(verbosityProgress, "  File '%s': Parent directory '%s' has non-standard metadata, saving\n", targetFilePath, directory)
			directoryTreeMetadata[directory] = dirMetadata
		}

		// Move up one directory for next loop iteration
		directory = filepath.Dir(directory)
	}

	printMessage(verbosityProgress, "  File '%s': Downloading file\n", targetFilePath)

	// Copy desired file to buffer location
	command := "cp " + targetFilePath + " " + tmpRemoteFilePath
	_, err = runSSHCommand(client, command, "", config.disableSudo, SudoPassword, 20)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	// Ensure buffer file can be read and then deleted later
	command = "chmod 666 " + tmpRemoteFilePath
	_, err = runSSHCommand(client, command, "", config.disableSudo, SudoPassword, 10)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	// Download remote file contents
	fileContents, err := SCPDownload(client, tmpRemoteFilePath)
	if err != nil {
		return
	}

	printMessage(verbosityProgress, "  File '%s': Parsing metadata information\n", targetFilePath)

	// Replace target path separators with local os ones
	hostFilePath := strings.ReplaceAll(targetFilePath, "/", config.osPathSeparator)

	// Use target file path and hosts name for repo file location
	filePath := endpointName + hostFilePath

	// Convert permissions string to number format
	numberPermissions := permissionsSymbolicToNumeric(fileInfo[0])

	// Put metadata into JSON format
	var metadataHeader MetaHeader
	metadataHeader.TargetFileOwnerGroup = fileInfo[1] + ":" + fileInfo[2]
	metadataHeader.TargetFilePermissions = numberPermissions

	printMessage(verbosityProgress, "  File '%s': Retrieving reload command information from user\n", targetFilePath)

	// Ask user for confirmation to use reloads
	reloadWanted, err := promptUser("Does file '%s' need reload commands? [y/N]: ", filePath)
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

	// Check what type of data is in retrieve file
	fileIsPlainText := isText(fileContents)

	// Make file depending on if plain text or binary
	if !fileIsPlainText {
		var userResponse string
		printMessage(verbosityStandard, "  File is not plain text, it should probably be stored outside of git\n")
		fmt.Print("  Specify a directory path where the actual file should be stored or enter nothing to store file directly in repository\n")
		fmt.Print("Path to External Directory: ")
		fmt.Scanln(&userResponse)

		// Add user specified directory to artifact path header field
		if userResponse != "" {
			// Combine remote file name with user supplied local path
			artifactFilePath := filepath.Join(userResponse, filepath.Base(filePath))

			// Clean up user supplied path
			artifactFilePath, err = filepath.Abs(artifactFilePath)
			if err != nil {
				return
			}

			// Store real file path in git-tracked file (set URI prefix)
			metadataHeader.ExternalContentLocation = fileURIPrefix + artifactFilePath

			// Ensure all parent directories exist for the file
			err = os.MkdirAll(filepath.Dir(artifactFilePath), 0750)
			if err != nil {
				return
			}

			// Create/Truncate artifact file
			var artifactFile *os.File
			artifactFile, err = os.OpenFile(artifactFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return
			}
			defer artifactFile.Close()

			// Write artifact file contents to the file
			_, err = artifactFile.WriteString(fileContents)
			if err != nil {
				return
			}

			// Correct file name of pointer to be stored in repo
			filePath = filePath + artifactPointerFileExtension

			// Ensure fileContents are not written into repository
			fileContents = ""
		}
	}

	printMessage(verbosityProgress, "Adding JSON metadata header to file '%s'\n", filePath)

	// Marshal metadata JSON
	metadata, errNoFatal := json.MarshalIndent(metadataHeader, "", "  ")
	if errNoFatal != nil {
		printMessage(verbosityStandard, "Failed to marshal metadata header into JSON format for file %s: %v\n", filePath, errNoFatal)
		return
	}

	// Add header to file contents
	file := metaDelimiter + "\n" + string(metadata) + "\n" + metaDelimiter + "\n" + fileContents

	printMessage(verbosityProgress, "Writing file '%s' to repository\n", filePath)

	// Create any missing directories in repository
	configParentDirs := filepath.Dir(filePath)
	errNoFatal = os.MkdirAll(configParentDirs, 0700)
	if errNoFatal != nil {
		printMessage(verbosityStandard, "Failed to create missing directories in local repository for file '%s': %v\n", filePath, errNoFatal)
		return
	}

	// Write config to file in repository
	errNoFatal = os.WriteFile(filePath, []byte(file), 0600)
	if errNoFatal != nil {
		printMessage(verbosityStandard, "Failed to write file '%s' to local repository: %v\n", filePath, errNoFatal)
		return
	}

	// Loop over parent directories and write any non-standard json directory metadata
	for directory, metadata := range directoryTreeMetadata {
		printMessage(verbosityProgress, "  File '%s': Writing metadata information for directory '%s'\n", targetFilePath, directory)

		// Prepare directory metadata file name
		directory = strings.ReplaceAll(directory, "/", config.osPathSeparator)
		directoryMetaPath := filepath.Join(directory, directoryMetadataFileName)
		directoryMetaPath = endpointName + directoryMetaPath

		// Marshall metadata json
		metadata, errNoFatal := json.MarshalIndent(metadata, "", "  ")
		if errNoFatal != nil {
			printMessage(verbosityStandard, "  Failed to marshal metadata header into JSON format for directory '%s': %v\n", directory, errNoFatal)
			continue
		}

		// Open/create the directory metadata file
		directoryMetaFile, errNoFatal := os.OpenFile(directoryMetaPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if errNoFatal != nil {
			printMessage(verbosityStandard, "  Failed to open/create directory metadata file: %v\n", errNoFatal)
			continue
		}
		defer directoryMetaFile.Close()

		// Write directory metadata file
		_, errNoFatal = directoryMetaFile.WriteString(string(metadata))
		if errNoFatal != nil {
			printMessage(verbosityStandard, "  Failed to write directory metadata to local repository: %v\n", errNoFatal)
			continue
		}
	}

	return
}

// isText checks if a string is likely plain text or binary data based on the first 500 bytes
func isText(inputString string) (isPlainText bool) {

	// Check for non-printable characters in the first 500 bytes
	nonPrintableCount := 0
	for _, r := range inputString {
		// Check for control characters or characters outside the printable ASCII range
		if r < 32 || r > 126 {
			nonPrintableCount++
		}
	}

	// Consider the string as plain text if less than 30% are non-printable characters
	// Adjust the threshold as needed
	if float64(nonPrintableCount)/float64(len(inputString)) < 0.3 {
		isPlainText = true
		return
	}
	isPlainText = false
	return
}
