package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/filesystem/content"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Sets up new git repository based on controller-expected directory format
// Also creates initial commit so the first deployment will have something to compare against
func NewRepository(ctx context.Context, repoPath string, initialBranchName string) {
	// Only take absolute paths from user choice
	absoluteRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get absolute path to new repository: %v\n", err)
		os.Exit(1)
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Creating new repository at %s\n", absoluteRepoPath)

	// Get individual dir names
	pathDirs := strings.Split(absoluteRepoPath, string(os.PathSeparator))

	// Error if it already exists
	_, err = os.Stat(absoluteRepoPath)
	if !os.IsNotExist(err) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create new repository: directory '%s' already exists\n", absoluteRepoPath)
			os.Exit(1)
		}
	}

	// Create repository directories if missing
	repoPath = ""
	for _, pathDir := range pathDirs {
		// Skip empty
		if pathDir == "" {
			continue
		}

		// Save current dir to main path
		repoPath = repoPath + string(os.PathSeparator) + pathDir

		// Check existence
		_, err := os.Stat(repoPath)
		if os.IsNotExist(err) {
			// Create if not exist
			err := os.Mkdir(repoPath, 0750)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create missing directory in repository path: %v\n", err)
				os.Exit(1)
			}
		}

		// Go to next dir in array
		pathDirs = pathDirs[:len(pathDirs)-1]
	}

	// Move into new repo directory
	err = os.Chdir(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to change into new repository directory: %v\n", err)
		os.Exit(1)
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Setting initial branch name to %s\n", initialBranchName)

	// Format branch name
	if initialBranchName != "refs/heads/"+initialBranchName {
		initialBranchName = "refs/heads/" + initialBranchName
	}

	// Set initial branch
	initialBranch := plumbing.ReferenceName(initialBranchName)
	initOptions := &git.InitOptions{
		DefaultBranch: initialBranch,
	}

	// Set git initial options
	plainInitOptions := &git.PlainInitOptions{
		InitOptions: *initOptions,
		Bare:        false,
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Initializing git repository\n")

	// Create git repo
	repo, err := git.PlainInitWithOptions(repoPath, plainInitOptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init git repository: %v\n", err)
		os.Exit(1)
	}

	// Read existing config options
	gitConfigPath := repoPath + "/.git/config"
	gitConfigFileBytes, err := os.ReadFile(gitConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read git config file: %v\n", err)
		os.Exit(1)
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Setting initial git repository configuration options\n")

	// Write options to config file if no garbage collection section
	if !strings.Contains(string(gitConfigFileBytes), "[gc]") {
		// Open git config file - APPEND
		gitConfigFile, err := os.OpenFile(gitConfigPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open git config file: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			_ = gitConfigFile.Close()
		}()

		// Define garbage collection section and options
		repoGCOptions := `[gc]
        auto = 10
        reflogExpire = 8.days
        reflogExpireUnreachable = 8.days
        pruneExpire = 16.days
`

		// Write (append) string
		_, err = gitConfigFile.WriteString(repoGCOptions + "\n")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write git garbage collection options: %v\n", err)
			os.Exit(1)
		}

		_ = gitConfigFile.Close()
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Adding example config metadata header files\n")

	// Create a working tree
	worktree, err := repo.Worktree()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create new git tree: %v\n", err)
		os.Exit(1)
	}

	// Example file
	const exampleFile string = ".example-metadata-header.txt"
	content.WriteTemplateFile(ctx, str.LocalRepoPath(exampleFile), true)

	// Stage the universal files
	_, err = worktree.Add(exampleFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add universal file: %v\n", err)
		os.Exit(1)
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Creating an initial commit in repository\n")

	// Create initial commit
	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  InternalCommitUserName,
			Email: InternalCommitUserEmail,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create first commit: %v\n", err)
		os.Exit(1)
	}

	logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Successfully created new git repository in %s\n", repoPath)
}
