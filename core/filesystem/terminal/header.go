package terminal

import (
	"bufio"
	"fmt"
	"os"
	"scmp/core/filesystem"
	"scmp/internal/str"
	"strings"

	"golang.org/x/term"
)

// Command line interactive editor for metadata JSON header
func HeaderEditor(initialHeader filesystem.MetaHeader, fileName str.LocalRepoPath) (modifiedHeader filesystem.MetaHeader, err error) {
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
			fmt.Sprintf("8  PostInstallCommands       : %v", header.PostInstallCommands),
			fmt.Sprintf("9  PreapplyCommands          : %v", header.PreapplyCommands),
			fmt.Sprintf("10 PostapplyCommands         : %v", header.PostapplyCommands),
			fmt.Sprintf("11 ReloadCommands            : %v", header.ReloadCommands),
			fmt.Sprintf("12 ReloadGroup               : %s", header.ReloadGroup),
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
			err = fmt.Errorf("failed to read user input: %w", err)
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
			header.SymbolicLinkTarget = str.RemotePath(promptString(reader, string(header.SymbolicLinkTarget), "Enter new SymbolicLinkTarget"))
		case "5":
			gotDeps := editStringSlice(reader, str.ToStrings(header.Dependencies), "Dependencies")
			header.Dependencies = str.FromStrings[str.LocalRepoPath](gotDeps)
		case "6":
			header.PreDeployCommands = editStringSlice(reader, header.PreDeployCommands, "PreDeployCommands")
		case "7":
			header.InstallCommands = editStringSlice(reader, header.InstallCommands, "InstallCommands")
		case "8":
			header.PostInstallCommands = editStringSlice(reader, header.PostInstallCommands, "PostInstallCommands")
		case "9":
			header.PreapplyCommands = editStringSlice(reader, header.PreapplyCommands, "PreapplyCommands")
		case "10":
			header.PostapplyCommands = editStringSlice(reader, header.PostapplyCommands, "PostapplyCommands")
		case "11":
			header.ReloadCommands = editStringSlice(reader, header.ReloadCommands, "ReloadCommands")
		case "12":
			header.ReloadGroup = str.ReloadID(promptString(reader, string(header.ReloadGroup), "Enter new ReloadGroup"))
		default:
			fmt.Println("Invalid choice.")
			waitForEnter(reader)
			clearLines(1)
		}
		clearLines(1)
	}
}
