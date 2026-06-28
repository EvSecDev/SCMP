package subcommands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"scmp/cli"
	"scmp/core/drn"
	assocation "scmp/core/drn/association"
	"scmp/core/drn/drnconfig"
	"scmp/core/drn/resolve"
	"scmp/internal/config"
	"scmp/internal/config/sshconfig"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"
)

func DRN(ctx context.Context, subcmdLineage []string, args []string) (exitCode int) {
	var hostAlias string
	var repoFilePath string
	var configPath string
	var opts config.Opts

	commandFlags := flag.NewFlagSet(subcmdLineage[len(subcmdLineage)-1], flag.ExitOnError)
	cli.SetDeployConfArguments(commandFlags, &configPath)
	commandFlags.StringVar(&hostAlias, "host-alias", "", "Use host alias for lookup context")
	commandFlags.StringVar(&repoFilePath, "repo-file-path", "", "Use repository relative path for lookup context")
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

	ctx, err = sshconfig.Set(ctx, configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in controller configuration: %v\n", err)
		return 1
	}

	remainingArgs := commandFlags.Args()

	newsub := append(subcmdLineage, args[0])

	invalidArgs, exitCode := drnSetup(ctx, args[0], remainingArgs, hostAlias, repoFilePath)
	if invalidArgs {
		cli.PrintHelpMenu(commandFlags, newsub, cli.GetCLICmds())
		return 1
	}
	return exitCode
}

func drnSetup(ctx context.Context, subcommand string, remainingArgs []string, hostAlias, repoFilePath string) (invalidArgs bool, exitCode int) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSDRN)

	switch subcommand {
	case "lookup":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			exitCode = 1
			return
		}
		requestedDRN := ensureOpenCloseChars(remainingArgs[0])

		value, err := resolve.LookupValue(ctx, requestedDRN, str.LocalRepoPath(repoFilePath), str.RepoRootDir(hostAlias))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed DRN '%s' lookup: %v\n", requestedDRN, err)
			exitCode = 1
			return
		}
		if value != "" {
			fmt.Printf("DRN:%s=%s\n", requestedDRN, value)
		} else {
			fmt.Printf("DRN %s does not resolve to a value\n", requestedDRN)
			exitCode = 2
		}
	case "new":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			exitCode = 1
			return
		}
		DRNandValue := strings.Split(remainingArgs[0], "=")
		if len(DRNandValue) != 2 {
			fmt.Fprintf(os.Stderr, "A new DRN must contain a value: format is <DRN string>=<DRN value>\n")
			exitCode = 1
			return
		}
		newDRN := ensureOpenCloseChars(DRNandValue[0])
		newValue := DRNandValue[1]

		path, err := drnconfig.WriteNewExternal(ctx, newDRN, str.DRNVal(newValue))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed DRN write: %v\n", err)
			exitCode = 1
			return
		}
		fmt.Printf("Successfully wrote DRN %s to repository configuration %s\n", newDRN, path)
	case "dump":
		// Retrieve required deployment options
		cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

		table, err := drnconfig.ShowAll(cfg.RepositoryPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed DRN dump: %v\n", err)
			exitCode = 1
			return
		}
		fmt.Printf("%s\n", table)
	case "reference":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			exitCode = 1
			return
		}

		var drnList []str.DRN
		chosenDRNs := strings.Split(remainingArgs[0], ",")
		for _, chosenDRN := range chosenDRNs {
			drnList = append(drnList, str.DRN(ensureOpenCloseChars(chosenDRN)))
		}

		// Retrieve required deployment options
		cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

		allDRNs, err := drnconfig.GetAllDRNs(cfg.RepositoryPath, nil, nil) // use live filesystem for search
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed DRN walk: %v\n", err)
			exitCode = 1
			return
		}

		refFinder, err := assocation.NewReferenceFinder(&cfg, allDRNs, nil, nil, nil) // use live filesystem for search
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed reference finder creation: %v\n", err)
			exitCode = 1
			return
		}

		files, hosts, err := refFinder.FilesReferencingExternals(ctx, drnList)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed finding references: %v\n", err)
			exitCode = 1
			return
		}

		fmt.Printf("File(s) referencing DRN(s) %v:\n", drnList)
		if len(files) == 0 {
			fmt.Printf("  **No Files Reference DRN**\n")
		}
		for _, file := range files {
			fmt.Printf("  %s\n", file)
		}
		fmt.Printf("\nHost(s) referencing DRN(s) %v:\n", drnList)
		if len(files) == 0 {
			fmt.Printf("  **No Hosts Reference DRN**\n")
		}
		for _, host := range hosts {
			fmt.Printf("  %s\n", host)
		}
		if len(files) == 0 && len(hosts) == 0 {
			exitCode = 2
		}
	case "validate":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			exitCode = 1
			return
		}
		chosenDRNs := strings.Split(remainingArgs[0], ",")
		for _, chosenDRN := range chosenDRNs {
			chosenDRN = ensureOpenCloseChars(chosenDRN)
			_, err := drn.Validate(chosenDRN)
			if err != nil {
				fmt.Printf("DRN %s is invalid: %v\n", chosenDRN, err)
				exitCode = 1
			} else {
				fmt.Printf("DRN %s is valid\n", chosenDRN)
			}
		}
	case "resolve-file":
		if len(remainingArgs) < 1 {
			invalidArgs = true
			exitCode = 1
			return
		}
		path := str.LocalRepoPath(remainingArgs[0])

		content, err := resolve.ShowFileResolved(ctx, path, str.RepoRootDir(hostAlias))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed resolution: %v\n", err)
			exitCode = 1
			return
		}
		fmt.Printf("%s", string(content)) // No newline
	default:
		invalidArgs = true
		exitCode = 1
		return
	}
	return
}

func ensureOpenCloseChars(input string) (normalized string) {
	normalized = input
	if !strings.HasPrefix(normalized, drn.OpenDelimiter) {
		normalized = drn.OpenDelimiter + normalized
	}
	if !strings.HasSuffix(normalized, drn.CloseDelimiter) {
		normalized = normalized + drn.CloseDelimiter
	}
	return
}
