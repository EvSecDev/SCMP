package predeploy

import (
	"context"
	"os"
	"scmp/core/filesystem"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/str"
	"strings"
)

// Splits host directory name from the expected target file path
// Requires localRepoPath be a relative path without leading slashes
// Returned targetFilePath will contain a leading slash
// Path separators are linux ("/")
// Function does not return errors, but unexpected input will return nil outputs
func translateLocalPathtoRemotePath(ctx context.Context, localRepoPath str.LocalRepoPath) (hostDir str.RepoRootDir, targetFilePath str.RemotePath) {
	config := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	// Enforce type at function boundary, but otherwise convert back for use here
	repoPath := string(localRepoPath)

	// Remove .remote-artifact extension if applicable
	repoPath = strings.TrimSuffix(repoPath, string(filesystem.ArtifactPointerFileExt))

	// Remove .directory_metadata_information.json
	repoPath = strings.TrimSuffix(repoPath, string(filesystem.DirMetaFileName))

	// Format repoFilePath with the expected host path separators
	repoPath = strings.ReplaceAll(repoPath, string(os.PathSeparator), "/")

	// Remove any trailing slashes
	repoPath = strings.TrimSuffix(repoPath, "/")

	// Remove repository path if its absolute local path
	if strings.HasPrefix(repoPath, config.RepositoryPath) {
		repoPath = strings.TrimPrefix(repoPath, config.RepositoryPath)
		repoPath = strings.TrimPrefix(repoPath, "/")
	}

	// Bad - Disallow relative paths
	if strings.Contains(repoPath, "../") {
		return
	}

	// Bad - not a path, just a name
	if !strings.Contains(repoPath, "/") {
		return
	}

	// Separate on first occurrence of path separator
	pathSplit := strings.SplitN(repoPath, "/", 2)

	// Bad - only accept length of 2
	if len(pathSplit) != 2 {
		return
	}

	// Bad - trailing slash no actual content
	if pathSplit[1] == "" {
		return
	}

	// Retrieve the first array item as the host directory name
	hostDir = str.RepoRootDir(pathSplit[0])

	// Allow relative paths within hosts (needed for relative symlinks)
	_, parentDirIsHost := config.HostInfo[hostDir]
	_, parentDirIsUniversal := config.AllUniversalGroups[hostDir]
	if !parentDirIsHost && !parentDirIsUniversal && hostDir != config.UniversalDirectory {
		targetFilePath = str.RemotePath(repoPath)
		return
	}

	// Retrieve the second array item as the expected target path
	targetFilePath = str.RemotePath(pathSplit[1])

	// Add leading slash to path
	targetFilePath = "/" + targetFilePath
	return
}
