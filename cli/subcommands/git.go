package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/internal/config"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
)

func Git(ctx context.Context, subcmdLineage []string, args []string) (exitCode int) {
	var commitMessage string
	var globalVerbosity int

	commandFlags := flag.NewFlagSet(subcmdLineage[len(subcmdLineage)-1], flag.ExitOnError)
	commandFlags.StringVar(&commitMessage, "m", "", "Commit message")
	commandFlags.StringVar(&commitMessage, "message", "", "Commit message")
	commandFlags.IntVar(&globalVerbosity, "v", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")
	commandFlags.IntVar(&globalVerbosity, "verbosity", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")

	commandFlags.Usage = func() {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
	}
	if len(args) < 1 {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		return 1
	}
	err := commandFlags.Parse(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, globalVerbosity)

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, config.Opts{DryRunEnabled: false})

	subcommand := args[0]

	invalidArgs, err := gitinternal.CLIEntry(ctx, subcommand, args, commitMessage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if invalidArgs {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		return 1
	}
	return 0
}
