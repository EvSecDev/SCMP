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

func Git(ctx context.Context, commandname string, args []string) {
	var commitMessage string
	var globalVerbosity int

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	commandFlags.StringVar(&commitMessage, "m", "", "Commit message")
	commandFlags.StringVar(&commitMessage, "message", "", "Commit message")
	commandFlags.IntVar(&globalVerbosity, "v", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")
	commandFlags.IntVar(&globalVerbosity, "verbosity", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")

	commandFlags.Usage = func() {
		cli.PrintHelpMenu(commandFlags, commandname, cli.GetCLICmds())
	}
	if len(args) < 1 {
		cli.PrintHelpMenu(commandFlags, commandname, cli.GetCLICmds())
		os.Exit(1)
	}
	err := commandFlags.Parse(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, globalVerbosity)

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, config.Opts{DryRunEnabled: false})

	subcommand := args[0]

	invalidArgs, err := gitinternal.CLIEntry(ctx, subcommand, args, commitMessage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if invalidArgs {
		cli.PrintHelpMenu(commandFlags, subcommand, cli.GetCLICmds())
		os.Exit(1)
	}
}
