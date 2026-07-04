package deployment

import "scmp/internal/str"

const (
	IgnoreDirectoryPrefix str.LocalRepoPath = "_"                                  // Top level only
	FailTrackerFile       string            = ".scmp-last-deployment-summary.json" // file name for recording deployment summary details

	FileCountPromptThreshold int = 50

	EmptyFileHash str.FileID = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Deployment modes, but also cli subcommands
	ModeAll      string = "all"
	ModeDiff     string = "diff"
	ModeRetry    string = "failures"
	ModeRollback string = "rollback"

	ActionFileCreate    str.DeployAction = "fileCreate"
	ActionFileModify    str.DeployAction = "fileModify"
	ActionFileDelete    str.DeployAction = "fileDelete"
	ActionDirCreate     str.DeployAction = "dirCreate"
	ActionDirModify     str.DeployAction = "dirModify"
	ActionDirDelete     str.DeployAction = "dirDelete"
	ActionSymLinkCreate str.DeployAction = "symlinkCreate"
	ActionSymLinkModify str.DeployAction = "symlinkModify"
	ActionSymLinkDelete str.DeployAction = "symlinkDelete"
)
