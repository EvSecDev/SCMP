package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/internal/config"
	"scmp/internal/config/sshconfig"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/secrets"
	"scmp/internal/str"
)

func Secrets(ctx context.Context, subcmdLineage []string, args []string) (exitCode int) {
	var modifyVaultHost string
	var genNewHash bool
	var configPath string
	var opts config.Opts

	commandFlags := flag.NewFlagSet(subcmdLineage[len(subcmdLineage)-1], flag.ExitOnError)
	cli.SetDeployConfArguments(commandFlags, &configPath)
	commandFlags.StringVar(&modifyVaultHost, "p", "", "Create/Update/Delete password for given host.Name")
	commandFlags.StringVar(&modifyVaultHost, "modify-vault-password", "", "Create/Update/Delete password for given host.Name")
	commandFlags.BoolVar(&genNewHash, "generate-password-hash", false, "Generate new user password hash for web")
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

	ctx, err = sshconfig.Set(ctx, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in controller configuration: %v\n", err)
		return 1
	}

	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	err = secrets.CLIEntry(ctx, config, str.RepoRootDir(modifyVaultHost), genNewHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
