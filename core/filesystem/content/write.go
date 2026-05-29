// Package for file content (header-aware) modifications
package content

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/filesystem"
	"scmp/internal/logctx"
	"scmp/internal/parsing"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"strings"
)

// Writes file contents to repository file with added metadata header
// File content optional
func WriteRepoFile(ctx context.Context, localFilePath str.LocalRepoPath, metadata filesystem.MetaHeader, fileContent *[]byte) (err error) {
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Adding JSON metadata header to file '%s'\n", localFilePath)

	var startDelimiter, endDelimiter, metaPrefix string
	switch str.FilePathExt(localFilePath) {
	case ".sh", ".zsh", ".bashrc", ".zshrc", ".yaml", ".yml", ".py":
		metaPrefix = "#"
		startDelimiter = filesystem.MetaDelimiter
		endDelimiter = filesystem.MetaDelimiter
	case ".html", ".htm", ".xml":
		startDelimiter = "<!--" + filesystem.MetaDelimiter
		endDelimiter = filesystem.MetaDelimiter + "-->"
	case ".go", ".css", ".js", ".php":
		startDelimiter = "/*" + filesystem.MetaDelimiter
		endDelimiter = filesystem.MetaDelimiter + "*/"
	default:
		startDelimiter = filesystem.MetaDelimiter
		endDelimiter = filesystem.MetaDelimiter
	}

	metaHeaderBytes, err := json.MarshalIndent(metadata, metaPrefix, "  ")
	if err != nil {
		err = fmt.Errorf("error parsing metadata header: %w", err)
		return
	}
	metaHeaderBytes = parsing.UnescapeShellRedirectors(metaHeaderBytes)
	header := string(metaHeaderBytes)

	var fullFileContent strings.Builder
	fullFileContent.WriteString(startDelimiter)
	fullFileContent.WriteString("\n")
	fullFileContent.WriteString(metaPrefix) // JSON MarshalIndent does not add prefix to the first line
	fullFileContent.WriteString(header)
	fullFileContent.WriteString("\n")
	fullFileContent.WriteString(endDelimiter)
	fullFileContent.WriteString("\n")
	if fileContent != nil {
		fullFileContent.WriteString(string(*fileContent))
	}

	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Writing file '%s' to repository\n", localFilePath)

	configParentDirs := filepath.Dir(string(localFilePath))
	err = os.MkdirAll(configParentDirs, 0700)
	if err != nil {
		err = fmt.Errorf("failed to create missing parent directories in local repository: %w", err)
		return
	}

	localFile, err := os.OpenFile(string(localFilePath), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		err = fmt.Errorf("failed to open/create directory metadata file: %w", err)
		return
	}
	defer func() {
		_ = localFile.Close()
	}()

	_, err = localFile.WriteString(fullFileContent.String())
	if err != nil {
		err = fmt.Errorf("failed to write file to local repository: %w", err)
		return
	}

	return
}

func WriteSymbolicLinkToRepo(ctx context.Context, localLinkPath str.LocalRepoPath, selectionMetadata sshinternal.RemoteFileInfo) (err error) {
	var linkMetadata filesystem.MetaHeader
	linkMetadata.SymbolicLinkTarget = selectionMetadata.LinkTarget
	linkMetadata.TargetFileOwnerGroup = "root:root"
	linkMetadata.TargetFilePermissions = 777

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Symbolic Link '%s': Target: %s\n", localLinkPath, selectionMetadata.LinkTarget)

	err = WriteRepoFile(ctx, localLinkPath, linkMetadata, nil)
	if err != nil {
		err = fmt.Errorf("failed to write directory metadata to local repository: %w", err)
		return
	}

	return
}

// Writes directory metadata of chosen dir to repo
func WriteNewDirectoryMetadata(ctx context.Context, localDirPath str.LocalRepoPath, selectionMetadata sshinternal.RemoteFileInfo) (err error) {
	var dirMetadata filesystem.MetaHeader
	dirMetadata.TargetFileOwnerGroup = selectionMetadata.Owner + ":" + selectionMetadata.Group
	dirMetadata.TargetFilePermissions = selectionMetadata.Permissions

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "  Directory '%s': Metadata: %d %s\n", localDirPath, selectionMetadata.Permissions, dirMetadata.TargetFileOwnerGroup)

	metadataFile := str.FilePathJoin(localDirPath, filesystem.DirMetaFileName)

	err = WriteRepoFile(ctx, metadataFile, dirMetadata, nil)
	if err != nil {
		err = fmt.Errorf("failed to write directory metadata to local repository: %w", err)
		return
	}

	return
}
