package api

import (
	"context"
	"os"
	"scmp/core/filesystem/content"
	"scmp/core/filesystem/metadata"
	"scmp/internal/global"
	"scmp/internal/str"
	"scmp/web/datastore"
	"scmp/web/internal"
	"strings"

	"github.com/google/uuid"
)

func contentReadAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[PathRequest](fullReq.Params, "req", "PathRequest")

	cleanRequestPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	fileInfo, err := os.Stat(cleanRequestPath)
	if err != nil {
		errObj.New(rpcInternalError, "Unable to read filesystem metadata for "+cleanRequestPath, err.Error())
		return
	}
	if fileInfo.IsDir() {
		errObj.New(rpcInvalidParams, "Requested file is a directory", "")
		return
	}

	reqFile, err := os.ReadFile(cleanRequestPath)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to read content for "+cleanRequestPath, err.Error())
		return
	}

	_, data, err := metadata.Extract(string(reqFile))
	if err != nil {
		// Only error when the file had a header
		if err.Error() == "json start delimiter missing" {
			// Use raw file content as data
			data = reqFile
			err = nil
		} else {
			errObj.New(rpcInternalError, "Failed parsing content for "+cleanRequestPath, err.Error())
			return
		}
	}

	userID := global.AssertFromContext[string](clientCtx, "userID", global.UserKey, "string")
	newDataID := uuid.New()

	datastore.Put(userID, newDataID.String(), data)

	resp = DownloadLink{
		Location: internal.DownloadBasePath + newDataID.String(),
	}
	return
}

func contentEditAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[ProcessUploadReq](fullReq.Params, "req", "ProcessUploadReq")

	cleanRequestPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	_, err = uuid.Parse(req.DataID)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid data ID", err.Error())
		return
	}

	userID := global.AssertFromContext[string](clientCtx, "userID", global.UserKey, "string")

	raw, err := datastore.Get(userID, req.DataID)
	if err != nil {
		errObj.New(rpcInvalidParams, "Unable to retrieve data", err.Error())
		return
	}
	newData, ok := raw.([]byte)
	if !ok {
		errObj.New(rpcInternalError, "Failed type assertion datastore bytes", "")
		return
	}

	datastore.Delete(userID, req.DataID) // clean immediately - requires user to re-upload on any subsequent error

	// Always ensure there is a trailing newline
	if !strings.HasSuffix(string(newData), "\n") {
		newData = append(newData, '\n')
	}

	fileInfo, err := os.Stat(cleanRequestPath)
	if err != nil {
		errObj.New(rpcInternalError, "Unable to read filesystem metadata for "+cleanRequestPath, err.Error())
		return
	}
	if fileInfo.IsDir() {
		errObj.New(rpcInvalidParams, "Requested file is a directory", "")
		return
	}

	reqFile, err := os.ReadFile(cleanRequestPath)
	if err != nil {
		errObj.New(rpcInternalError, "Failed to read content for "+cleanRequestPath, err.Error())
		return
	}

	var noHeaderInfile bool
	existingHeader, _, err := metadata.Extract(string(reqFile))
	if err != nil {
		// Only error when the file had a header
		if err.Error() == "json start delimiter missing" {
			noHeaderInfile = true
			err = nil
		} else {
			errObj.New(rpcInternalError, "Failed parsing content for "+cleanRequestPath, err.Error())
			return
		}
	}

	if noHeaderInfile {
		err = os.WriteFile(cleanRequestPath, newData, 0640)
	} else {
		err = content.WriteRepoFile(clientCtx, str.LocalRepoPath(cleanRequestPath), existingHeader, &newData)
	}
	if err != nil {
		errObj.New(rpcInternalError, "Failed writing content for "+cleanRequestPath, err.Error())
		return
	}

	resp = NilSuccess{}
	return
}
