package api

import (
	"context"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/str"
	"scmp/web/internal"
	"slices"
)

func repoListAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	userPermissions := global.AssertFromContext[[]internal.UserPermission](clientCtx, "userPermissions", global.PermKey, "[]internal.UserPermission")

	var repoNames []string
	for _, userPerm := range userPermissions {
		repoNames = append(repoNames, userPerm.Repo)
	}

	resp = RepoList{
		Repositories: repoNames,
	}
	return
}

func hostListAPI(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	req := global.AssertType[HostListReq](fullReq.Params, "req", "HostListReq")

	cfg := global.AssertFromContext[config.Config](clientCtx, "config", global.ConfKey, "config.Config")

	var hostNameList []str.RepoRootDir
	var hostDetails map[str.RepoRootDir]HostSettings

	if req.WithDetails {
		hostDetails = make(map[str.RepoRootDir]HostSettings)
	}

	for hostName, hostInfo := range cfg.HostInfo {
		hostNameList = append(hostNameList, hostName)

		// Skip unnecessary processing if not requested
		if !req.WithDetails {
			continue
		}

		var collectedDetails HostSettings
		collectedDetails.DeploymentState = hostInfo.DeploymentState
		collectedDetails.IgnoreUniversal = hostInfo.IgnoreUniversal
		collectedDetails.RequiresVault = hostInfo.RequiresVault
		for group := range hostInfo.UniversalGroups {
			collectedDetails.UniversalGroups = append(collectedDetails.UniversalGroups, group)
		}
		collectedDetails.Proxy = hostInfo.Proxy
		collectedDetails.Endpoint = hostInfo.Endpoint
		collectedDetails.EndpointUser = hostInfo.EndpointUser
		collectedDetails.IdentityFile = hostInfo.IdentityFile
		collectedDetails.ConnectTimeout = hostInfo.ConnectTimeout
		hostDetails[hostName] = collectedDetails
	}

	slices.Sort(hostNameList)

	resp = HostList{
		Hosts:   hostNameList,
		Details: hostDetails,
	}
	return
}
