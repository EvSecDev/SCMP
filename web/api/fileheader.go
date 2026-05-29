package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/deployment/remote"
	"scmp/core/filesystem"
	"scmp/core/filesystem/content"
	"scmp/core/filesystem/metadata"
	"scmp/internal/global"
	"scmp/internal/str"
	"scmp/web/internal"
	"time"
)

func fileMetadataGetAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[PathRequest](fullReq.Params, "req", "PathRequest")

	cleanRequestPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	fileInfo, err := os.Stat(cleanRequestPath)
	if err != nil {
		errObj.New(rpcInternalError, "Error retrieving info for file "+cleanRequestPath, err.Error())
		return
	}

	metadata, data, err := extractExistingContent(cleanRequestPath, fileInfo)
	if err != nil {
		errObj.New(rpcInternalError, "Failed extracting metadata", err.Error())
		return
	}

	modTime := fileInfo.ModTime().Format(time.RFC3339)

	resp = translateHeaderToWebMeta(cleanRequestPath, remote.FileType, modTime, metadata, &data)
	return
}

func fileMetadataEditAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[FileMetadata](fullReq.Params, "req", "FileMetadata")

	newHeader, err := translateWebMetaToHeader(req)
	if err != nil {
		errObj.New(rpcInternalError, "Invalid metadata header", err.Error())
		return
	}

	cleanRequestPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	fileInfo, err := os.Stat(cleanRequestPath)
	if err != nil {
		errObj.New(rpcInternalError, "Error retrieving info for file "+cleanRequestPath, err.Error())
		return
	}

	_, data, err := extractExistingContent(cleanRequestPath, fileInfo)
	if err != nil {
		errObj.New(rpcInternalError, "Failed extracting metadata", err.Error())
		return
	}

	err = content.WriteRepoFile(clientCtx, str.LocalRepoPath(cleanRequestPath), newHeader, &data)
	if err != nil {
		errObj.New(rpcInternalError, "Failed updating metadata", err.Error())
		return
	}

	resp = NilSuccess{}
	return
}

func extractExistingContent(filePath string, fileInfo os.FileInfo) (metaHeader filesystem.MetaHeader, contentSection []byte, err error) {
	var fileType string
	if fileInfo.IsDir() {
		fileType = "directory"
	} else {
		fileType = "file"
	}

	// Allow differentiating between the file itself and the metadata location
	// useful only for dirs, but keeps compatibility with regular files
	var metadataFilePath string

	// Change path to dir meta file
	if fileType == "directory" {
		metadataFilePath = filepath.Join(filePath, string(filesystem.DirMetaFileName))
	} else {
		metadataFilePath = filePath
	}

	reqFile, err := os.ReadFile(metadataFilePath)
	if err != nil {
		err = fmt.Errorf("failed reading file: %w", err)
		return
	}

	metaHeader, contentSection, err = metadata.Extract(string(reqFile))
	if err != nil {
		// Only error when the file had a header
		if err.Error() == "json start delimiter missing" {
			contentSection = reqFile
			err = nil
		} else {
			err = fmt.Errorf("failed parsing metadata: %w", err)
			return
		}
	}
	return
}
