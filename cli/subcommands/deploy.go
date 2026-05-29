// Package for the primary entry points for different subcommands
package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/core/deployment/local"
	"scmp/internal/config"
	"scmp/internal/config/sshconfig"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

func Deploy(ctx context.Context, commandname string, args []string) {
	var commitID string
	var hostOverride string
	var localFileOverride string
	var testConfig bool
	var calledByGitHook bool
	var configPath string
	var opts config.Opts

	commandFlags := flag.NewFlagSet(commandname, flag.ExitOnError)
	commandFlags.StringVar(&hostOverride, "r", "", "Override hosts for deployment")
	commandFlags.StringVar(&hostOverride, "remote-hosts", "", "Override hosts for deployment")
	commandFlags.StringVar(&localFileOverride, "l", "", "Override file(s) for deployment")
	commandFlags.StringVar(&localFileOverride, "local-files", "", "Override file(s) for deployment")
	commandFlags.StringVar(&commitID, "C", "", "Commit ID (hash) to deploy from")
	commandFlags.StringVar(&commitID, "commitid", "", "Commit ID (hash) to deploy from")
	commandFlags.IntVar(&opts.MaxDeployConcurrency, "M", sshinternal.MaxSSHChannels, "Maximum simultaneous file deployments per host (1 disables threading)")
	commandFlags.IntVar(&opts.MaxDeployConcurrency, "max-deploy-threads", sshinternal.MaxSSHChannels, "Maximum simultaneous file deployments per host (1 disables threading)")
	commandFlags.BoolVar(&opts.RunInstallCommands, "install", false, "Run installation commands during deployment")
	commandFlags.BoolVar(&opts.DisableReloads, "disable-reloads", false, "Disables running any reload commands")
	commandFlags.BoolVar(&opts.IgnoreDeploymentState, "ignore-deployment-state", false, "Ignores deployment state in configuration file")
	commandFlags.BoolVar(&calledByGitHook, "enable-commit-auto-rollback", false, "Enable git commit rollback on local processing errors")
	commandFlags.BoolVar(&testConfig, "t", false, "Test configuration syntax and option validity")
	commandFlags.BoolVar(&testConfig, "test-config", false, "Test configuration syntax and option validity")
	commandFlags.BoolVar(&opts.RegexEnabled, "regex", false, "Enables regular expression parsing for file/host overrides")
	globalVerbosity := cli.SetGlobalArguments(commandFlags, &opts)
	cli.SetSSHArguments(commandFlags, &opts)
	cli.SetDeployConfArguments(commandFlags, &configPath)

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
	subcommand := args[0]

	// Set verbosity again if the user change at this command level
	logctx.SetLogLevel(ctx, *globalVerbosity)

	// Set options in context
	ctx = context.WithValue(ctx, global.OpsKey, opts)

	// Environment variable to flag when rollback should be performed on local deploy errors
	_, gitDeployEnvFlag := os.LookupEnv("SCMP_GIT_DEPLOY")
	if gitDeployEnvFlag && subcommand == "diff" {
		calledByGitHook = true
	}

	ctx, err = sshconfig.Set(ctx, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in controller configuration: %v\n", err)

		err = gitinternal.RollBackOneCommit(ctx, commitID, calledByGitHook, true)
		if err != nil {
			fmt.Printf("Error rolling back commit. %v\n", err)
		}

		os.Exit(1)
	}

	if testConfig {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "configuration file %s test is successful\n", configPath)
		return
	}

	if cli.IsValidSubcommand(cli.GetCLICmds(), commandname, subcommand) {
		var rollbackCommit bool
		rollbackCommit, err = local.StartDeploy(ctx, subcommand, commitID, hostOverride, localFileOverride)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Deployment Failed: %v\n", err)

			err = gitinternal.RollBackOneCommit(ctx, commitID, calledByGitHook, rollbackCommit)
			if err != nil {
				fmt.Printf("Error rolling back commit. %v\n", err)
			}

			os.Exit(1)
		}
	} else {
		cli.PrintHelpMenu(commandFlags, commandname, cli.GetCLICmds())
		os.Exit(1)
	}
}
