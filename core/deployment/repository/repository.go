// Package for parsing git state into actionable deployments
package repository

import (
	"context"
	"fmt"
	"io"
	"os"
	"scmp/core/deployment"
	"scmp/core/filesystem"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Retrieves file paths and file mode for a given commit
func GetChangedFiles(ctx context.Context, commit *object.Commit) (changedFiles []GitChangedFileMetadata, err error) {
	ctx = logctx.AppendCtxTag(ctx, logctx.NSRepo)
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Retrieving changed files from commit... \n")

	parentCommit, err := commit.Parents().Next()
	if err != nil {
		err = fmt.Errorf("failed retrieving parent commit: %w", err)
		return
	}

	// Get the diff between the commits
	patch, err := parentCommit.Patch(commit)
	if err != nil {
		err = fmt.Errorf("failed retrieving difference between commits: %w", err)
		return
	}

	for _, file := range patch.FilePatches() {
		var changedFile GitChangedFileMetadata

		from, to := file.Files()

		// Must safely retrieve file information to avoid panic
		if from != nil {
			_, err = os.Stat(string(changedFile.fromPath))
			if err != nil {
				// Any error other than file is not present, return
				if !strings.Contains(err.Error(), "no such file or directory") {
					return
				}
				err = nil

				// Actual on-disk file is missing
				changedFile.fromNotOnFS = true
			}

			changedFile.fromPath = str.LocalRepoPath(from.Path())
			changedFile.fromMode = from.Mode()
		}
		if to != nil {
			_, err = os.Stat(string(changedFile.fromPath))
			if err != nil {
				// Any error other than file is not present, return
				if !strings.Contains(err.Error(), "no such file or directory") {
					return
				}
				err = nil

				// Actual on-disk file is missing
				changedFile.toNotOnFS = true
			}

			changedFile.toPath = str.LocalRepoPath(to.Path())
			changedFile.toMode = to.Mode()
		}

		changedFiles = append(changedFiles, changedFile)
	}
	return
}

// Parses changed files according to presence, path, and mode validity
// Marks files with create/delete/modify action for deployment
func ParseChangedFiles(ctx context.Context, changedFiles []GitChangedFileMetadata, fileOverride string) (commitFiles map[str.LocalRepoPath]str.DeployAction) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSRepo)
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Parsing commit files\n")

	commitFiles = make(map[str.LocalRepoPath]str.DeployAction)

	for _, changedFile := range changedFiles {
		// If either from/to path matches user request, continue parsing
		// TODO:
		//  If user provided override for a deleted file
		//   (that was moved, so source was deleted, destination is still present)
		//   both the deletion and the moved file will be added to deployment
		//  So user specifying only delete will actually get delete and create
		skipFromFile := parsing.CheckForOverride(ctx, fileOverride, string(changedFile.fromPath), cfg.HostInfo)
		skipToFile := parsing.CheckForOverride(ctx, fileOverride, string(changedFile.toPath), cfg.HostInfo)
		if skipToFile && skipFromFile {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "  File not desired\n")
			continue
		}

		fromFileIsValid := fileIsValid(ctx, changedFile.fromPath, changedFile.fromMode.String())
		toFileIsValid := fileIsValid(ctx, changedFile.toPath, changedFile.toMode.String())

		if changedFile.fromPath == "" && changedFile.toPath == "" {
			continue
		} else if changedFile.fromPath == "" && toFileIsValid {
			// Newly created files
			//   like `touch etc/file.txt`
			if str.HasSuffix(changedFile.toPath, filesystem.DirMetaFileName) {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
					"  Dir Metadata '%s' is brand new and will affect parent\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = deployment.ActionDirCreate
			} else {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
					"  File '%s' is brand new and to be created\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = deployment.ActionCreate
			}
		} else if changedFile.toPath == "" && fromFileIsValid {
			// Deleted Files
			//   like `rm etc/file.txt`
			if opts.AllowDeletions {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
					"  File '%s' is to be deleted\n", changedFile.fromPath)
				commitFiles[changedFile.fromPath] = deployment.ActionDelete
			} else {
				logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
					"  Skipping deletion of file '%s'\n", changedFile.fromPath)
			}
		} else if changedFile.fromPath != changedFile.toPath && fromFileIsValid && toFileIsValid {
			// Copied or renamed files
			//   like `cp etc/file.txt etc/file2.txt` or `mv etc/file.txt etc/file2.txt`

			if changedFile.fromNotOnFS {
				fromDirs := strings.Split(string(changedFile.fromPath), "/")
				topLevelDirFrom := fromDirs[0]
				toDirs := strings.Split(string(changedFile.toPath), "/")
				topLevelDirTo := toDirs[0]

				if topLevelDirFrom != topLevelDirTo {
					// File was moved between hosts - must remove source
					if opts.AllowDeletions {
						logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
							"  File '%s' is to be deleted\n", changedFile.fromPath)
						commitFiles[changedFile.fromPath] = deployment.ActionDelete
					} else {
						logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
							"  Skipping deletion of file '%s'\n", changedFile.fromPath)
					}
				} else if opts.AllowDeletions {
					logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
						"  File '%s' is to be deleted\n", changedFile.fromPath)
					commitFiles[changedFile.fromPath] = deployment.ActionDelete
				}
			}

			if str.HasSuffix(changedFile.toPath, filesystem.DirMetaFileName) {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
					"  Dir Metadata '%s' is modified and will modify target directory\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = deployment.ActionDirModify
			} else {
				logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog,
					"  File '%s' is modified and to be created\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = deployment.ActionCreate
			}
		} else if changedFile.fromPath == changedFile.toPath && fromFileIsValid && toFileIsValid {
			// Edited in place
			//   like `nano etc/file.txt`
			if str.HasSuffix(changedFile.toPath, filesystem.DirMetaFileName) {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
					"  Dir Metadata '%s' is modified in place and will modify target directory\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = deployment.ActionDirModify
			} else {
				logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
					"  File '%s' is modified in place and to be created\n", changedFile.toPath)
				commitFiles[changedFile.toPath] = deployment.ActionCreate
			}
		} else {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
				"  File '%s' unsupported\n", changedFile.fromPath)
		}
	}

	return
}

// Retrieves all files for current commit (regardless if changed)
// This is used to also get all files in commit for deployment of unchanged files when requested
func GetRepoFiles(ctx context.Context, tree *object.Tree, fileOverride string) (commitFiles map[str.LocalRepoPath]str.DeployAction, err error) {
	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	// Initialize maps
	commitFiles = make(map[str.LocalRepoPath]str.DeployAction)

	// Get list of all files in repo tree
	allFiles := tree.Files()

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Retrieving all files in repository\n")

	// Use all repository files to create map and array of files/hosts
	for {
		// Go to next file in list
		var repoFile *object.File
		repoFile, err = allFiles.Next()
		if err != nil {
			// Break at end of list
			if err == io.EOF {
				err = nil
				break
			}

			// Fail if next file doesn't work
			err = fmt.Errorf("failed retrieving commit file: %w", err)
			return
		}

		// Get file path
		repoFilePath := str.LocalRepoPath(repoFile.Name)

		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Filtering file %s\n", repoFilePath)

		if !fileIsValid(ctx, repoFilePath, repoFile.Mode.String()) {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "    File not valid\n")
			continue
		}

		// Skip file if not user requested file (if requested)
		skipFile := parsing.CheckForOverride(ctx, fileOverride, string(repoFilePath), config.HostInfo)
		if skipFile {
			logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "    File not desired\n")
			continue
		}

		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    File available\n")

		// Decide if file is dir metadata or actual config
		if str.HasSuffix(repoFilePath, filesystem.DirMetaFileName) {
			commitFiles[repoFilePath] = deployment.ActionDirCreate
		} else {
			// Add repo file to the commit map with always create action
			commitFiles[repoFilePath] = deployment.ActionCreate
		}
	}

	return
}

// Retrieves file paths in maps per host and universal conf dir
func ParseAllRepoFiles(ctx context.Context, tree *object.Tree) (allHostsFiles map[str.RepoRootDir]map[str.RemotePath]struct{}, allUniversalFiles map[str.RepoRootDir]map[str.RemotePath]struct{}, err error) {
	// Retrieve files from commit tree
	repoFiles := tree.Files()

	// Initialize maps
	allHostsFiles = make(map[str.RepoRootDir]map[str.RemotePath]struct{})
	allUniversalFiles = make(map[str.RepoRootDir]map[str.RemotePath]struct{})

	// Retrieve all non-changed repository files for this host (and universal dir) for later deduping
	for {
		// Go to next file in list
		var repoFile *object.File
		repoFile, err = repoFiles.Next()
		if err != nil {
			// Break at end of list
			if err == io.EOF {
				err = nil
				break
			}

			// Fail if next file doesn't work
			err = fmt.Errorf("failed retrieving commit file: %w", err)
			return
		}

		// Parse out by host/universal
		mapFilesByHostOrUniversal(ctx, repoFile.Name, allHostsFiles, allUniversalFiles)
	}
	return
}

// Modifies input maps to divide up repository files between host directories and universal directories
func mapFilesByHostOrUniversal(ctx context.Context, repoFilePath string, allHostsFiles map[str.RepoRootDir]map[str.RemotePath]struct{}, allUniversalFiles map[str.RepoRootDir]map[str.RemotePath]struct{}) {
	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	// Split host dir and target path
	commitSplit := strings.SplitN(repoFilePath, string(os.PathSeparator), 2)

	// Skip repo files in root of repository
	if len(commitSplit) <= 1 {
		return
	}

	// Get host dir part and target file path part
	topLevelDirName := str.RepoRootDir(commitSplit[0])
	tgtFilePath := str.RemotePath(commitSplit[1])

	// Add files by universal group dirs to map for later deduping
	_, fileIsInUniversalGroup := config.AllUniversalGroups[topLevelDirName]
	if fileIsInUniversalGroup || topLevelDirName == config.UniversalDirectory {
		// Make map if inner map isn't initialized already
		_, dirAlreadyExistsInMap := allUniversalFiles[topLevelDirName]
		if !dirAlreadyExistsInMap {
			allUniversalFiles[topLevelDirName] = make(map[str.RemotePath]struct{})
		}

		// Repo file is under one of the universal group directories
		allUniversalFiles[topLevelDirName][tgtFilePath] = struct{}{}
		return
	}

	// Add files by their host to the map - make map if host map isn't initialized yet
	_, hostAlreadyExistsInMap := allHostsFiles[topLevelDirName]
	if !hostAlreadyExistsInMap {
		allHostsFiles[topLevelDirName] = make(map[str.RemotePath]struct{})
	}
	allHostsFiles[topLevelDirName][tgtFilePath] = struct{}{}
}

