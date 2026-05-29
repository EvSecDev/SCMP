package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/core/filesystem/header"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
)

func Header(ctx context.Context, commandname string, args []string) {
	var editInPlace bool
	var inputMetadata string
	var compactJSONMode bool
	var opts config.Opts

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	commandFlags.BoolVar(&editInPlace, "i", false, "Modify file in-place")
	commandFlags.BoolVar(&editInPlace, "in-place", false, "Modify file in-place")
	commandFlags.StringVar(&inputMetadata, "j", "", "Use provided metadata JSON ('-' to read it from stdin)")
	commandFlags.StringVar(&inputMetadata, "json-metadata", "", "Use provided metadata JSON ('-' to read it from stdin)")
	commandFlags.BoolVar(&compactJSONMode, "C", false, "Print JSON headers in single-line format")
	commandFlags.BoolVar(&compactJSONMode, "compact", false, "Print JSON headers in single-line format")
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

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, opts)

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, *globalVerbosity)

	remainingArgs := commandFlags.Args()

	invalidArgs := headerSetup(ctx, args[0], remainingArgs, editInPlace, compactJSONMode, inputMetadata)
	if invalidArgs {
		cli.PrintHelpMenu(commandFlags, args[0], cli.GetCLICmds())
		os.Exit(1)
	}
}

func headerSetup(ctx context.Context, subcommand string, remainingArgs []string, editInPlace, compactJSONMode bool, inputMetadata string) (invalidArgs bool) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSFiles)

	path := str.LocalRepoPath(remainingArgs[0])

	switch subcommand {
	case "edit":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			return
		}

		header.Modify(ctx, path, inputMetadata, editInPlace)
	case "strip":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			return
		}

		header.Strip(ctx, path, editInPlace)
	case "insert":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			return
		}

		header.AddToExistingFile(ctx, path, inputMetadata, editInPlace)
	case "read":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			return
		}

		header.Print(ctx, path, compactJSONMode)
	case "verify":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			return
		}

		header.Verify(ctx, path)
	default:
		invalidArgs = true
		return
	}
	return
}
