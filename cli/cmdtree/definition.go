// Package for the specific command/subcommand help menu tree specific to this program
package cmdtree

import (
	"scmp/cli"
	"scmp/cli/subcommands"
	"scmp/core/deployment"
)

// Defines all commands/subcommands and their relationships and descriptions
func DefineOptions() (cmdOpts *cli.CommandSet) {
	// Root level
	root := &cli.CommandSet{
		Description:     "Secure Configuration Management Program (SCMP)",
		FullDescription: "  Deploy configuration files from a git repository to Linux servers via SSH\n  Deploy ad-hoc commands and scripts to Linux servers via SSH",
		CommandName:     cli.RootCLICommand,
		ChildCommands:   make(map[string]*cli.CommandSet),
	}

	// Deployment
	root.ChildCommands["deploy"] = &cli.CommandSet{
		Description:     "Deploy configurations",
		FullDescription: "Takes configuration files from local repository, transfers them to remote servers, and reloads associated services",
		PrimaryFunc:     subcommands.Deploy,
		CommandName:     "deploy",
		UsageOption:     "",
		ChildCommands: map[string]*cli.CommandSet{
			deployment.ModeAll: {
				CommandName:     deployment.ModeAll,
				Description:     "Deploy Current Configurations",
				FullDescription: "Deploy configurations from HEAD commit or specified commit regardless of commit difference",
			},
			deployment.ModeDiff: {
				CommandName:     deployment.ModeDiff,
				Description:     "Deploy Configurations from Commit Diff",
				FullDescription: "Deploy the difference in configurations from the given commit",
			},
			deployment.ModeRetry: {
				CommandName:     deployment.ModeRetry,
				Description:     "Deploy Configurations from last Complete Deployment Failure",
				FullDescription: "Deploy failed configurations from last total failed deployment using local cached failure file",
			},
		},
	}

	// Web
	root.ChildCommands["web"] = &cli.CommandSet{
		CommandName:     "web",
		Description:     "Start Web Server",
		FullDescription: "Start HTTPS server to serve web graphical user interface",
		PrimaryFunc:     subcommands.Web,
	}

	// Repo Seeding
	root.ChildCommands["seed"] = &cli.CommandSet{
		CommandName:     "seed",
		Description:     "Download Remote Configurations",
		FullDescription: "Retrieve existing remote configurations and associated metadata and store in local repository",
		PrimaryFunc:     subcommands.Seed,
	}

	// Local file data handling
	root.ChildCommands["file"] = &cli.CommandSet{
		CommandName:     "file",
		Description:     "Modify Local Data",
		FullDescription: "Manipulate local repository files and their data",
		PrimaryFunc:     subcommands.File,
		ChildCommands: map[string]*cli.CommandSet{
			"new": {
				CommandName:     "new",
				UsageOption:     "<file path>",
				Description:     "Create File with Template Metadata",
				FullDescription: "Makes file at specified path with example metadata and data",
			},
			"replace-data": {
				CommandName:     "replace-data",
				UsageOption:     "<source file> <destination file>",
				Description:     "Replace File Data",
				FullDescription: "Replace Chosen File's Data with Given File's Data",
			},
		},
	}

	// Local file metadata handling
	root.ChildCommands["header"] = &cli.CommandSet{
		CommandName:     "header",
		Description:     "Modify File Headers",
		FullDescription: "Manipulate local file JSON metadata headers",
		PrimaryFunc:     subcommands.Header,
		ChildCommands: map[string]*cli.CommandSet{
			"edit": {
				CommandName:     "edit",
				UsageOption:     "<file path>",
				Description:     "Change Metadata Header Values",
				FullDescription: "Modify values in the existing JSON header via direct input JSON or via interactive prompts",
			},
			"strip": {
				CommandName:     "strip",
				UsageOption:     "<file path>",
				Description:     "Remove Metadata Header",
				FullDescription: "Deletes the JSON header from the given file",
			},
			"insert": {
				CommandName:     "insert",
				UsageOption:     "<file path>",
				Description:     "Add Metadata Header to Existing File",
				FullDescription: "Use provided JSON to add metadata header to a file that does not have one",
			},
			"read": {
				CommandName:     "read",
				UsageOption:     "<file path>",
				Description:     "Print Metadata Header from File",
				FullDescription: "Extract JSON header from file and format",
			},
			"verify": {
				CommandName:     "verify",
				UsageOption:     "<file path>",
				Description:     "Test Metadata Header Validity",
				FullDescription: "Tests the extraction of file header and the syntax validity of the JSON",
			},
		},
	}

	// Executions
	root.ChildCommands["exec"] = &cli.CommandSet{
		CommandName:     "exec",
		UsageOption:     "<remote command | file://local-script>",
		Description:     "Execute Remote Commands",
		FullDescription: "Execute remote commands and scripts on remote hosts and universal groups",
		PrimaryFunc:     subcommands.Exec,
	}

	// File transfers
	root.ChildCommands["scp"] = &cli.CommandSet{
		CommandName:     "scp",
		UsageOption:     "[src host:]<src path> [dst host:]<dst path>",
		Description:     "Transfer Files",
		FullDescription: "Transfer local files to remote hosts and universal groups",
		PrimaryFunc:     subcommands.SCP,
	}

	// Repository
	root.ChildCommands["git"] = &cli.CommandSet{
		CommandName:     "git",
		Description:     "Repository Actions",
		FullDescription: "Standard git repository manipulations and support for artifact file tracking",
		PrimaryFunc:     subcommands.Git,
		ChildCommands: map[string]*cli.CommandSet{
			"add": {
				CommandName:     "add",
				UsageOption:     "<path|glob>",
				Description:     "Add file(s)/dir(s) to the worktree",
				FullDescription: "Add files and/or directories by exact path or glob matches to the working tree",
			},
			"status": {
				CommandName:     "status",
				Description:     "Show Current Worktree Status",
				FullDescription: "Display status of files and/or directories both in the worktree and not tracked",
			},
			"commit": {
				CommandName:     "commit",
				Description:     "Commit Changes to Repository",
				FullDescription: "Commit any tracked changes in the worktree to the repository",
			},
		},
	}

	// Secrets
	root.ChildCommands["secrets"] = &cli.CommandSet{
		CommandName:     "secrets",
		Description:     "Modify Vault",
		FullDescription: "Add/Modify/Delete entries in the local password vault",
		PrimaryFunc:     subcommands.Secrets,
	}

	// Controller installation
	root.ChildCommands["install"] = &cli.CommandSet{
		CommandName:     "install",
		Description:     "Initial Setups",
		FullDescription: "Install default configurations for apparmor and SSH and setup new repositories",
		PrimaryFunc:     subcommands.Install,
	}

	// Version Info
	root.ChildCommands["version"] = &cli.CommandSet{
		CommandName:     "version",
		Description:     "Show Version Information",
		FullDescription: "Display meta information about program",
		PrimaryFunc:     subcommands.Version,
	}

	cmdOpts = root
	return
}
