package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/core/transfer"
	"scmp/internal/config"
	"scmp/internal/config/sshconfig"
	"scmp/internal/global"
	"scmp/internal/logctx"
)

func SCP(ctx context.Context, commandname string, args []string) {
	var configPath string
	var opts config.Opts

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	cli.SetDeployConfArguments(commandFlags, &configPath)
	globalVerbosity := cli.SetGlobalArguments(commandFlags, &opts)

	commandFlags.Usage = func() {
		cli.PrintHelpMenu(commandFlags, commandname, cli.GetCLICmds())
	}
	if len(args) < 1 {
		cli.PrintHelpMenu(commandFlags, commandname, cli.GetCLICmds())
		os.Exit(1)
	}
	err := commandFlags.Parse(args[0:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, *globalVerbosity)

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, opts)

	remainingArgs := commandFlags.Args()

	sourceHost, sourcePath, destHost, destPath := transfer.ParseArgs(remainingArgs)

	ctx, err = sshconfig.Set(ctx, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in controller configuration: %v\n", err)
		os.Exit(1)
	}
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	err = transfer.BulkFile(ctx, cfg.HostInfo, sourceHost, sourcePath, destHost, destPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to transfer files: %v\n", err)
		os.Exit(1)
	}
}
