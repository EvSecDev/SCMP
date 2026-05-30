// Package for user input
package input

import (
	"context"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/web/api/prompt"
	"time"
)

// Generic user input gatherer
func AskUser(ctx context.Context, title, details string) (response string, err error) {
	username := global.AssertFromContext[string](ctx, "username", global.UserKey, "string")

	if username == global.GlobalUsername {
		// CLI mode always uses global username

		// Catch up logger prior to prompt print
		logger := logctx.GetLogger(ctx)
		logger.Wake()
		time.Sleep(100 * time.Microsecond)

		response, err = promptUser(title + ": ")
	} else {
		// Web user
		response, err = prompt.WaitForInput(ctx, title, details)
	}
	return
}

// Generic user secret input gatherer
func AskUserSecret(ctx context.Context, title, details string) (response []byte, err error) {
	username := global.AssertFromContext[string](ctx, "username", global.UserKey, "string")

	if username == global.GlobalUsername {
		// CLI mode always uses global username

		// Catch up logger prior to prompt print
		logger := logctx.GetLogger(ctx)
		logger.Wake()
		time.Sleep(100 * time.Microsecond)

		response, err = promptUserForSecret(title + ": ")
	} else {
		// Web user
		response, err = prompt.WaitForSecret(ctx, title, details)
	}

	return
}
