package api

import (
	"context"
	"fmt"
	"path/filepath"
	"scmp/internal/gitinternal"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/web/internal"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

func repoStatusAPI(baseCtx context.Context, clientCtx context.Context, req internal.Request) (resp any, errObj internal.Error) {
	status, errObj := getCurrentRepoStatus(clientCtx)
	if errObj.Message != "" {
		errObj.Message = "Failed checking repository staging status"
		return
	}

	resp = status
	return
}

func repoRefreshAPI(baseCtx context.Context, clientCtx context.Context, req internal.Request) (resp any, errObj internal.Error) {
	repoPath, err := gitinternal.RetrieveRepoPath(clientCtx)
	if err != nil {
		errObj.New(rpcInternalError, "Failed retrieving repository path", err.Error())
		return
	}

	err = gitinternal.ArtifactTracking(clientCtx, repoPath)
	if err != nil {
		errObj.New(rpcInternalError, "Failed refreshing artifacts", err.Error())
		return
	}

	status, errObj := getCurrentRepoStatus(clientCtx)
	if errObj.Message != "" {
		errObj.Message = "Failed checking repository staging status"
		return
	}

	resp = status
	return
}

func repoStageAddAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[PathList](fullReq.Params, "req", "PathList")

	worktree, _, err := gitinternal.OpenCWD(clientCtx)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to open local repository", err.Error())
		return
	}

	failedAddFiles := make(map[string][]string)
	for _, path := range req.Paths {
		cleanPath, err := validateRequestedFilePath(clientCtx, path)
		if err != nil {
			errObj.New(rpcInvalidParams, "Invalid requested file path "+path, err.Error())
			return
		}

		_, err = worktree.Add(cleanPath)
		if err != nil {
			failedAddFiles[err.Error()] = append(failedAddFiles[err.Error()], path)
		}
	}

	// Get new status
	status, errObj := getCurrentRepoStatus(clientCtx)
	if errObj.Message != "" {
		errObj.Message = "Failed checking repository staging status"
		return
	}

	// All staging failed
	if len(failedAddFiles) > 0 {
		var combinedErrorMsg string
		for errMsg, paths := range failedAddFiles {
			combinedErrorMsg += fmt.Sprintf("'%s': %v,", errMsg, paths)
		}
		combinedErrorMsg = strings.TrimSuffix(combinedErrorMsg, ",")

		errObj.New(rpcInternalError, "All items failed to stage", combinedErrorMsg)
		return
	}

	// All staging succeeded
	resp = status
	return
}

func repoStageRemoveAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[PathList](fullReq.Params, "req", "PathList")

	worktree, _, err := gitinternal.OpenCWD(clientCtx)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to open local repository", err.Error())
		return
	}

	var cleanPaths []string
	for _, path := range req.Paths {
		cleanPath, err := validateRequestedFilePath(clientCtx, path)
		if err != nil {
			errObj.New(rpcInvalidParams, "Invalid requested file path", err.Error())
			return
		}

		cleanPaths = append(cleanPaths, cleanPath)
	}

	err = worktree.Reset(&git.ResetOptions{
		Mode:  git.MixedReset,
		Files: cleanPaths,
	})
	if err != nil {
		errObj.New(rpcInternalError, "Failed removing files from staging", err.Error())
		return
	}

	status, errObj := getCurrentRepoStatus(clientCtx)
	if errObj.Message != "" {
		errObj.Message = "Failed checking repository staging status"
		return
	}

	resp = status
	return
}

func getCurrentRepoStatus(ctx context.Context) (repoStatus RepoStatus, errObj internal.Error) {
	worktree, status, err := gitinternal.OpenCWD(ctx)
	if err != nil {
		errObj.Code = rpcInternalError
		errObj.Data = "git open failed: " + err.Error()
		return
	}

	repoStatus.Staged = []RepoFilestatus{}
	repoStatus.Unstaged = []RepoFilestatus{}

	// Return early if worktree is clean
	if status.IsClean() {
		return
	}

	statusMap, err := worktree.Status()
	if err != nil {
		errObj.Code = rpcInternalError
		errObj.Data = "unable to check current staging status:" + err.Error()
		return
	}

	for path, stat := range statusMap {
		cleanPath := filepath.ToSlash(path)

		// Only consider staged if it's NOT unmodified AND NOT untracked
		if stat.Staging != git.Unmodified && stat.Staging != git.Untracked {
			repoStatus.Staged = append(repoStatus.Staged, RepoFilestatus{
				Path:   cleanPath,
				Status: gitinternal.StatusCodeToString(stat.Staging),
			})
		}

		// Any change in worktree is considered unstaged (including untracked)
		if stat.Worktree != git.Unmodified {
			repoStatus.Unstaged = append(repoStatus.Unstaged, RepoFilestatus{
				Path:   cleanPath,
				Status: gitinternal.StatusCodeToString(stat.Worktree),
			})
		}
	}

	return
}

func repoCommitAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[RepoCommit](fullReq.Params, "req", "RepoCommit")

	worktree, status, err := gitinternal.OpenCWD(clientCtx)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to open repository", err.Error())
		return
	}

	if status.IsClean() {
		errObj.New(rpcConflict, "Nothing to commit", "")
		return
	}

	commitHash, err := worktree.Commit(req.Message, &git.CommitOptions{
		AllowEmptyCommits: false,
	})
	if err != nil {
		errObj.New(rpcInternalError, "Failed commit", err.Error())
		return
	}

	repoPath, err := gitinternal.RetrieveRepoPath(clientCtx)
	if err != nil {
		errObj.New(rpcInternalError, "Failed retrieving repository path", err.Error())
		return
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to refresh repository", err.Error())
		return
	}

	commitInfo, err := buildWebRepoCommitInfo(repo, commitHash)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to retrieve new commit information", err.Error())
		return
	}

	resp = commitInfo
	return
}

func buildWebRepoCommitInfo(repo *git.Repository, commitHash plumbing.Hash) (newCommitInfo RepoCommitInfo, err error) {
	commit, err := repo.CommitObject(commitHash)
	if err != nil {
		err = fmt.Errorf("error retrieving commit object: %w", err)
		return
	}

	// Basic info
	newCommitInfo = RepoCommitInfo{
		ShortHash:   strings.ToUpper(commitHash.String()[:7]),
		FullHash:    strings.ToUpper(commitHash.String()),
		Date:        commit.Author.When.UTC().Format(time.RFC3339),
		AuthorName:  commit.Author.Name,
		AuthorEmail: commit.Author.Email,
		Message:     commit.Message,
	}

	// Include GPG signature when present
	if commit.PGPSignature != "" {
		newCommitInfo.GPGSignature = commit.PGPSignature
	}

	// Get files changed by this commit
	var filesChanged []RepoFilestatus

	var patch *object.Patch
	if commit.NumParents() == 0 {
		// Initial commit: diff against empty tree
		emptyTree := &object.Tree{}

		var tree *object.Tree
		tree, err = commit.Tree()
		if err != nil {
			err = fmt.Errorf("failed to get commit tree: %w", err)
			return
		}
		patch, err = emptyTree.Patch(tree)
	} else {
		var parent *object.Commit
		parent, err = commit.Parent(0)
		if err != nil {
			err = fmt.Errorf("failed to get parent commit: %w", err)
			return
		}
		patch, err = parent.Patch(commit)
	}
	if err != nil {
		err = fmt.Errorf("failed to get patch: %w", err)
		return
	}

	// Iterate over file patches to get status
	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()
		var status string
		var path string

		switch {
		case from == nil && to != nil:
			status = "added"
			path = to.Path()
		case from != nil && to == nil:
			status = "deleted"
			path = from.Path()
		case from != nil && to != nil:
			status = "modified"
			path = to.Path()
		default:
			status = "unknown"
			path = ""
		}

		filesChanged = append(filesChanged, RepoFilestatus{
			Path:   path,
			Status: status,
		})
	}

	newCommitInfo.FilesChanged = filesChanged
	newCommitInfo.NumberOfChanges = len(filesChanged)

	// Get branches pointing to this commit
	branches := []string{}
	tags := []string{}

	// Iterate over all references
	refs, err := repo.References()
	if err != nil {
		err = fmt.Errorf("failed to list references: %w", err)
		return
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference {
			return nil
		}

		if ref.Hash() != commitHash {
			return nil
		}

		name := ref.Name()
		switch {
		case name.IsBranch():
			branches = append(branches, name.Short())
		case name.IsTag():
			tags = append(tags, name.Short())
		}

		return nil
	})
	if err != nil && err != storer.ErrStop {
		err = fmt.Errorf("failed to inspect references: %w", err)
		return
	}

	newCommitInfo.Branches = branches
	newCommitInfo.Tags = tags
	return
}

func repoHistoryAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[PaginationReq](fullReq.Params, "req", "PaginationReq")
	maxCommits := req.Limit
	commitListOffset := req.Offset

	repoPath, err := gitinternal.RetrieveRepoPath(clientCtx)
	if err != nil {
		errObj.New(rpcInternalError, "Failed retrieving repository path", err.Error())
		return
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to refresh repository", err.Error())
		return
	}

	headRef, err := repo.Head()
	if err != nil {
		errObj.New(rpcInternalError, "Failed to retrieve HEAD", err.Error())
		return
	}

	commitIter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		errObj.New(rpcInternalError, "Failed to retrieve commit history", err.Error())
		return
	}

	var commitList []RepoCommitInfo
	count := 0
	skipped := 0

	err = commitIter.ForEach(func(curCommit *object.Commit) error {
		// Skip to current requested offset
		if skipped < commitListOffset {
			skipped++
			return nil
		}

		// Hit requested limit
		if count >= maxCommits {
			return storer.ErrStop
		}

		commitInfo, err := buildWebRepoCommitInfo(repo, curCommit.Hash)
		if err != nil {
			// Log and skip bad commits
			logctx.LogStdErr(baseCtx, "Failed to build commit info for %s: %w\n", curCommit.Hash.String(), err)
			return nil
		}

		commitList = append(commitList, commitInfo)
		count++
		return nil
	})
	if err != nil && err != storer.ErrStop {
		errObj.New(rpcInternalError, "Failed to iterate commit history", err.Error())
		return
	}

	resp = commitList
	return
}

func repoDiffAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[RepoFileDiffReq](fullReq.Params, "req", "RepoFileDiffReq")

	cleanRequestPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid path", err.Error())
		return
	}

	repoPath, err := gitinternal.RetrieveRepoPath(clientCtx)
	if err != nil {
		errObj.New(rpcInternalError, "Failed retrieving repository path", err.Error())
		return
	}

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to refresh repository", err.Error())
		return
	}

	// Validate both commits
	validBase := parsing.IsHex40(req.BaseCommitHash)
	if !validBase {
		errObj.New(rpcInvalidParams, "Invalid request", "requested base commit hash is not a valid hash")
		return
	}
	validTgt := parsing.IsHex40(req.TgtCommitHash)
	if !validTgt && req.TgtCommitHash != "" {
		errObj.New(rpcInvalidParams, "Invalid request", "requested target commit hash is not a valid hash")
		return
	}

	patch, err := getPatchFromCommits(repo, req.BaseCommitHash, req.TgtCommitHash)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to get commit patch", err.Error())
		return
	}

	var diffs RepoFileDiffResp

	for _, filePatch := range patch.FilePatches() {
		from, to := filePatch.Files()

		oldPath := ""
		newPath := ""
		if from != nil {
			oldPath = from.Path()
		}
		if to != nil {
			newPath = to.Path()
		}

		// Filter for requested path
		if cleanRequestPath != oldPath && cleanRequestPath != newPath {
			continue
		}

		// Determine change type
		changeType := "modified"
		switch {
		case from == nil:
			changeType = "added"
		case to == nil:
			changeType = "deleted"
		case from.Path() != to.Path():
			changeType = "renamed"
		}

		diffFile := DiffFile{
			OldPath:    oldPath,
			NewPath:    newPath,
			ChangeType: changeType,
			IsBinary:   filePatch.IsBinary(),
		}

		if !filePatch.IsBinary() {
			for _, chunk := range filePatch.Chunks() {
				lines := strings.Split(chunk.Content(), "\n")

				for _, line := range lines {
					// Skip empty lines at the end of a chunk
					if line == "" {
						continue
					}

					var change LineChange
					switch chunk.Type() {
					case diff.Add:
						change = LineChange{Type: "add", Content: line}
					case diff.Delete:
						change = LineChange{Type: "del", Content: line}
					case diff.Equal:
						change = LineChange{Type: "context", Content: line}
					}

					// Add to a single hunk (for simplicity)
					if len(diffFile.Hunks) == 0 {
						diffFile.Hunks = append(diffFile.Hunks, DiffHunk{
							OldStartLine: 0,
							OldLineCount: 0,
							NewStartLine: 0,
							NewLineCount: 0,
							Changes:      []LineChange{},
						})
					}
					diffFile.Hunks[0].Changes = append(diffFile.Hunks[0].Changes, change)
				}
			}
		}

		diffs.Files = append(diffs.Files, diffFile)
		break
	}

	resp = diffs
	return
}

func getPatchFromCommits(repo *git.Repository, baseHash, targetHash string) (*object.Patch, error) {
	baseCommitID := plumbing.NewHash(baseHash)
	tgtCommitID := plumbing.NewHash(targetHash)

	baseCommit, err := repo.CommitObject(baseCommitID)
	if err != nil {
		return nil, fmt.Errorf("failed to get base commit: %w", err)
	}

	targetCommit, err := repo.CommitObject(tgtCommitID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target commit: %w", err)
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get base tree: %w", err)
	}

	targetTree, err := targetCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get target tree: %w", err)
	}

	patch, err := baseTree.Patch(targetTree)
	if err != nil {
		return nil, fmt.Errorf("failed to generate patch: %w", err)
	}

	return patch, nil
}
