// A Secure Configuration Management Program
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/cli/cmdtree"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
)

func main() {
	allOpts := cmdtree.DefineOptions()
	err := cli.WOCLICmds(allOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed building CLI option tree: %v\n", err)
		os.Exit(1)
	}
	var opts config.Opts

	subcmdLineage := []string{cli.RootCLICommand}

	args := os.Args
	commandFlags := flag.NewFlagSet(args[0], flag.ExitOnError)
	globalVerbosity := cli.SetGlobalArguments(commandFlags, &opts)

	commandFlags.Usage = func() {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
	}
	if len(args) < 2 {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		os.Exit(1)
	}
	err = commandFlags.Parse(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Retrieve command and args
	command := args[1]
	args = args[2:]

	// Setting global logging
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = logctx.New(ctx, global.GlobalUsername, *globalVerbosity, ctx.Done())
	logger := logctx.GetLogger(ctx)
	logger.SetFormattedOutput(os.Stdout)
	logctx.StartOutput(ctx)

	// Identify CLI mode
	ctx = context.WithValue(ctx, global.UserKey, global.GlobalUsername)

	// Use primary function from CLI definition
	exitCode := 0
	cmdInfo := allOpts.ChildCommands[command]
	if cmdInfo == nil {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		exitCode = 1
	} else if cmdInfo.PrimaryFunc != nil {
		exitCode = cmdInfo.PrimaryFunc(ctx, append(subcmdLineage, command), args)
	} else {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		exitCode = 1
	}

	// Finish up any stdout writes for global logger
	cancel()
	logger.Wake()
	logger.Wait()
	os.Exit(exitCode)
}
