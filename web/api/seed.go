package api

import (
	"context"
	"scmp/web/internal"
)

func seedAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	//req := utils.AssertType[SeedRequest](fullReq.Params, "req", "SeedRequest")

	return
}

func seedSelectionsAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	return
}
