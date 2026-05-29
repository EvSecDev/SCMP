package internal

import "sync"

var (
	// Store API descriptions, input, output
	apiDef      map[string]Catalog
	apiDefSet   bool
	apiDefOnce  sync.Once
	apiDefMutex sync.RWMutex

	// Stores user configuration and authentication globals
	authConfig     AuthConfig
	authConfigSet  bool
	authConfigOnce sync.Once
	authLock       sync.RWMutex

	// Stores repo configurations
	repoConfig     map[string]RepoConfig
	repoConfigSet  bool
	repoConfigOnce sync.Once
	repoLock       sync.RWMutex

	// Permission Inheritance based on singular permission action (does not include self permission)
	permissionHierarchy = map[PermissionAction][]PermissionAction{
		AdminAction:  {ReadAction, WriteAction, SeedAction, DeployAction, NoAuthAction},
		DeployAction: {ReadAction, NoAuthAction},
		WriteAction:  {ReadAction, NoAuthAction},
		SeedAction:   {ReadAction, NoAuthAction},
		ReadAction:   {NoAuthAction},
		NoAuthAction: {},
	}
)
