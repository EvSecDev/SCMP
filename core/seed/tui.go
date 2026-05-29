package seed

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strconv"
	"strings"
)

// Runs the CLI-based menu that user will use to select which files to download
func interactiveSelection(ctx context.Context, host sshinternal.HostMeta) (selectedFiles []string, err error) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	logger := logctx.GetLogger(ctx)
	logVerbosityLevel := logger.PrintLevel

	// Start selection at root of filesystem - '/'
	var directoryState DirectoryState
	directoryState.current = "/"
	directoryState.stack = []string{"/"}

	// Loop until user is done selecting
	for {
		// Get file names and info for the directory
		command := sshinternal.BuildLsList(str.RemotePath(directoryState.current))
		command.DisableSudo = opts.DisableSudo
		command.RunAsUser = opts.RunAsUser

		var directoryList string
		directoryList, err = command.SSHexec(ctx, host.SSHClient, host.Password)
		if err != nil {
			// All errors except permission denied exits selection menu
			if !strings.Contains(err.Error(), "Permission denied") {
				return
			}

			// Exit menu if it failed reading the first directory after ssh connection (i.e. "/")
			if directoryState.current == "/" {
				err = fmt.Errorf("permission denied when reading '/'")
				return
			}

			// Show progress to user
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to read '%s'\n", directoryState.current)

			// Set next loop directory to parent directory
			directoryState.current = directoryState.stack[len(directoryState.stack)-2]

			// Remove current directory from the stack
			directoryState.stack = directoryState.stack[:len(directoryState.stack)-1]
			continue
		}

		// Extract info from ls directory listing
		dirList, maxNameLenght := parseDirEntries(directoryList)

		// Show Menu - Print the directory contents in columns
		userSelections := dirListMenu(string(host.Name), maxNameLenght, dirList, directoryState.current, logVerbosityLevel)

		// Parse users selections
		var userRequestedExit bool
		var dirSelectedFiles []string
		userRequestedExit, dirSelectedFiles, directoryState = parseUserSelections(ctx, userSelections, dirList, directoryState, host)
		selectedFiles = append(selectedFiles, dirSelectedFiles...)
		if userRequestedExit {
			// Stop selecting
			break
		}
	}

	return
}

// Prints out table-like menu for a directory listing
// Prompts the user to supply their choices of files/directories and returns array of choices (in user chosen order)
func dirListMenu(endpointName string, maxNameLenght int, dirList []string, currentDirectory string, logVerbosityLevel int) (userSelections []string) {
	// Menu (Table) sizing
	const numberOfColumns int = 4
	numberOfDirEntries := len(dirList)
	maxRows := (numberOfDirEntries + numberOfColumns - 1) / numberOfColumns
	columnWidth := maxNameLenght + 4

	// Populate table items
	fmt.Printf("============================================================\n")
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
	fmt.Printf("============================================================\n")
	fmt.Printf("     Select File     Change Dir ^/v   Recursive   Exit\n")
	fmt.Printf("     [ # # ## ### ]  [ c0 ]  [ c# ]    [ #r ]     [ ! ]\n")
	fmt.Printf("%s:%s # Type your selections: ", endpointName, currentDirectory)

	reader := bufio.NewReader(os.Stdin)
	userInput, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("\nWarning: could not read user input\n")
	}

	// Split input into individual selections separated by spaces
	userSelections = strings.Fields(userInput)

	if len(userSelections) == 0 {
		fmt.Printf("\nDid not receive any selections!\n")
	}

	// Clear menu rows - add to row count to account for the prompts (only for standard verbosity)
	if logVerbosityLevel < 2 {
		maxRows += 5
		for maxRows > 0 {
			fmt.Printf("\033[A\033[K")
			maxRows--
		}
	}

	return
}

// Takes user selection array and parses options
// Handles saving file/director choices, changing directories, and exiting selection
func parseUserSelections(ctx context.Context, userSelections []string, dirList []string, directoryState DirectoryState, host sshinternal.HostMeta) (userRequestedExit bool, selectedFiles []string, directoryStateNew DirectoryState) {
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Sync current directory state to return value
	directoryStateNew = directoryState

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "\nParsing Selections for Current Directory: '%s'\n", directoryState.current)

	for _, selection := range userSelections {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Selection: '%s'\n", selection)

		dirIndex, err := strconv.Atoi(selection)

		if selection == "!" { // Exit menu only after processing selections

			userRequestedExit = true
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Requested exit: will exit selections after parsing current selection\n")

		} else if strings.HasSuffix(selection, "r") { // Recurse directory and grab all files

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Requested recursive selection\n")

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

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Recursing into directory '%s' for all files\n", absolutePath)

			command := sshinternal.RemoteCommand{
				Raw:          "find '" + absolutePath + "' -type f",
				RunAsUser:    opts.RunAsUser,
				DisableSudo:  opts.DisableSudo,
				Timeout:      opts.ExecutionTimeout,
				StreamStdout: false,
			}
			findOutput, err := command.SSHexec(ctx, host.SSHClient, host.Password)
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

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Requested directory change\n")

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

				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    Moving up from '%s' to '%s'\n", directoryState.current, directoryStateNew.current)

				// Remove current directory from the stack
				directoryStateNew.stack = directoryState.stack[:len(directoryState.stack)-1]
			} else if dirIndex >= 1 && dirIndex <= len(dirList) {
				// If selection is not directory, don't cd into anything
				name := dirList[dirIndex-1] // Get file name from user selection number
				if !strings.HasSuffix(name, "/") {
					continue
				}

				// Set next loop directory to chosen dir
				directoryStateNew.current = filepath.Join(directoryState.current, dirList[dirIndex-1])

				logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    Moving down from '%s' to '%s'\n", directoryState.current, directoryStateNew.current)

				// Add chosen dir to the stack
				directoryStateNew.stack = append(directoryState.stack, directoryState.current)
			}
		} else if err == nil && dirIndex > 0 && dirIndex <= len(dirList) { // Select file by number

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Requested individual item\n")

			// Get file name from user selection number
			name := dirList[dirIndex-1]

			absolutePath := filepath.Join(directoryState.current, name)

			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    Marking item '%s' for retrieval\n", absolutePath)
			selectedFiles = append(selectedFiles, absolutePath)
		} else {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Warning: unknown option '%s'\n", selection)
		}
	}

	return
}
