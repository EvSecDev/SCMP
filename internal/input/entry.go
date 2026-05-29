// Package for user input
package input

import (
	"context"
	"scmp/internal/global"
	"scmp/web/api/prompt"
)

// Generic user input gatherer
func AskUser(ctx context.Context, title, details string) (response string, err error) {
	username := global.AssertFromContext[string](ctx, "username", global.UserKey, "string")

	if username == global.GlobalUsername {
		// CLI mode always uses global username
		response, err = PromptUser(title + ": ")
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
		response, err = PromptUserForSecret(title + ": ")
	} else {
		// Web user
		response, err = prompt.WaitForSecret(ctx, title, details)
	}

	return
}
