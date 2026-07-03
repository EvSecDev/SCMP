package deployment

import "scmp/internal/str"

const (
	IgnoreDirectoryPrefix str.LocalRepoPath = "_"                                  // Top level only
	FailTrackerFile       string            = ".scmp-last-deployment-summary.json" // file name for recording deployment summary details

	FileCountPromptThreshold int = 50

	EmptyFileHash str.FileID = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Deployment modes, but also cli subcommands
	ModeAll   string = "all"
	ModeDiff  string = "diff"
	ModeRetry string = "failures"

	ActionDelete        str.DeployAction = "delete"
	ActionCreate        str.DeployAction = "fileCreate"
	ActionDirCreate     str.DeployAction = "dirCreate"
	ActionDirModify     str.DeployAction = "dirModify"
	ActionSymLinkCreate str.DeployAction = "symlinkCreate"
)
