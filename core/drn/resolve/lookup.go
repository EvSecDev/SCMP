package resolve

import (
	"context"
	"fmt"
	"scmp/core/drn"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/str"
)

// Single lookup function with optional contextual host/file information.
// Not performant.
func LookupValue(ctx context.Context, requestedDRN string, ctxPath str.LocalRepoPath, ctxHost str.RepoRootDir) (value str.DRNVal, err error) {
	drnConfig, err := drn.Validate(requestedDRN)
	if err != nil {
		err = fmt.Errorf("validate: %w", err)
		return
	}

	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	// Ensure context is present, only in scenarios that need them.
	// If a nested DRN has values that need context, let them bubble up errors at that point.
	macros := drn.ExtractMacros(requestedDRN)
	for _, macro := range macros {
		if drn.IsFileMacro(macro) && ctxPath == "" {
			err = fmt.Errorf("provided DRN contains a file-based macro but no context file path was supplied")
			return
		}
		if drn.IsHostMacro(macro) && ctxHost == "" {
			err = fmt.Errorf("provided DRN contains a host-based macro but no context host alias was supplied")
			return
		}
	}
	if drnConfig.IsInternalDRN() {
		if drn.IsFileDRN(requestedDRN) && ctxPath == "" {
			err = fmt.Errorf("provided internal DRN requires context file path")
			return
		}
		if drn.IsHostDRN(requestedDRN) && ctxHost == "" {
			err = fmt.Errorf("provided internal DRN requires context host alias")
			return
		}
	}

	var info config.EndpointInfo
	if ctxHost != "" {
		// Lookup with file/host to expand macros
		var validHost bool
		info, validHost = cfg.HostInfo[str.RepoRootDir(ctxHost)]
		if !validHost {
			err = fmt.Errorf("unknown host alias '%s'", ctxHost)
			return
		}
	}

	key := originKey{
		globalID: ctxHost,
		file:     ctxPath,
	}

	// resolve requires replacer for cache even though we don't use the cache for single lookups here
	replacer := NewReplacer(cfg.RepositoryPath, nil)
	value, err = replacer.resolve(ctx, key, info, &drnConfig, nil)
	if err != nil {
		err = fmt.Errorf("resolve: %w", err)
		return
	}
	return
}
