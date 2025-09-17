package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

func entryMetaHeader(commandname string, args []string) {
	var editInPlace bool
	var inputMetadata string
	var compactJSONMode bool

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	commandFlags.BoolVar(&editInPlace, "i", false, "Modify file in-place")
	commandFlags.BoolVar(&editInPlace, "in-place", false, "Modify file in-place")
	commandFlags.StringVar(&inputMetadata, "j", "", "Use provided metadata JSON ('-' to read it from stdin)")
	commandFlags.StringVar(&inputMetadata, "json-metadata", "", "Use provided metadata JSON ('-' to read it from stdin)")
	commandFlags.BoolVar(&compactJSONMode, "C", false, "Print JSON headers in single-line format")
	commandFlags.BoolVar(&compactJSONMode, "compact", false, "Print JSON headers in single-line format")
	setGlobalArguments(commandFlags)

	commandFlags.Usage = func() {
		printHelpMenu(commandFlags, commandname, allCmdOpts)
	}
	if len(args) < 1 {
		printHelpMenu(commandFlags, commandname, allCmdOpts)
		os.Exit(1)
	}
	commandFlags.Parse(args[1:])

	remainingArgs := commandFlags.Args()

	switch args[0] {
	case "edit":
		if len(remainingArgs) < 1 {
			printHelpMenu(commandFlags, args[0], allCmdOpts)
			os.Exit(1)
		}

		modifyHeader(remainingArgs[0], inputMetadata, editInPlace)
	case "strip":
		if len(remainingArgs) < 1 {
			printHelpMenu(commandFlags, args[0], allCmdOpts)
			os.Exit(1)
		}

		stripHeader(remainingArgs[0], editInPlace)
	case "insert":
		if len(remainingArgs) < 1 {
			printHelpMenu(commandFlags, args[0], allCmdOpts)
			os.Exit(1)
		}

		addHeaderToExistingFile(remainingArgs[0], inputMetadata, editInPlace)
	case "read":
		if len(remainingArgs) < 1 {
			printHelpMenu(commandFlags, args[0], allCmdOpts)
			os.Exit(1)
		}

		printFileHeader(remainingArgs[0], compactJSONMode)
	case "verify":
		if len(remainingArgs) < 1 {
			printHelpMenu(commandFlags, args[0], allCmdOpts)
			os.Exit(1)
		}

		verifyHeader(remainingArgs[0])
	default:
		printHelpMenu(commandFlags, commandname, allCmdOpts)
		os.Exit(1)
	}
}

// Extracts and validates existing metadata headers (including JSON syntax) in files
func verifyHeader(fileInput string) {
	fileInput, err := retrieveURIFile(fileInput)
	logError("Failed to read file contents for verification input files", err, false)

	var files []string
	if strings.Contains(fileInput, ",") {
		files = strings.Split(fileInput, ",")
	} else {
		files = append(files, fileInput)
	}

	for _, filePath := range files {
		inputFileContents, err := os.ReadFile(filePath)
		logError(fmt.Sprintf("Failed to read contents of specified file '%s'", filePath), err, false)

		// Ignoring all outputs, just checking to make sure it works
		_, _, err = extractMetadata(string(inputFileContents))
		logError(fmt.Sprintf("Failed to extract contents from the specified file '%s'", filePath), err, false)

		printMessage(verbosityStandard, "Metadata header in '%s' is valid\n", filePath)
	}
}

// Removes header from file to get just the contents
// Prints to stdout or writes back to file
func stripHeader(filePath string, editInPlace bool) {
	// Pull file contents and grab just the data
	inputFileContents, err := os.ReadFile(filePath)
	logError(fmt.Sprintf("Failed to read contents of specified file '%s'", filePath), err, false)

	_, ouputFileContents, err := extractMetadata(string(inputFileContents))
	logError(fmt.Sprintf("Failed to extract contents from the specified file '%s'", filePath), err, false)

	// Write back or straight to stdout
	if editInPlace {
		err = os.WriteFile(filePath, ouputFileContents, 0600)
		logError("Failed to write stripped contents to existing file", err, false)
	} else {
		printMessage(verbosityStandard, "%s", string(ouputFileContents))
	}
}

// Extracts metadata header from file
// Prints to stdout or writes back to file
func printFileHeader(filePath string, compactJSONMode bool) {
	file, err := os.ReadFile(filePath)
	logError(fmt.Sprintf("Failed to read file '%s'", filePath), err, false)

	metadata, _, err := extractMetadata(string(file))
	logError(fmt.Sprintf("Failed to read header from file '%s'", filePath), err, false)

	var header []byte
	if compactJSONMode {
		header, err = json.Marshal(metadata)
	} else {
		header, err = json.MarshalIndent(metadata, "", "  ")
	}
	logError(fmt.Sprintf("Failed to parse header from file '%s'", filePath), err, false)

	header = unescapeShellRedirectors(header)

	printMessage(verbosityStandard, "%s\n", string(header))
}

func modifyHeader(filePath string, input string, editInPlace bool) {
	inputFileContents, err := os.ReadFile(filePath)
	logError(fmt.Sprintf("Failed to read contents of specified file '%s'", filePath), err, false)

	oldHeader, fileContents, err := extractMetadata(string(inputFileContents))
	logError(fmt.Sprintf("Failed to extract contents from the specified file '%s'", filePath), err, false)

	// User controls when we read the JSON from stdin via special flag
	var newHeader MetaHeader
	if input != "" {
		newHeader, err = getUserMetaHeaderInput(input)
		logError("Failed to retrieve metadata input", err, false)
	} else {
		newHeader, err = headerEditor(oldHeader, filePath)
		logError("Failed to run interactive header editor", err, false)
	}

	if editInPlace {
		err = writeLocalRepoFile(filePath, newHeader, &fileContents)
		logError(fmt.Sprintf("Failed to write modified header to existing file '%s'", filePath), err, false)
	} else {
		metaHeaderBytes, err := json.MarshalIndent(newHeader, "", "  ")
		logError("Failed to create new header", err, false)

		metaHeaderBytes = unescapeShellRedirectors(metaHeaderBytes)
		header := string(metaHeaderBytes)

		var fullFileContent strings.Builder
		fullFileContent.WriteString(metaDelimiter)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(header)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(metaDelimiter)
		fullFileContent.WriteString("\n")
		if fileContents != nil {
			fullFileContent.Write(fileContents)
		}

		printMessage(verbosityStandard, "%s", fullFileContent.String())
	}
}

func addHeaderToExistingFile(filePath string, input string, editInPlace bool) {
	// Pull file contents and grab just the data
	existingFileContents, err := os.ReadFile(filePath)
	logError(fmt.Sprintf("Failed to read contents of specified file '%s'", filePath), err, false)

	// Use extraction function as canary to determine if file has a header
	_, _, err = extractMetadata(string(existingFileContents))
	if err == nil {
		logError("Existing metadata header detected in file '%s'", fmt.Errorf("cannot overwrite headers with add subcommand, please use modify subcommand to change headers"), false)
	} else {
		err = nil
	}

	// User controls when we read the JSON from stdin via special flag
	var inputHeader MetaHeader
	if input != "" {
		inputHeader, err = getUserMetaHeaderInput(input)
		logError("Failed to retrieve metadata input", err, false)
	} else {
		inputHeader, err = headerEditor(inputHeader, filePath)
		logError("Failed to run interactive header editor", err, false)
	}

	// Write back or straight to stdout
	if editInPlace {
		err := writeLocalRepoFile(filePath, inputHeader, &existingFileContents)
		logError("Failed to write header to existing file", err, false)
	} else {
		metaHeaderBytes, err := json.MarshalIndent(inputHeader, "", "  ")
		logError("Failed to create new header", err, false)

		metaHeaderBytes = unescapeShellRedirectors(metaHeaderBytes)
		header := string(metaHeaderBytes)

		var fullFileContent strings.Builder
		fullFileContent.WriteString(metaDelimiter)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(header)
		fullFileContent.WriteString("\n")
		fullFileContent.WriteString(metaDelimiter)
		fullFileContent.WriteString("\n")
		if existingFileContents != nil {
			fullFileContent.Write(existingFileContents)
		}

		printMessage(verbosityStandard, "%s", fullFileContent.String())
	}
}

// Command line interactive editor for metadata JSON header
func headerEditor(initialHeader MetaHeader, fileName string) (modifiedHeader MetaHeader, err error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		err = fmt.Errorf("not in a terminal, cannot interactively edit header")
		return
	}

	reader := bufio.NewReader(os.Stdin)
	header := initialHeader

	for {
		menuOptions := []string{
			"===============================",
			fmt.Sprintf("1  TargetFileOwnerGroup      : %s", header.TargetFileOwnerGroup),
			fmt.Sprintf("2  TargetFilePermissions     : %d", header.TargetFilePermissions),
			fmt.Sprintf("3  ExternalContentLocation   : %s", header.ExternalContentLocation),
			fmt.Sprintf("4  SymbolicLinkTarget        : %s", header.SymbolicLinkTarget),
			fmt.Sprintf("5  Dependencies              : %v", header.Dependencies),
			fmt.Sprintf("6  PreDeployCommands         : %v", header.PreDeployCommands),
			fmt.Sprintf("7  InstallCommands           : %v", header.InstallCommands),
			fmt.Sprintf("8  CheckCommands             : %v", header.CheckCommands),
			fmt.Sprintf("9  ReloadCommands            : %v", header.ReloadCommands),
			fmt.Sprintf("10 ReloadGroup               : %s", header.ReloadGroup),
			"===============================",
			"Selection  Delete Field  Exit",
			" [ # ## ]      [ - ]     [ ! ]",
		}
		var linesPrinted int
		for _, menuOption := range menuOptions {
			fmt.Println(menuOption)
			linesPrinted++
		}

		var choice string
		fmt.Printf("File:%s # Type your selection: ", fileName)
		choice, err = reader.ReadString('\n')
		if err != nil {
			err = fmt.Errorf("failed to read user input: %v", err)
			return
		}
		choice = strings.TrimSpace(choice)
		linesPrinted++

		clearLines(linesPrinted)

		switch choice {
		case "!":
			modifiedHeader = header
			return
		case "":
			modifiedHeader = header
			return // No input exits
		case "1":
			header.TargetFileOwnerGroup = promptString(reader, header.TargetFileOwnerGroup, "Enter new ownership")
		case "2":
			header.TargetFilePermissions = promptInt(reader, header.TargetFilePermissions, "Enter new permissions")
		case "3":
			header.ExternalContentLocation = promptString(reader, header.ExternalContentLocation, "Enter new external content location")
		case "4":
			header.SymbolicLinkTarget = promptString(reader, header.SymbolicLinkTarget, "Enter new SymbolicLinkTarget")
		case "5":
			header.Dependencies = editStringSlice(reader, header.Dependencies, "Dependencies")
		case "6":
			header.PreDeployCommands = editStringSlice(reader, header.PreDeployCommands, "PreDeployCommands")
		case "7":
			header.InstallCommands = editStringSlice(reader, header.InstallCommands, "InstallCommands")
		case "8":
			header.CheckCommands = editStringSlice(reader, header.CheckCommands, "CheckCommands")
		case "9":
			header.ReloadCommands = editStringSlice(reader, header.ReloadCommands, "ReloadCommands")
		case "10":
			header.ReloadGroup = promptString(reader, header.ReloadGroup, "Enter new ReloadGroup")
		default:
			fmt.Println("Invalid choice.")
			waitForEnter(reader)
			clearLines(1)
		}
		clearLines(1)
	}
}

// Dedicated editing menu for arrays
func editStringSlice(reader *bufio.Reader, itemList []string, name string) (modifiedList []string) {
	for {
		var linesPrinted int
		fmt.Printf("==== %s ====\n", name)
		linesPrinted++
		for index, item := range itemList {
			fmt.Printf("%d  %s\n", index+1, item)
			linesPrinted++
		}

		menuOptions := []string{
			fmt.Sprintf("=====%s=====", strings.Repeat("=", len(name))),
			" Add   Edit   Remove  Back",
			"[ a ] [ e# ]  [ r# ]  [ ! ]",
		}
		for _, menuOption := range menuOptions {
			fmt.Println(menuOption)
			linesPrinted++
		}

		fmt.Print("# Type your selection: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		clearLines(linesPrinted)

		if choice == "" {
			modifiedList = itemList
			return
		}

		switch string(choice[0]) {
		case "a":
			clearLines(1)
			item := promptString(reader, "", "Enter new item")
			itemList = append(itemList, item)
			clearLines(1)
		case "e":
			editIndex, invalid := validateUserIndexChoice(choice, itemList, reader)
			if invalid {
				continue
			}
			clearLines(1)
			newItem := promptString(reader, itemList[editIndex-1], "Enter new value")
			itemList[editIndex-1] = newItem
			clearLines(1)
		case "r":
			removeIndex, invalid := validateUserIndexChoice(choice, itemList, reader)
			if invalid {
				continue
			}
			itemList = append(itemList[:removeIndex-1], itemList[removeIndex:]...)
			clearLines(1)
		case "!":
			modifiedList = itemList
			return
		default:
			fmt.Printf("Unknown option %s\n", choice)
			waitForEnter(reader)
			clearLines(2)
		}
	}
}

// Validates user chosen index for given array
func validateUserIndexChoice(userChoice string, itemList []string, reader *bufio.Reader) (index int, invalid bool) {
	if len(itemList) == 0 {
		fmt.Println("List is empty")
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	if len(userChoice) < 2 {
		fmt.Println("Must include number following 'r'")
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	chosenIndex, err := strconv.Atoi(string(userChoice[1:]))
	if err != nil {
		fmt.Printf("Invalid numeric choice: %v\n", err)
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	if chosenIndex < 1 || chosenIndex > len(itemList) {
		fmt.Println("Choice out of range for list length")
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	index = chosenIndex
	return
}

// Prompt user for new string value
// "-"  input returns empty string
// Empty input returns currentValue
func promptString(reader *bufio.Reader, currentValue string, prompt string) (userInput string) {
	if currentValue != "" {
		fmt.Print("[Current: " + currentValue + "] " + prompt + ": ")
	} else {
		fmt.Print(prompt + ": ")
	}
	text, _ := reader.ReadString('\n')
	userInput = strings.TrimSpace(text)

	switch userInput {
	case "":
		// Entering nothing uses existing value
		userInput = currentValue
	case "-":
		// Special character to actually delete field value
		userInput = ""
	}
	return
}

// Prompt user for new number
// "-"  input returns empty string
// Empty input returns currentValue
func promptInt(reader *bufio.Reader, currentValue int, prompt string) (userInput int) {
	currentValueText := strconv.Itoa(currentValue)
	for {
		if currentValue != 0 {
			fmt.Print("[Current: " + currentValueText + "] " + prompt + ": ")
		} else {
			fmt.Print(prompt + ": ")
		}
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		switch text {
		case "":
			// Entering nothing uses existing value
			text = currentValueText
		case "-":
			// Special character to actually delete field value
			text = "0"
		}

		num, err := strconv.Atoi(text)
		if err == nil {
			userInput = num
			return
		}
		fmt.Println("Invalid number, try again.")
	}
}

// Blocks until newline is encountered (from user)
func waitForEnter(reader *bufio.Reader) {
	fmt.Print("Press Enter to continue...")
	reader.ReadString('\n')
}

// Clear lines (no scrolling menu) for low verbosity
func clearLines(maxLines int) {
	if globalVerbosityLevel < 2 {
		for i := 0; i < maxLines; i++ {
			fmt.Print("\033[1A") // move cursor up
			fmt.Print("\033[2K") // clear entire line
		}
	}
}
