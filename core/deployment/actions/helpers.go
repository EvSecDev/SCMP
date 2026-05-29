package actions

import (
	"context"
	"scmp/internal/logctx"
	"time"
)

func watchLongCommand(ctx context.Context, command string, done chan struct{}) {
	select {
	case <-time.After(5 * time.Second):
		// If task takes more than 5 seconds, print status
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Command still running: '%s'\n", command)
	case <-done:
		// If the task finishes before 5 seconds, no feedback message
	}
}
