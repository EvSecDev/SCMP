package api

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"scmp/core/filesystem"
	"scmp/core/filesystem/metadata"
	"scmp/internal/global"
	"scmp/web/internal"
	"strconv"
	"strings"
	"time"
)

func dirListAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[PathRequest](fullReq.Params, "req", "PathRequest")

	cleanRequestPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	var fileList []FileMetadata

	var dirItems []fs.DirEntry
	dirItems, err = os.ReadDir(cleanRequestPath)
	if err != nil {
		errObj.New(rpcInternalError, "Error walking directory "+cleanRequestPath, err.Error())
		return
	}

	for _, dirItem := range dirItems {
		// Full relative path
		itemPath := filepath.Join(cleanRequestPath, dirItem.Name())

		// Actual dir metafiles are not shown
		if strings.HasSuffix(itemPath, string(filesystem.DirMetaFileName)) {
			continue
		}

		// Don't show git dir (not accessible anyways)
		if dirItem.Name() == ".git" {
			continue
		}

		// Retrieve requested path type
		fileStat, err := os.Stat(itemPath)
		if err != nil {
			errObj.New(rpcInternalError, "Error retrieving local filesystem metadata for "+itemPath, err.Error())
			return
		}

		// Basic Metadata JSON
		var fileInfo FileMetadata

		if !fileStat.IsDir() {
			fileInfo, err = retrieveWebFileInfo(itemPath, fileStat)
			if err != nil {
				errObj.New(rpcInternalError, "Error retrieving file metadata for "+itemPath, err.Error())
				return
			}
		} else if fileStat.IsDir() {
			fileInfo, err = retrieveWebDirInfo(itemPath, fileStat)
			if err != nil {
				errObj.New(rpcInternalError, "Error retrieving directory metadata for "+itemPath, err.Error())
				return
			}
		}
		fileList = append(fileList, fileInfo)
	}

	// Send info back to client
	resp = fileList
	return
}

func retrieveWebFileInfo(file string, localInfo os.FileInfo) (info FileMetadata, err error) {
	// Read contents of this config file
	fileBytes, err := os.ReadFile(file)
	if err != nil {
		err = fmt.Errorf("unable to read '%s': %w", file, err)
		return
	}

	var localOnlyFile bool

	metaHeader, content, err := metadata.Extract(string(fileBytes))
	if err != nil {
		// Only error when the file had a header
		if err.Error() == "json start delimiter missing" {
			localOnlyFile = true
			err = nil
		} else {
			err = fmt.Errorf("failed parsing metadata header for '%s': %w", file, err)
			return
		}
	}

	info.Path = file
	info.Type = "file"

	if !localOnlyFile {
		ownership := metaHeader.TargetFileOwnerGroup
		OwnerGroup := strings.Split(ownership, ":")
		if len(OwnerGroup) != 2 {
			err = fmt.Errorf("invalid metadata header in file '%s'", file)
			return
		}
		info.OwnerName = OwnerGroup[0]
		info.GroupName = OwnerGroup[1]
		info.Permissions = strconv.Itoa(metaHeader.TargetFilePermissions)
		info.Size = len(content)
		if localInfo != nil {
			// Last mod time is always pulled from local OS
			info.LastModified = localInfo.ModTime().Format(time.RFC3339)
		}
	} else {
		info.OwnerName = "root"
		info.GroupName = "root"
		info.Permissions = "644"
		if localInfo != nil {
			// Last mod time is always pulled from local OS
			info.LastModified = localInfo.ModTime().Format(time.RFC3339)

			info.Size = int(localInfo.Size())
		}
	}

	return
}

func retrieveWebDirInfo(dir string, localInfo os.FileInfo) (info FileMetadata, err error) {
	// Dir metadata is in the metadata file under the dir
	dirMetadataFile := filepath.Join(dir, string(filesystem.DirMetaFileName))

	metaFileInfo, err := os.Stat(dirMetadataFile)
	if err == nil {
		// Dir metadata file exists
		var content []byte
		content, err = os.ReadFile(dirMetadataFile)
		if err != nil {
			err = fmt.Errorf("failed reading directory metadata")
			return
		}

		var metaHeader filesystem.MetaHeader
		metaHeader, _, err = metadata.Extract(string(content))
		if err != nil {
			err = fmt.Errorf("failed parsing directory metadata header")
			return
		}

		var modTime string
		if metaFileInfo != nil {
			// Last mod time is always pulled from local OS (but from meta file here)
			modTime = metaFileInfo.ModTime().Format(time.RFC3339)
		}

		info = translateHeaderToWebMeta(dir, "directory", modTime, metaHeader, nil)
	} else if os.IsNotExist(err) {
		// No dir metadata, assume remote default
		info.Path = dir
		info.OwnerName = "root"
		info.GroupName = "root"
		info.Permissions = "755"
		info.Type = "directory"
		info.Size = 0
		if localInfo != nil {
			// Last mod time is always pulled from local OS (dir itself)
			info.LastModified = localInfo.ModTime().Format(time.RFC3339)
		}

		// No concern for missing dir meta files
		err = nil
	} else {
		err = fmt.Errorf("failure reading directory metadata")
		return
	}

	return
}
