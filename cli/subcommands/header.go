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

func Header(ctx context.Context, subcmdLineage []string, args []string) (exitCode int) {
	var editInPlace bool
	var inputMetadata string
	var compactJSONMode bool
	var opts config.Opts

	commandFlags := flag.NewFlagSet(subcmdLineage[len(subcmdLineage)-1], flag.ExitOnError)
	commandFlags.BoolVar(&editInPlace, "i", false, "Modify file in-place")
	commandFlags.BoolVar(&editInPlace, "in-place", false, "Modify file in-place")
	commandFlags.StringVar(&inputMetadata, "j", "", "Use provided metadata JSON ('-' to read it from stdin)")
	commandFlags.StringVar(&inputMetadata, "json-metadata", "", "Use provided metadata JSON ('-' to read it from stdin)")
	commandFlags.BoolVar(&compactJSONMode, "C", false, "Print JSON headers in single-line format")
	commandFlags.BoolVar(&compactJSONMode, "compact", false, "Print JSON headers in single-line format")
	globalVerbosity := cli.SetGlobalArguments(commandFlags, &opts)

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

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, opts)

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, *globalVerbosity)

	remainingArgs := commandFlags.Args()

	invalidArgs := headerSetup(ctx, args[0], remainingArgs, editInPlace, compactJSONMode, inputMetadata)
	if invalidArgs {
		cli.PrintHelpMenu(commandFlags, append(subcmdLineage, args[0]), cli.GetCLICmds())
		return 1
	}
	return 0
}

func headerSetup(ctx context.Context, subcommand string, remainingArgs []string, editInPlace, compactJSONMode bool, inputMetadata string) (invalidArgs bool) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSFiles)

	if len(remainingArgs) < 1 {
		invalidArgs = true
		return
	}

	path := str.LocalRepoPath(remainingArgs[0])

	switch subcommand {
	case "edit":
		header.Modify(ctx, path, inputMetadata, editInPlace)
	case "strip":
		header.Strip(ctx, path, editInPlace)
	case "insert":
		header.AddToExistingFile(ctx, path, inputMetadata, editInPlace)
	case "read":
		header.Print(ctx, path, compactJSONMode)
	case "verify":
		header.Verify(ctx, path)
	default:
		invalidArgs = true
		return
	}
	return
}
