package cli

import (
	"flag"
	"scmp/internal/config"
	"scmp/internal/sshinternal"
)

// Argument Groups

func SetGlobalArguments(fs *flag.FlagSet, opts *config.Opts) (requestedLogLevel *int) {
	requestedLogLevel = new(int)
	fs.BoolVar(&opts.DetailedSummaryRequested, "with-summary", false, "Generate JSON summary of actions")
	fs.BoolVar(&opts.ForceEnabled, "force", false, "Do not exit/abort on failures")
	fs.BoolVar(&opts.AllowDeletions, "allow-deletions", false, "Permits deletions of files/entries")
	fs.BoolVar(&opts.DryRunEnabled, "T", false, "Conducts non-mutating actions (no remote actions)")
	fs.BoolVar(&opts.DryRunEnabled, "dry-run", false, "Conducts non-mutating actions (no remote actions)")
	fs.BoolVar(&opts.WetRunEnabled, "w", false, "Conducts non-mutating actions (including remote actions)")
	fs.BoolVar(&opts.WetRunEnabled, "wet-run", false, "Conducts non-mutating actions (including remote actions)")
	fs.IntVar(requestedLogLevel, "v", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")
	fs.IntVar(requestedLogLevel, "verbosity", 1, "Increase detailed progress messages (Higher is more verbose) <0...5>")
	return
}

func SetDeployConfArguments(fs *flag.FlagSet, configPath *string) {
	fs.StringVar(configPath, "c", sshinternal.DefaultConfigPath, "Path to the configuration file")
	fs.StringVar(configPath, "config", sshinternal.DefaultConfigPath, "Path to the configuration file")
}

func SetSSHArguments(fs *flag.FlagSet, opts *config.Opts) {
	fs.StringVar(&opts.RunAsUser, "u", "root", "User name to run sudo commands as")
	fs.StringVar(&opts.RunAsUser, "run-as-user", "root", "User name to run sudo commands as")
	fs.BoolVar(&opts.DisableSudo, "disable-privilege-escalation", false, "Disables use of sudo when executing commands remotely")
	fs.IntVar(&opts.ExecutionTimeout, "execution-timeout", sshinternal.DefaultCommandTimeout, "Timeout in seconds for user-defined commands")
	fs.IntVar(&opts.MaxSSHConcurrency, "m", sshinternal.MaxSSHConnections, "Maximum simultaneous SSH connections (1 disables threading)")
	fs.IntVar(&opts.MaxSSHConcurrency, "max-conns", sshinternal.MaxSSHConnections, "Maximum simultaneous SSH connections (1 disables threading)")
}
