package api

import (
	"context"
	"fmt"
	"scmp/internal/global"
	"scmp/web/internal"
)

func checkUserPermissions(clientCtx context.Context, api internal.Catalog) (authorized bool, err error) {
	// Return immediately if endpoint is set to noauth (i.e login api)
	if hasPermission(api.AllowedPermissions, []internal.PermissionAction{internal.NoAuthAction}) {
		authorized = true
		return
	}

	username := global.AssertFromContext[string](clientCtx, "username", global.UserKey, "string")
	userPermissions := global.AssertFromContext[[]internal.UserPermission](clientCtx, "userPermissions", global.PermKey, "[]internal.UserPermission")

	userImpliedPerms := expandPermissions(userPermissions)

	if !hasPermission(userImpliedPerms, api.AllowedPermissions) {
		err = fmt.Errorf("user '%s' lacks permission for %s (requires: %v, has: %v)",
			username, api.Method, api.AllowedPermissions, userImpliedPerms)
		return
	}

	authorized = true
	return
}

func expandPermissions(userPermissions []internal.UserPermission) (impliedPerms []internal.PermissionAction) {
	seen := make(map[internal.PermissionAction]bool)

	var mapUserPerms func(internal.PermissionAction)
	mapUserPerms = func(perm internal.PermissionAction) {
		if seen[perm] {
			return
		}
		seen[perm] = true

		// Recursively include inherited permissions
		for _, inherited := range internal.GetPermissionsHierarchy()[perm] {
			mapUserPerms(inherited)
		}
	}

	// Collect permission strings from user config
	for _, explicitPerms := range userPermissions {
		for _, act := range explicitPerms.Actions {
			mapUserPerms(act)
		}
	}

	// Convert back to slice
	for impliedPerm := range seen {
		impliedPerms = append(impliedPerms, impliedPerm)
	}

	return
}

func hasPermission(userPerms, requiredPerms []internal.PermissionAction) (authorized bool) {
	permSet := make(map[internal.PermissionAction]struct{}, len(userPerms))
	for _, userPerm := range userPerms {
		permSet[userPerm] = struct{}{}
	}
	for _, requiredPerm := range requiredPerms {
		_, ok := permSet[requiredPerm]
		if ok {
			authorized = true
			return
		}
	}
	return
}
