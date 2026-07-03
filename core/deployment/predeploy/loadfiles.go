package predeploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"scmp/core/deployment"
	"scmp/core/filesystem/metadata"
	"scmp/internal/config"
	"scmp/internal/crypto"
	"scmp/internal/fsops"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// Retrieves all file content for this deployment
func LoadGitFileContent(ctx context.Context, allDeploymentFiles map[str.LocalRepoPath]str.DeployAction, tree *object.Tree) (rawFileContent map[str.LocalRepoPath][]byte, err error) {
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Loading files for deployment... \n")

	rawFileContent = make(map[str.LocalRepoPath][]byte)

	for repoFilePath, commitFileAction := range allDeploymentFiles {
		if commitFileAction == deployment.ActionDelete {
			continue
		}

		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Loading repository file %s\n", repoFilePath)

		// Get file from git tree
		file, lerr := tree.File(string(repoFilePath))
		if lerr != nil {
			err = fmt.Errorf("failed retrieving file information from git tree: %w", lerr)
			return
		}

		reader, lerr := file.Reader()
		if lerr != nil {
			err = fmt.Errorf("failed retrieving file reader: %w", lerr)
			return
		}
		defer func() {
			lerr := reader.Close()
			if err == nil && lerr != nil {
				err = lerr
			}
		}()

		content, lerr := io.ReadAll(reader)
		if lerr != nil {
			err = fmt.Errorf("failed reading file content: %w", lerr)
			return
		}

		rawFileContent[repoFilePath] = content
	}

	return
}

// Loads artifact file contents and uses hash in pointer file
func loadArtifactContent(artifactPath string, artifactPointerPath str.LocalRepoPath, artifactPointerContent []byte, deployFiles *deployment.AllFiles) (content []byte, trackedHash str.FileID, err error) {
	// Only allow file URIs for now
	if !strings.HasPrefix(artifactPath, global.FileURIPrefix) {
		err = fmt.Errorf("remote-artifact file '%s': must use '%s' before file paths in 'ExternalContentLocation' field", artifactPointerPath, global.FileURIPrefix)
		return
	}

	// Use hash already in pointer file as hash of actual artifact file contents
	validHash, hash := parsing.HasHex64Prefix(string(artifactPointerContent))
	if !validHash {
		err = fmt.Errorf("invalid hash retrieved from remote-artifact file '%s'", artifactPointerPath)
		return
	}
	trackedHash = str.FileID(hash)

	// Retrieve artifact file data if not already loaded
	if !deployFiles.AlreadyLoaded(trackedHash) {
		// Not adhering to actual URI standards -- I just want file paths
		artifactFileName := strings.TrimPrefix(artifactPath, global.FileURIPrefix)
		artifactFileName, err = fsops.ExpandHomeDirectory(artifactFileName)
		if err != nil {
			err = fmt.Errorf("failed to resolve absolute path for '%s': %w", artifactFileName, err)
			return
		}

		// Re-hash the content against git-backed hash to ensure we are not deploying a different version
		hash, err = crypto.SHA256SumStream(artifactFileName)
		if err != nil {
			err = fmt.Errorf("failed to hash current artifact file contents: %w", err)
			return
		}

		actualHash := str.FileID(hash)
		if trackedHash != actualHash {
			err = fmt.Errorf("artifact '%s': repository is tracking artifact hash that is different than actual hash: expected: '%s' current: '%s'",
				artifactFileName, trackedHash[:16], actualHash[:16])
			return
		}

		// Retrieve artifact file contents
		content, err = os.ReadFile(artifactFileName)
		if err != nil {
			return
		}
	}
	return
}

// Parses loaded file content and retrieves needed metadata
// Return vales provide the content keyed on local file path for the file data, metadata, hashes, and actions
func ParseFileContent(ctx context.Context, allDeploymentFiles map[str.LocalRepoPath]str.DeployAction, rawFileContent map[str.LocalRepoPath][]byte) (deployFiles *deployment.AllFiles, err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	ctx = logctx.AppendCtxTag(ctx, logctx.NSParsing)
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Parsing files for deployment... \n")

	// Initialize maps
	deployFiles = deployment.NewAllFiles()

	// Load file contents, metadata, hashes, and actions into their own maps
	for repoFilePath, commitFileAction := range allDeploymentFiles {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Parsing repository file %s\n", repoFilePath)
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Marked as '%s'\n", commitFileAction)

		// Actions that do not require content loading
		if commitFileAction == deployment.ActionDelete {
			// Add it to the deploy target files so it can be deleted during ssh
			_, deletedFilePath := parsing.TranslateLocalPathtoRemotePath(cfg.RepositoryPath, repoFilePath)
			deployFiles.AddMetadata(repoFilePath, deployment.FileInfo{Action: commitFileAction, RepoFilePath: repoFilePath, TargetFilePath: deletedFilePath})
			continue
		} else if commitFileAction != deployment.ActionCreate &&
			commitFileAction != deployment.ActionDirCreate &&
			commitFileAction != deployment.ActionDirModify {
			// Skip unsupported file types - safety blocker
			continue
		}

		content := rawFileContent[repoFilePath]

		// Retrieve metadata depending on if this is a directory or a file
		jsonMetadata, fileContent, lerr := metadata.Extract(string(content))
		if lerr != nil {
			err = fmt.Errorf("file '%s': failed to separate metadata from file content: %w", repoFilePath, lerr)
			return
		}

		// Retrieve actual artifact contents and hash
		var contentIdentifier str.FileID
		if len(jsonMetadata.ExternalContentLocation) > 0 {
			fileContent, contentIdentifier, err = loadArtifactContent(jsonMetadata.ExternalContentLocation, repoFilePath, fileContent, deployFiles)
			if err != nil {
				err = fmt.Errorf("failed to load artifact file content: %w", err)
				return
			}
		} else if len(fileContent) > 0 {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "Hashing file '%s' content\n", repoFilePath)

			// Hash the metadata-less contents
			contentIdentifier = str.FileID(crypto.SHA256Sum(fileContent))
		}

		// Put all metadata gathered into map
		metadata := jsonToFileInfo(ctx, repoFilePath, jsonMetadata, len(fileContent), commitFileAction, contentIdentifier)
		deployFiles.AddMetadata(repoFilePath, metadata)

		// Put file content into map (only applies to file creation)
		if len(fileContent) > 0 && commitFileAction == deployment.ActionCreate {
			deployFiles.StoreDataOnce(contentIdentifier, fileContent)
		}
	}

	// Guard against empty return value
	if deployFiles.IsEmpty() {
		err = fmt.Errorf("something went wrong, no files available to load")
		return
	}

	return
}
