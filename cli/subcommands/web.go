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
	"scmp/web"
)

func Web(ctx context.Context, subcmdLineage []string, args []string) (exitCode int) {
	var webConfigPath string
	var startServer bool
	var opts config.Opts

	commandFlags := flag.NewFlagSet(subcmdLineage[len(subcmdLineage)-1], flag.ExitOnError)
	commandFlags.StringVar(&webConfigPath, "c", web.DefaultWebConfigPath, "Path to web configuration")
	commandFlags.StringVar(&webConfigPath, "config", web.DefaultWebConfigPath, "Path to web configuration")
	commandFlags.BoolVar(&startServer, "s", false, "Start HTTPS server")
	commandFlags.BoolVar(&startServer, "start-server", false, "Start HTTPS server")
	globalVerbosity := cli.SetGlobalArguments(commandFlags, &opts)

	commandFlags.Usage = func() {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
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

	fmt.Printf("Warning: The web interface is highly experimental and a work-in-progress. Expect incomplete interfaces and bugs.\n")

	if startServer {
		web.StartListener(ctx, webConfigPath)
	} else {
		cli.PrintHelpMenu(commandFlags, subcmdLineage, cli.GetCLICmds())
		return 1
	}
	return 0
}
