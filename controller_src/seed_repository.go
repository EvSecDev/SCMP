// controller
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ###################################
//  SEED REPO FILES FUNCTIONS
// ###################################

// Entry point for user to select remote files to download and format into local repository
func seedRepositoryFiles(config Config, hostOverride string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			logError("Controller panic while seeding repository files", fmt.Errorf("%v", fatalError), false)
		}
	}()

	fmt.Printf("==== Secure Configuration Management Repository Seeding ====\n")

	// Check local system
	err := localSystemChecks()
	logError("Error in system checks", err, false)

	// Loop hosts in config and prepare relevant host information for deployment
	for endpointName, endpointInfo := range config.DeployerEndpoints {
		// Use hosts user specifies if requested
		SkipHost := checkForHostOverride(hostOverride, endpointName)
		if SkipHost {
			continue
		}

		// Extract vars for endpoint information
		var info EndpointInfo
		info, err = retrieveEndpointInfo(endpointInfo, config.SSHClientDefault)
		logError("Failed to retrieve endpoint information", err, false)

		// Connect to the SSH server
		client, err := connectToSSH(info.Endpoint, info.EndpointUser, info.PrivateKey, info.KeyAlgo)
		logError("Failed connect to SSH server", err, false)
		defer client.Close()

		// Run menu for user to select desired files
		selectedFiles, err := runSelectionMenu(endpointName, client, info.SudoPassword)
		logError("Error retrieving remote file list", err, false)

		// Initialize buffer file (with random byte) - ensures ownership of buffer stays correct when retrieving remote files
		err = RunSFTP(client, []byte{12}, info.RemoteTransferBuffer)
		logError(fmt.Sprintf("Failed to initialize buffer file on remote host %s", endpointName), err, false)

		// Download user file choices to local repo and format
		for targetFilePath, fileInfo := range selectedFiles {
			err = retrieveSelectedFile(targetFilePath, fileInfo, endpointName, client, info.SudoPassword, info.RemoteTransferBuffer)
			logError("Error seeding repository", err, false)
		}
	}

	fmt.Printf("============================================================\n")
}

// Runs the CLI-based menu that user will use to select which files to download
func runSelectionMenu(endpointName string, client *ssh.Client, SudoPassword string) (selectedFiles map[string][]string, err error) {
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
		directoryList, err = RunSSHCommand(client, command, SudoPassword)
		if err != nil {
			// All errors except permission denied exits selection menu
			if !strings.Contains(err.Error(), "Permission denied") {
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
			fmt.Println()
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
			fmt.Print("\033[A\033[K")
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
	// Copy desired file to buffer location - MUST keep buffer file permissions for successful sftp
	command := "cp --no-preserve=mode,ownership " + targetFilePath + " " + tmpRemoteFilePath
	_, err = RunSSHCommand(client, command, SudoPassword)
	if err != nil {
		err = fmt.Errorf("ssh command failure: %v", err)
		return
	}

	// Open new session with ssh client
	var sftpClient *sftp.Client
	sftpClient, err = sftp.NewClient(client)
	if err != nil {
		err = fmt.Errorf("failed to create sftp session: %v", err)
		return
	}
	defer sftpClient.Close()

	// Open remote file
	var remoteFile *sftp.File
	remoteFile, err = sftpClient.Open(tmpRemoteFilePath)
	if err != nil {
		err = fmt.Errorf("failed to read tmp buffer file '%s': %v", tmpRemoteFilePath, err)
		return
	}

	// Download remote file contents
	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, remoteFile)
	if err != nil {
		err = fmt.Errorf("failed to download remote file from buffer file '%s': %v", tmpRemoteFilePath, err)
		return
	}

	// Convert recevied bytes to string
	fileContents := buffer.String()

	// Replace target path separators with local os ones
	hostFilePath := strings.ReplaceAll(targetFilePath, "/", OSPathSeparator)

	// Use target file path and hosts name for repo file location
	configFilePath := endpointName + hostFilePath

	// Convert permissions string to number format
	numberPermissions := permissionsSymbolicToNumeric(fileInfo[0])

	// Put metadata into JSON format
	var metadataHeader MetaHeader
	metadataHeader.TargetFileOwnerGroup = fileInfo[1] + ":" + fileInfo[2]
	metadataHeader.TargetFilePermissions = numberPermissions

	// Ask user for confirmation to use reloads
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Does file '%s' need reload commands? [y/N]: ", configFilePath)

	// Read user choice and format
	reloadWanted, _ := reader.ReadString('\n')
	reloadWanted = strings.TrimSpace(reloadWanted)
	reloadWanted = strings.ToLower(reloadWanted)

	// Setup metadata depending on user choice
	if reloadWanted == "y" {
		metadataHeader.ReloadRequired = true
		var reloadCmds []string

		// Get array of commands from user
		fmt.Printf("Enter reload commands (press Enter after each command, leave an empty line to finish):\n")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			cmd := scanner.Text()
			if cmd == "" { // Done once empty line
				// Get confirmation of input
				fmt.Printf("Are these commands correct? [Y/n]: ")
				userConfirmation, _ := reader.ReadString('\n')
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

		// Write user supplied command array to metadata header
		metadataHeader.ReloadCommands = reloadCmds
	} else {
		metadataHeader.ReloadRequired = false
	}

	// Marshal metadata JSON
	metadata, errNoFatal := json.MarshalIndent(metadataHeader, "", "  ")
	if errNoFatal != nil {
		fmt.Printf("Failed to marshal metadata header into JSON format for file %s\n", configFilePath)
		return
	}

	// Add header to file contents
	configFile := Delimiter + "\n" + string(metadata) + "\n" + Delimiter + "\n" + fileContents

	// Create any missing directories in repository
	configParentDirs := filepath.Dir(configFilePath)
	errNoFatal = os.MkdirAll(configParentDirs, os.ModePerm)
	if errNoFatal != nil {
		fmt.Printf("Failed to create missing directories in local repository for file '%s'\n", configFilePath)
		return
	}

	// Write config to file in repository
	errNoFatal = os.WriteFile(configFilePath, []byte(configFile), 0640)
	if errNoFatal != nil {
		fmt.Printf("Failed to write file '%s' to local repository\n", configFilePath)
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
