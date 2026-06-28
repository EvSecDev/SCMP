package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/setup"
)

func Install(ctx context.Context, subcmdLineage []string, args []string) (exitCode int) {
	var installAAProf bool
	var installDefaultConfig bool
	var installBashAutoComplete bool
	var newRepoBranch string
	var newRepoPath string
	var opts config.Opts

	commandFlags := flag.NewFlagSet(subcmdLineage[len(subcmdLineage)-1], flag.ExitOnError)
	commandFlags.StringVar(&newRepoPath, "repository-path", "", "Path to repository")
	commandFlags.StringVar(&newRepoBranch, "repository-branch-name", "main", "Initial branch new for new repository")
	commandFlags.BoolVar(&installDefaultConfig, "default-config", false, "Write default SSH configuration file")
	commandFlags.BoolVar(&installBashAutoComplete, "bash-autocomplete", false, "Setup BASH autocompletion function")
	commandFlags.BoolVar(&installAAProf, "apparmor-profile", false, "Enable apparmor profile if supported")
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

	ctx = logctx.AppendCtxTag(ctx, logctx.NSSetup)

	if installAAProf {
		setup.AAProfile(ctx, newRepoPath)
	} else if installDefaultConfig {
		setup.SSHConfig(ctx)
	} else if installBashAutoComplete {
		setup.BashAutocomplete(ctx)
	} else if newRepoPath != "" {
		setup.NewRepository(ctx, newRepoPath, newRepoBranch)
	} else {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		return 1
	}
	return 0
}
