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
	"strings"
	"time"
)

func fsPathSearch(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[FilePathSearchReq](fullReq.Params, "req", "FilePathSearchReq")

	cleanRequestPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	// Search based on text match type
	fileNameResults, err := searchfilesystem(cleanRequestPath, req.Query, req.QueryType, req.FileType, req.Depth)
	if err != nil {
		errObj.New(rpcInternalError, "Search error", err.Error())
		return
	}

	var searchResults FilePathSearchResults

	// Get metadata for each result
	for _, fileName := range fileNameResults {
		fileMeta, err := getFileMetadata(fileName)
		if err != nil {
			errObj.New(rpcInternalError, "Error retrieving metadata for "+fileName, err.Error())
			return
		}
		searchResults.Matches = append(searchResults.Matches, fileMeta)
	}

	searchResults.MatchCount = len(searchResults.Matches)

	// Send back request parameters with results
	searchResults.OriginalReq = req
	searchResults.OriginalReq.Path = cleanRequestPath

	resp = searchResults
	return
}

func searchfilesystem(searchBaseDir string, searchText string, searchType string, searchFileType string, searchDepth int) (results []string, err error) {
	baseDepth := strings.Count(filepath.Clean(searchBaseDir), string(filepath.Separator))

	err = filepath.WalkDir(searchBaseDir, func(path string, dirEntry fs.DirEntry, err error) (retErr error) {
		if err != nil {
			return nil // skip errors
		}

		currentDepth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - baseDepth

		// Keep total depth within bounds
		if currentDepth > global.MaxDirectoryLoopCount {
			return
		}

		// 0 unlimited depth
		// 1 current dir
		if searchDepth > 0 && currentDepth >= searchDepth {
			if dirEntry.IsDir() {
				retErr = filepath.SkipDir
				return
			}
			return
		}

		// File type check
		switch searchFileType {
		case "file":
			if dirEntry.IsDir() {
				return
			}
		case "directory":
			if !dirEntry.IsDir() {
				return
			}
		}

		name := dirEntry.Name()
		match := false
		switch searchType {
		case "exact":
			match = name == searchText
		case "contains":
			match = strings.Contains(name, searchText)
		case "prefix":
			match = strings.HasPrefix(name, searchText)
		case "suffix":
			match = strings.HasSuffix(name, searchText)
		}

		if match {
			results = append(results, path)
		}
		return
	})

	return
}

func getFileMetadata(file string) (fileMeta FileMetadata, err error) {
	// Stat the file once
	fileInfo, err := os.Stat(file)
	if err != nil {
		err = fmt.Errorf("failed stating file '%s': %w", file, err)
		return
	}

	fileType := "file"
	metadataFilePath := file

	// Use directory metadata file if it's a directory
	if fileInfo.IsDir() {
		fileType = "directory"
		metadataFilePath = filepath.Join(file, string(filesystem.DirMetaFileName))
	}

	// Check if metadata file exists; fallback to original file if not
	_, err = os.Stat(metadataFilePath)
	if err != nil {
		if os.IsNotExist(err) && fileType == "directory" {
			metadataFilePath = file
			err = nil
		} else {
			err = fmt.Errorf("failed stating metadata file '%s': %w", metadataFilePath, err)
			return
		}
	}

	var data []byte
	var metaHeader filesystem.MetaHeader
	if fileType != "directory" {
		// Read the file
		var fileData []byte
		fileData, err = os.ReadFile(metadataFilePath)
		if err != nil {
			err = fmt.Errorf("unable to read '%s': %w", metadataFilePath, err)
			return
		}

		// Extract metadata if present
		metaHeader, data, err = metadata.Extract(string(fileData))
		if err != nil && err.Error() == "json start delimiter missing" {
			data = fileData
			err = nil
		} else if err != nil {
			err = fmt.Errorf("unable to parse '%s': %w", metadataFilePath, err)
			return
		}
	}

	modTime := fileInfo.ModTime().Format(time.RFC3339)
	fileMeta = translateHeaderToWebMeta(file, fileType, modTime, metaHeader, &data)
	return
}
