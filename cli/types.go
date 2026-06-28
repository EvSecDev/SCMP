// Package for dynamically building subcommand help menus
package cli

import "context"

const RootCLICommand string = "root"

type CommandSet struct {
	CommandName string // Exact name of cli command
	PrimaryFunc func(ctx context.Context,
		subcmdLineage []string,
		args []string,
	) (exitCode int) // Function to call for top level command only (i.e. direct children of root only)
	UsageOption     string                 // Expected command value in usage top line
	Description     string                 // Short text displayed on parent command
	FullDescription string                 // Long text displayed on current command
	ChildCommands   map[string]*CommandSet // Available subcommands
}
