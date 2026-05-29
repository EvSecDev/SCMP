package gitinternal

import (
	"context"
	"fmt"
	"scmp/internal/logctx"

	"github.com/go-git/go-git/v5"
)

func CLIEntry(ctx context.Context, subcommand string, args []string, commitMessage string) (invalidArgs bool, err error) {
	switch subcommand {
	case "add":
		ctx = logctx.AppendCtxTag(ctx, logctx.NSGit)

		if len(args) < 2 {
			invalidArgs = true
			return
		}

		files := args[1]
		err = Add(ctx, files)
		if err != nil {
			err = fmt.Errorf("failed to add changes to working tree: %w", err)
			return
		}
	case "status":
		ctx = logctx.AppendCtxTag(ctx, logctx.NSGit)

		var status git.Status
		_, status, err = OpenCWD(ctx)
		if err != nil {
			err = fmt.Errorf("failed to retrieve worktree status: %w", err)
			return
		}

		if status.IsClean() {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "no changes, working tree clean\n")
		} else if !status.IsClean() {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "%s", status.String())
		}
	case "commit":
		if commitMessage == "" {
			invalidArgs = true
			return
		}

		// Proceed with the commit
		err = Commit(ctx, commitMessage)
		if err != nil {
			err = fmt.Errorf("failed to commit changes: %w", err)
			return
		}
	default:
		invalidArgs = true
		return
	}
	return
}
