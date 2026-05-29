package api

import (
	"context"
	"os"
	"scmp/internal/global"
	"scmp/web/internal"
)

func fsDeleteAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[FileOp](fullReq.Params, "req", "FileOp")

	cleanRequestedPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	_, err = os.Stat(cleanRequestedPath)
	if err == nil {
		// file exists
		switch req.Recursive {
		case false:
			err := os.Remove(cleanRequestedPath)
			if err != nil {
				errObj.New(rpcInternalError, "Internal Error", err.Error())
				return
			}
		case true:
			err := os.RemoveAll(cleanRequestedPath)
			if err != nil {
				errObj.New(rpcInternalError, "Failed to delete "+cleanRequestedPath, err.Error())
				return
			}
		}
	} else if os.IsNotExist(err) {
		// Files does not exist
		// treat success
		err = nil
	} else {
		errObj.New(rpcInternalError, "Failed checking existence for "+cleanRequestedPath, "")
		return
	}

	resp = NilSuccess{}
	return
}
