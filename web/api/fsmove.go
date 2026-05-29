package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"scmp/internal/global"
	"scmp/web/internal"
)

func fsMoveAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[FileMove](fullReq.Params, "req", "FileMove")

	cleanReqSrcPath, err := validateRequestedFilePath(clientCtx, req.SourcePath)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested source path", err.Error())
		return
	}
	cleanReqDstPath, err := validateRequestedFilePath(clientCtx, req.DestinationPath)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested destination path", err.Error())
		return
	}

	srcInfo, err := os.Stat(cleanReqSrcPath)
	if err != nil {
		errObj.New(rpcInternalError, "Error checking source path info", err.Error())
		return
	}

	if srcInfo == nil {
		errObj.New(rpcInternalError, "Failed to retrieve source path info", "")
		return
	}

	// Bail when file is present but user did not say to overwrite it
	dstInfo, err := os.Stat(cleanReqDstPath)
	if err == nil && !req.OverwriteDestination {
		errObj.New(rpcInvalidState, "Destination exists, but overwrite destination was not requested", "")
		return
	} else if err != nil {
		// Stat destination had an error other than it not existing
		if !os.IsNotExist(err) {
			errObj.New(rpcInternalError, "Error checking destination path info", err.Error())
			return
		}
	}

	// Disallow moving between different types
	if dstInfo != nil {
		if (srcInfo.IsDir() && !srcInfo.IsDir()) || (!srcInfo.IsDir() && srcInfo.IsDir()) {
			errObj.New(rpcInvalidParams, "Invalid request", "cannot move items between types (file/directory)")
			return
		}
	}

	if srcInfo.IsDir() {
		err = os.Mkdir(cleanReqDstPath, 0750)
		if err != nil {
			if !os.IsExist(err) {
				errObj.New(rpcInternalError, "Error creating destination directory "+cleanReqDstPath, err.Error())
				return
			}
		}
	} else if !srcInfo.IsDir() {
		// Retrieve source file content
		srcFile, err := os.Open(cleanReqSrcPath)
		if err != nil {
			errObj.New(rpcInternalError, "Error opening source file "+cleanReqSrcPath, err.Error())
			return
		}
		defer func() {
			_ = srcFile.Close()
		}()

		// Blindly create/truncate destination path (guarded by file existing/user not wanting overwrite previously)
		dstFile, err := os.Create(cleanReqDstPath)
		if err != nil {
			errObj.New(rpcInternalError, "Error creating destination file "+cleanReqDstPath, err.Error())
			return
		}
		defer func() {
			_ = dstFile.Close()
		}()

		// Stream source data to destination
		_, err = io.Copy(dstFile, srcFile)
		if err != nil {
			errObj.New(rpcInternalError, fmt.Sprintf("Error streaming data from %s to %s", cleanReqSrcPath, cleanReqDstPath), err.Error())
			return
		}
	}

	// Only remove source after destination has been created
	if req.DeleteSource {
		err = os.Remove(cleanReqSrcPath)
		if err != nil {
			errObj.New(rpcInternalError, "Error removing source file "+cleanReqSrcPath, err.Error())
			return
		}
	}

	resp = NilSuccess{}
	return
}
