// Package for executing ad-hoc commands and scripts on remote hosts
package execution

import (
	"context"
	"fmt"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

func CLIEntry(ctx context.Context, executeCommands, hostOverride, remoteFileOverride string) (err error) {
	// Pull contents of out file URIs
	hostOverride, err = parsing.RetrieveURIFile(ctx, hostOverride)
	if err != nil {
		err = fmt.Errorf("failed to parse remove-hosts URI: %w", err)
		return
	}
	remoteFileOverride, err = parsing.RetrieveURIFile(ctx, remoteFileOverride)
	if err != nil {
		err = fmt.Errorf("failed to parse local-files URI: %w", err)
		return
	}

	if strings.HasPrefix(executeCommands, "file:") {
		runScript(ctx, executeCommands, hostOverride, str.RemotePath(remoteFileOverride))
	} else if executeCommands != "" {
		runCmd(ctx, executeCommands, hostOverride)
	}
	return
}
