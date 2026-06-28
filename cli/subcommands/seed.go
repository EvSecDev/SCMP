package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/core/seed"
	"scmp/internal/config"
	"scmp/internal/config/sshconfig"
	"scmp/internal/global"
	"scmp/internal/logctx"
)

func Seed(ctx context.Context, subcmdLineage []string, args []string) (exitCode int) {
	var hostOverride string
	var remoteFileOverride string
	var configPath string
	var opts config.Opts

	commandFlags := flag.NewFlagSet(subcmdLineage[len(subcmdLineage)-1], flag.ExitOnError)
	cli.SetDeployConfArguments(commandFlags, &configPath)
	commandFlags.StringVar(&hostOverride, "r", "", "Override remote hosts")
	commandFlags.StringVar(&hostOverride, "remote-hosts", "", "Override remote hosts")
	commandFlags.StringVar(&remoteFileOverride, "R", "", "Override remote file(s)")
	commandFlags.StringVar(&remoteFileOverride, "remote-files", "", "Override remote file(s)")
	commandFlags.BoolVar(&opts.RegexEnabled, "regex", false, "Enables regular expression parsing for file/host overrides")
	commandFlags.BoolVar(&opts.IgnoreDeploymentState, "ignore-deployment-state", false, "Ignores deployment state in configuration file")
	globalVerbosity := cli.SetGlobalArguments(commandFlags, &opts)

	commandFlags.Usage = func() {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
	}
	if len(args) < 1 {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		return 1
	}
	err := commandFlags.Parse(args[0:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, *globalVerbosity)

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, opts)

	ctx = logctx.AppendCtxTag(ctx, logctx.NSSeed)

	ctx, err = sshconfig.Set(ctx, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in controller configuration: %v\n", err)
		return 1
	}

	seed.SeedRepositoryFiles(ctx, hostOverride, remoteFileOverride)
	return 0
}
