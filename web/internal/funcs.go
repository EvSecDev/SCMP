package internal

import "fmt"

func WOAPIDef(def map[string]Catalog) (err error) {
	apiDefMutex.Lock()
	defer apiDefMutex.Unlock()

	if apiDefSet {
		err = fmt.Errorf("global already set")
		return
	}

	apiDefOnce.Do(func() {
		apiDef = def
		apiDefSet = true
	})

	return
}
func GetAPIDef() map[string]Catalog {
	apiDefMutex.RLock()
	defer apiDefMutex.RUnlock()

	return apiDef
}

func WOAuthConfig(cfg AuthConfig) (err error) {
	authLock.Lock()
	defer authLock.Unlock()

	if authConfigSet {
		err = fmt.Errorf("global already set")
		return
	}

	authConfigOnce.Do(func() {
		authConfig = cfg
		authConfigSet = true
	})

	return
}
func GetAuthConfig() AuthConfig {
	authLock.RLock()
	defer authLock.RUnlock()

	return authConfig
}

func WORepoConfig(cfg map[string]RepoConfig) (err error) {
	repoLock.Lock()
	defer repoLock.Unlock()

	if repoConfigSet {
		err = fmt.Errorf("global already set")
		return
	}

	repoConfigOnce.Do(func() {
		repoConfig = cfg
		repoConfigSet = true
	})

	return
}
func GetRepoConfig() map[string]RepoConfig {
	repoLock.RLock()
	defer repoLock.RUnlock()

	return repoConfig
}

func GetPermissionsHierarchy() map[PermissionAction][]PermissionAction {
	return permissionHierarchy
}
