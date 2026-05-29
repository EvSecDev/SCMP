package api

import (
	"context"
	"encoding/base64"
	"scmp/internal/global"
	"scmp/web/api/prompt"
	"scmp/web/internal"
)

func answerPendingPrompts(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[[]PromptAnswer](fullReq.Params, "req", "[]PromptAnswer")
	username := global.AssertFromContext[string](clientCtx, "username", global.UserKey, "string")

	for _, answerInfo := range req {
		// Decode answer
		userAnswer, err := base64.StdEncoding.DecodeString(answerInfo.Data)
		if err != nil {
			errObj.New(rpcInvalidParams, "Failed to decode answer data", err.Error())
			return
		}

		err = prompt.Answer(username, answerInfo.AssociatedDataID, answerInfo.PromptID, userAnswer)
		if err != nil {
			errObj.New(rpcInternalError, "Failed to answer prompt", err.Error())
			return
		}
	}

	resp = NilSuccess{}
	return
}
