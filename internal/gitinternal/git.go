// Package for interacting with git repositories
package gitinternal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/internal/config"
	"scmp/internal/fsops"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Gets absolute path to the root of the git repository using the current working directory
func RetrieveRepoPath(ctx context.Context) (repoPath string, err error) {
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Retrieving repository file path\n")

	// When we already have the repo path set in context, do not use current directory
	_, confPresent := ctx.Value(global.ConfKey).(config.Config)
	if confPresent {
		config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
		if config.RepositoryPath != "" {
			config.RepositoryPath, err = fsops.ExpandHomeDirectory(config.RepositoryPath)
			if err != nil {
				err = fmt.Errorf("failed parsing existing repository path: %w", err)
				return
			}
			repoPath = config.RepositoryPath
			return
		}
	}

	// Get current dir (expected to be root of git repo)
	currentWorkingDir, err := os.Getwd()
	if err != nil {
		return
	}
	expectedDotGitPath := filepath.Join(currentWorkingDir, ".git")

	// Error if .git directory is not present in current directory
	_, err = os.Stat(expectedDotGitPath)
	if os.IsNotExist(err) {
		err = fmt.Errorf("not in the root of a git repository, unable to continue")
		return
	} else if err != nil {
		return
	}

	// Guard against empty repo path
	if currentWorkingDir == "" {
		err = fmt.Errorf("failed to retrieve git repository path")
		return
	}

	// Current dir is absolute git repo path
	repoPath = currentWorkingDir
	return
}

// Retrieves working tree and worktree status using git repository in current working directory
func OpenCWD(ctx context.Context) (worktree *git.Worktree, status git.Status, err error) {
	// Check working dir for git repo
	repoPath, err := RetrieveRepoPath(ctx)
	if err != nil {
		return
	}

	// Open repository
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return
	}

	// Get working tree
	worktree, err = repo.Worktree()
	if err != nil {
		return
	}

	// Get worktree status
	status, err = worktree.Status()

	return
}

// Adds changes based on user glob to the working tree
func Add(ctx context.Context, addGlob string) (err error) {
	// Retrieve required options
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Need repository path for artifact processing
	repoPath, err := RetrieveRepoPath(ctx)
	if err != nil {
		return
	}

	// Check for artifacts and update pointers if required
	err = ArtifactTracking(ctx, repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed artifact tracking: %v\n", err)
		os.Exit(1)
	}

	// Retrieve working tree
	worktree, status, err := OpenCWD(ctx)
	if err != nil {
		return
	}

	// Return early if nothing to add
	if status.IsClean() {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "nothing to add, working tree clean\n")
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Raw add option: '%s'\n", addGlob)

	// Exit if dry-run requested
	if opts.DryRunEnabled {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Dry-run requested, not altering worktree\n")
		return
	}

	// Add all files to worktree
	err = worktree.AddGlob(addGlob)
	if err != nil {
		return
	}

	return
}

// Commit only already added worktree items
func Commit(ctx context.Context, gitCommitAction string) (err error) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSGit)

	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	// Retrieve working tree
	worktree, status, err := OpenCWD(ctx)
	if err != nil {
		return
	}

	// Return early if nothing to commit
	if status.IsClean() {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "nothing to commit, working tree clean\n")
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "Raw commit option: '%s'\n", gitCommitAction)

	// Retrieve commit message from user supplied file
	var commitMessage string
	if strings.HasPrefix(gitCommitAction, global.FileURIPrefix) {
		// Not adhering to actual URI standards -- I just want file paths
		pathToCommitMessage := strings.TrimPrefix(gitCommitAction, global.FileURIPrefix)

		// Check for ~/ and expand if required
		pathToCommitMessage, err = fsops.ExpandHomeDirectory(pathToCommitMessage)
		if err != nil {
			err = fmt.Errorf("failed to resolve absolute path for commit file '%s': %w", pathToCommitMessage, err)
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Retrieving commit message from file: '%s'\n", pathToCommitMessage)

		// Retrieve the file contents
		var fileBytes []byte
		fileBytes, err = os.ReadFile(pathToCommitMessage)
		if err != nil {
			return
		}

		// Convert file to string
		commitMessage = string(fileBytes)
	} else {
		// Correct text
		commitMessage = strings.Trim(gitCommitAction, "'\" \n\r")
	}

	// Retrieve any user details from context
	username := global.AssertFromContext[string](ctx, "username", global.UserKey, "string")
	var userEmail string
	if username != global.GlobalUsername {
		userEmail = global.AssertFromContext[string](ctx, "userEmail", global.EmailKey, "string")
	}

	// Set user details for commit - default to config otherwise
	var commitAuthor *object.Signature
	if username != "" && username != global.GlobalUsername {
		var newAuthor object.Signature
		newAuthor.Name = username
		newAuthor.Email = userEmail
		newAuthor.When = time.Now()
		commitAuthor = &newAuthor
	} else {
		commitAuthor = nil
	}

	// Return if dry-run requested
	if opts.DryRunEnabled {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Dry-run requested, not committing\n")
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Received commit message: '%s'\n", commitMessage)
		if commitAuthor.Name != "" {
			logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Would commit under user: %s (%s)\n", commitAuthor.Name, commitAuthor.Email)
		}
		return
	}

	// Commit changes
	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		Author:            commitAuthor,
		AllowEmptyCommits: false,
	})
	if err != nil {
		return
	}

	return
}

// Opens repository and retrieves details about given commit
// If commitID is empty, will default to using HEAD commit
func GetCommit(ctx context.Context, commitID *string) (tree *object.Tree, commit *object.Commit, err error) {
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Retrieving commit and tree from git repository\n")

	repoPath, err := RetrieveRepoPath(ctx)
	if err != nil {
		return
	}

	// Open the repository
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		err = fmt.Errorf("unable to open repository: %w", err)
		return
	}

	// If no commitID, assume they want to use the HEAD commit
	if *commitID == "" {
		// Get the pointer to the HEAD commit
		var ref *plumbing.Reference
		ref, err = repo.Head()
		if err != nil {
			err = fmt.Errorf("unable to get HEAD reference: %w", err)
			return
		}

		// Set HEAD commitID
		*commitID = ref.Hash().String()
	}

	// Verify commit ID string content
	if !parsing.IsHex40(*commitID) {
		err = fmt.Errorf("invalid commit ID: hash is not 40 characters and/or is not hexadecimal")
		return
	}

	// Set hash
	commitHash := plumbing.NewHash(*commitID)

	// Get the commit
	commit, err = repo.CommitObject(commitHash)
	if err != nil {
		err = fmt.Errorf("unable to get commit object: %w", err)
		return
	}

	// Get the tree from the commit
	tree, err = commit.Tree()
	if err != nil {
		err = fmt.Errorf("unable to get commit tree: %w", err)
		return
	}

	return
}

// Resets HEAD to previous commit without changing working directory
// Only roll back commit if the program was started by a hook and if the commit rollback is requested
// Reset commit because the current commit should reflect what is deployed in the network
// Conceptually, the rough equivalent of this command: git reset --soft HEAD~1
func RollBackOneCommit(ctx context.Context, commitID string, calledByGitHook bool, rollback bool) (err error) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSGit)

	// Not in a hook or no rollback needed
	if !calledByGitHook || !rollback {
		return
	}

	repoPath, err := RetrieveRepoPath(ctx)
	if err != nil {
		return
	}

	// Warn user
	fmt.Printf("WARNING: Removing current repository commit due to processing error.\n")
	fmt.Printf("         Working directory is **NOT** affected.\n")

	// Open the repo
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		err = fmt.Errorf("failed to open repository: %w", err)
		return
	}

	// Get the current branch reference
	currentBranchReference, err := repo.Reference(plumbing.ReferenceName("HEAD"), true)
	if err != nil {
		err = fmt.Errorf("failed to get branch name from HEAD commit: %w", err)
		return
	}

	// If user had tried deployment with a past commit, do not permit rolling back
	if currentBranchReference.Hash().String() != commitID {
		err = fmt.Errorf("refusing to rollback HEAD commit when deploy attempt was not using HEAD commit")
		return
	}

	// Get the branch HEAD commit
	currentBranchHeadCommit, err := repo.CommitObject(currentBranchReference.Hash())
	if err != nil {
		err = fmt.Errorf("failed to get HEAD commit: %w", err)
		return
	}

	// Ensure a previous commit exists before retrieve the hash
	if len(currentBranchHeadCommit.ParentHashes) == 0 {
		err = fmt.Errorf("head does not have a previous commit")
		return
	}

	// Get the previous commit hash
	previousCommitHash := currentBranchHeadCommit.ParentHashes[0]

	// Get the branch short name
	currentBranchNameString := currentBranchReference.Name()

	// Create new reference with the current branch and previous commit hash
	newBranchReference := plumbing.NewHashReference(plumbing.ReferenceName(currentBranchNameString), previousCommitHash)

	// Reset HEAD of current branch to previous commit
	err = repo.Storer.SetReference(newBranchReference)
	if err != nil {
		err = fmt.Errorf("failed to roll back current commit to previous commit: %w", err)
		return
	}

	// Tell user how to continue
	fmt.Printf("Please fix the below error then `git add` and `git commit` then restart the deployment process.\n")

	return
}

func StatusCodeToString(code git.StatusCode) (status string) {
	var statusCodeNames = map[git.StatusCode]string{
		git.Unmodified:         "unmodified",
		git.Untracked:          "new",
		git.Modified:           "changed",
		git.Added:              "new",
		git.Deleted:            "deleted",
		git.Renamed:            "renamed",
		git.Copied:             "copied",
		git.UpdatedButUnmerged: "conflict",
	}

	status, validCode := statusCodeNames[code]
	if !validCode {
		status = "unknown"
	}

	return
}
