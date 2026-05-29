package api

import (
	"context"
	"os"
	"scmp/internal/global"
	"scmp/web/internal"
)

func fsNewAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[FileOp](fullReq.Params, "req", "FileOp")

	cleanRequestedPath, err := validateRequestedFilePath(clientCtx, req.Path)
	if err != nil {
		errObj.New(rpcInvalidParams, "Invalid requested path", err.Error())
		return
	}

	_, err = os.Stat(cleanRequestedPath)
	if err == nil {
		// file exists
		errObj.New(rpcConflict, "Path already exists: "+cleanRequestedPath, "")
		return
	} else if os.IsNotExist(err) {
		// Files does not exist

		switch req.Type {
		case "file":
			file, err := os.Create(cleanRequestedPath)
			if err != nil {
				errObj.New(rpcInternalError, "Failed to create "+cleanRequestedPath, err.Error())
				return
			}
			defer func() {
				_ = file.Close()
			}()
		case "directory":
			if req.Recursive {
				err = os.MkdirAll(cleanRequestedPath, 0750)
			} else {
				err = os.Mkdir(cleanRequestedPath, 0750)
			}
			if err != nil {
				errObj.New(rpcInternalError, "Failed to create "+cleanRequestedPath, err.Error())
				return
			}
		default:
			errObj.New(rpcInvalidParams, "Unknown type "+req.Type, "")
			return
		}
		err = nil
	} else {
		errObj.New(rpcInternalError, "Failed checking existence for "+cleanRequestedPath, "")
		return
	}

	resp = NilSuccess{}
	return
}
