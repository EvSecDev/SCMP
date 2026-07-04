package repository

import (
	"context"
	"fmt"
	"scmp/core/deployment"
	"scmp/internal/logctx"
	"scmp/internal/str"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Generates an inverse commit files map of a given commit file change list
func GetRollbackFiles(ctx context.Context, changedFiles []GitChangedFileMetadata, fileOverride string) (commitFiles map[str.LocalRepoPath]str.DeployAction, err error) {
	commitFiles = make(map[str.LocalRepoPath]str.DeployAction)

	fwdCommitFiles := ParseChangedFiles(ctx, changedFiles, fileOverride)
	for repoPath, action := range fwdCommitFiles {
		// Creates become deletes
		// Deletes become creates
		// Modify stays modify (just going to deploy the previous content version)
		var newAction str.DeployAction
		switch action {
		case deployment.ActionFileCreate:
			newAction = deployment.ActionFileDelete
		case deployment.ActionFileModify:
			newAction = deployment.ActionFileModify
		case deployment.ActionFileDelete:
			newAction = deployment.ActionFileCreate
		case deployment.ActionDirCreate:
			newAction = deployment.ActionDirDelete
		case deployment.ActionDirModify:
			newAction = deployment.ActionDirModify
		case deployment.ActionDirDelete:
			newAction = deployment.ActionDirCreate
		case deployment.ActionSymLinkCreate:
			newAction = deployment.ActionSymLinkDelete
		case deployment.ActionSymLinkModify:
			newAction = deployment.ActionSymLinkModify
		case deployment.ActionSymLinkDelete:
			newAction = deployment.ActionSymLinkCreate
		}

		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Inverting file '%s': action '%s' -> '%s'\n", repoPath, action, newAction)

		commitFiles[repoPath] = newAction
	}
	return
}

// Retrieves the parent tree of the given commit
func GetParentTree(commit *object.Commit) (parentTree *object.Tree, err error) {
	parentCommit, err := commit.Parents().Next()
	if err != nil {
		err = fmt.Errorf("failed to retrieve parent commit: %w", err)
		return
	}

	parentTree, err = parentCommit.Tree()
	if err != nil {
		err = fmt.Errorf("failed to retrieve parent commit tree: %w", err)
		return
	}
	return
}
