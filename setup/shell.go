package setup

import (
	"context"
	"os"
	"path/filepath"
	"scmp/internal/logctx"
	"strings"
)

func BashAutocomplete(ctx context.Context) {
	const sysAutocompleteDir string = "/usr/share/bash-completion/completions"
	autoCompleteFunc, err := installationConfigs.ReadFile("static-files/autocomplete.sh")
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to retrieve autocomplete file from embedded filesystem: %v\n", err)
		return
	}

	executablePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to retrieve absolute executable path for profile installation: %v\n", err)
		return
	}
	executableName := filepath.Base(executablePath)

	// Inject actual executable name into completion script
	autoCompletion := strings.Replace(string(autoCompleteFunc), "_controller()", "_"+executableName+"()", 1)
	autoCompletion = strings.Replace(autoCompletion, "complete -F _controller controller", "complete -F _"+executableName+" "+executableName, 1)
	autoCompleteFunc = []byte(autoCompletion)

	// Write to system, or fallback to users home
	var autoCompleteFilePath string
	_, err = os.Stat(sysAutocompleteDir)
	if err == nil {
		autoCompleteFilePath = filepath.Join(sysAutocompleteDir, executableName)
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to find user home directory: %v\n", err)
			return
		}
		userDir := filepath.Join(homeDir, ".bash_completion.d")
		err = os.MkdirAll(userDir, 0750)
		if err != nil {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to create user autocomplete dir: %v\n", err)
			return
		}

		autoCompleteFilePath = filepath.Join(userDir, executableName)
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "System completion dir missing, installing bash completion at %s\n", autoCompleteFilePath)
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Make sure ~/.bashrc sources ~/.bash_completion and ~/.bash_completion.d/*\n")
	}

	err = os.WriteFile(autoCompleteFilePath, autoCompleteFunc, 0644)
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to write autocompletion file: %v\n", err)
		return
	}
}
