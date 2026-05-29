package setup

import (
	"context"
	"os"
	"scmp/internal/fsops"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

// Install sample SSH config if it doesn't already exist
func SSHConfig(ctx context.Context) {
	configPath, err := fsops.ExpandHomeDirectory(sshinternal.DefaultConfigPath)
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to resolve absolute path for '%s': %v\n", sshinternal.DefaultConfigPath, err)
		return
	}

	defaultConfig, err := installationConfigs.ReadFile("static-files/configurations/default-ssh-config")
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to retrieve configuration file from embedded filesystem: %v\n", err)
		return
	}

	// Check if config already exists
	_, err = os.Stat(configPath)
	if !os.IsNotExist(err) {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.WarnLog, "SSH Config file already exists, not overwriting it. Please configure manually.\n")
		return
	} else if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Unable to check if SSH config file already exists: %v\n", err)
		return
	}

	err = os.WriteFile(configPath, defaultConfig, 0640)
	if err != nil {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.ErrorLog, "Failed to write sample SSH config: %v\n", err)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Successfully created new example config in %s\n", configPath)
}
