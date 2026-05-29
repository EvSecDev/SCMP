package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/core/filesystem/content"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
)

func File(ctx context.Context, commandname string, args []string) {
	var userConfirmed bool
	var opts config.Opts

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	commandFlags.BoolVar(&userConfirmed, "y", false, "Confirm file overwrites")
	commandFlags.BoolVar(&userConfirmed, "yes", false, "Confirm file overwrites")
	globalVerbosity := cli.SetGlobalArguments(commandFlags, &opts)

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
	remainingArgs := commandFlags.Args()

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, *globalVerbosity)

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, opts)

	invalidArgs := fileSetup(ctx, args[0], remainingArgs, userConfirmed)
	if invalidArgs {
		cli.PrintHelpMenu(commandFlags, args[0], cli.GetCLICmds())
		os.Exit(1)
	}
}

func fileSetup(ctx context.Context, subcommand string, remainingArgs []string, userConfirmed bool) (invalidArgs bool) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSFiles)

	switch subcommand {
	case "new":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			return
		}

		content.WriteTemplateFile(ctx, str.LocalRepoPath(remainingArgs[0]), userConfirmed)
	case "replace-data":
		if len(remainingArgs) < 2 {
			invalidArgs = true
			return
		}

		srcFile := str.LocalRepoPath(remainingArgs[0])
		dstFile := str.LocalRepoPath(remainingArgs[1])
		content.ReplaceData(ctx, srcFile, dstFile, userConfirmed)
	default:
		invalidArgs = true
		return
	}
	return
}
