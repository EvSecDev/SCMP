package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"scmp/core/deployment"
	"scmp/core/drn"
	assocation "scmp/core/drn/association"
	"scmp/core/drn/drnconfig"
	"scmp/core/filesystem"
	"scmp/internal/config"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Core logic for handling DRN association/references for any given deployment.
func TrackDRNChanges(ctx context.Context, commitFiles map[str.LocalRepoPath]str.DeployAction, commit *object.Commit) (hostOverride string, err error) {
	additions := make(map[str.LocalRepoPath]str.DeployAction)
	var removals []str.LocalRepoPath

	ctx = logctx.AppendCtxTag(ctx, logctx.NSDepEval)
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	parentCommit, err := commit.Parents().Next()
	if err != nil {
		err = fmt.Errorf("failed retrieving parent commit: %w", err)
		return
	}

	patch, err := parentCommit.Patch(commit)
	if err != nil {
		err = fmt.Errorf("failed retrieving difference between commits: %w", err)
		return
	}

	tree, err := commit.Tree()
	if err != nil {
		err = fmt.Errorf("failed to retrieve commit tree: %w", err)
		return
	}

	walker := gitinternal.NewTreeWalker(tree, cfg.RepositoryPath)
	searcher := gitinternal.NewTreeSearcher(tree)
	reader := gitinternal.NewTreeReader(tree)

	allDRNs, err := drnconfig.GetAllDRNs(cfg.RepositoryPath, walker, reader)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("all drn retrieval: %w", err)
			return
		}
	}

	ref, err := assocation.NewReferenceFinder(&cfg, allDRNs, walker, searcher, reader)
	if err != nil {
		err = fmt.Errorf("reference finder: %w", err)
		return
	}

	var hostFilter []str.RepoRootDir
	for path, drnFileAction := range commitFiles {
		if !strings.HasPrefix(string(path), drn.ExternalVariableDirectory) {
			continue
		}

		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Evaluating DRN config '%s' for any files (not in this deployment) that reference it's DRNs\n", path)

		var chosenDRNs []str.DRN
		chosenDRNs, err = diffDRNConfig(path, patch)
		if err != nil {
			err = fmt.Errorf("drn diff: %w", err)
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"DRN config '%s' has %d DRN(s): '%v'\n",
			path, len(chosenDRNs), chosenDRNs)

		var relatedFiles []str.LocalRepoPath
		var relatedHosts []str.RepoRootDir
		relatedFiles, relatedHosts, err = ref.FilesReferencingExternals(ctx, chosenDRNs)
		if err != nil {
			err = fmt.Errorf("drn association: %w", err)
			return
		}
		hostFilter = append(hostFilter, relatedHosts...)

		if len(relatedFiles) == 0 {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"DRN config '%s': found 0 files that reference at least one of the marked DRNs\n", path)
		} else {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"DRN config '%s': found %d file(s) that reference at least one of the marked DRN(s): file list '%v'\n",
				path, len(relatedFiles), relatedFiles)
		}

		var listAdditions map[str.LocalRepoPath]str.DeployAction
		var listDeletions []str.LocalRepoPath
		listAdditions, listDeletions, err = drnDeployUpdates(ctx, path, drnFileAction, relatedFiles, commitFiles)
		if err != nil {
			return
		}
		maps.Copy(additions, listAdditions)
		removals = append(removals, listDeletions...)
	}

	hostOverride = str.Join(hostFilter, ",")

	if len(additions) > 0 {
		maps.Copy(commitFiles, additions)
	}
	if len(removals) > 0 {
		for _, path := range removals {
			// Remove DRN from the list, DRN configs do not get deployed
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"DRN config '%s': DRN references tracked, removing file from deployment list\n", path)
			delete(commitFiles, path)
		}
	}

	if len(commitFiles) == 0 {
		logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
			"Deployment contained only DRN config(s) and none of the DRNs are referenced anywhere in the repository\n")
	}
	return
}

// Generate Deploy List update based on DRN config related file list
func drnDeployUpdates(ctx context.Context,
	drnPath str.LocalRepoPath, drnFileAction str.DeployAction,
	relatedFiles []str.LocalRepoPath, commitFiles map[str.LocalRepoPath]str.DeployAction,
) (additions map[str.LocalRepoPath]str.DeployAction, removals []str.LocalRepoPath, err error) {
	additions = make(map[str.LocalRepoPath]str.DeployAction)

	for _, relatedFile := range relatedFiles {
		_, knownFile := commitFiles[relatedFile]
		if knownFile {
			// Already tracked for this deployment, verify drn is not being deleted
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"DRN config '%s': related file '%s': already present in deployment list\n", drnPath, relatedFile)
			continue
		}

		// Related file not in deployment, but DRN config is being deleted.
		// Since the related file was found (i.e. exists on disk) then it will have references to a DRN that does not exist
		// This protects the DRN replacement failing later with unknown DRN
		if drnFileAction == deployment.ActionFileDelete {
			err = fmt.Errorf("drn config '%s' is being deleted but is still referenced by file '%s' which still exists", drnPath, relatedFile)
			return
		}

		// Get the correct deployment action for the file since it is not in the deployment list already
		// Anything that references a DRN must be a file, and the only non "create" actions are for the dir meta file (symlink action is added later)
		var newAction str.DeployAction
		if str.HasSuffix(relatedFile, filesystem.DirMetaFileName) {
			newAction = deployment.ActionDirModify // Correct since the path would have been in the commitFiles already otherwise
		} else {
			newAction = deployment.ActionFileModify
		}
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"DRN config '%s': related file '%s': using intermediate deployment action '%s'\n", drnPath, relatedFile, newAction)

		// File not tracked for this deployment
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"DRN config '%s': related file '%s': adding to deployment list\n", drnPath, relatedFile)
		additions[relatedFile] = newAction
	}

	removals = append(removals, drnPath) // Always delete
	return
}

// Finds differing DRNs and/or values for a DRN config from a git patch
func diffDRNConfig(drnFile str.LocalRepoPath, patch *object.Patch) (changedDRNs []str.DRN, err error) {
	for _, file := range patch.FilePatches() {
		from, to := file.Files()
		if from != nil && from.Path() != string(drnFile) {
			continue
		}
		if to != nil && to.Path() != string(drnFile) {
			continue
		}

		var beforeFile, afterFile string
		for _, chunk := range file.Chunks() {
			switch chunk.Type() {
			case diff.Equal:
				beforeFile += chunk.Content()
				afterFile += chunk.Content()
			case diff.Add:
				afterFile += chunk.Content()
			case diff.Delete:
				beforeFile += chunk.Content()
			default:
				err = fmt.Errorf("unsupported git chunk type %d", chunk.Type())
				return
			}
		}

		if beforeFile == "" && afterFile == "" {
			err = fmt.Errorf("git diff has no content for before or after version of config %s", drnFile)
			return
		}

		var beforeDRNs map[str.DRN]str.DRNVal
		if beforeFile != "" {
			var node drnconfig.CfgNode
			err = json.Unmarshal([]byte(beforeFile), &node)
			if err != nil {
				err = fmt.Errorf("parse before version '%s': %w", drnFile, err)
				return
			}
			beforeDRNs, err = node.FormatAll(string(drnFile))
			if err != nil {
				err = fmt.Errorf("config before version '%s': %w", drnFile, err)
				return
			}
		}

		var afterDRNs map[str.DRN]str.DRNVal
		if afterFile != "" {
			var node drnconfig.CfgNode
			err = json.Unmarshal([]byte(afterFile), &node)
			if err != nil {
				err = fmt.Errorf("parse after version '%s': %w", drnFile, err)
				return
			}
			afterDRNs, err = node.FormatAll(string(drnFile))
			if err != nil {
				err = fmt.Errorf("config after version '%s': %w", drnFile, err)
				return
			}
		}

		changedDRNs = drn.DiffSet(beforeDRNs, afterDRNs)
		if len(changedDRNs) == 0 {
			err = fmt.Errorf("drn diff for config '%s' returned no results, something went wrong", drnFile)
			return
		}

		break // There should only be one matching path in the patch
	}
	return
}
