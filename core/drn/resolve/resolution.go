package resolve

import (
	"context"
	"fmt"
	"scmp/core/drn"
	"scmp/core/drn/drnconfig"
	glbConfig "scmp/internal/config"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"strings"
)

// Resolve all DRNs to values
func (replacer *Replacer) ResolveAll(ctx context.Context, hostInfo map[str.RepoRootDir]glbConfig.EndpointInfo) (err error) {
	err = replacer.initDRConfigs()
	if err != nil {
		err = fmt.Errorf("initial validation: %w", err)
		return
	}

	replacer.originMutex.RLock()
	defer replacer.originMutex.RUnlock()

	for originKey, drnConfigs := range replacer.originOfDRN {
		for _, drnConfig := range drnConfigs {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"Starting resolution for DRN %s\n", drnConfig.Original)
			_, err = replacer.resolve(ctx, originKey, hostInfo[originKey.globalID], drnConfig, nil)
			if err != nil {
				err = fmt.Errorf("%s: %w", drnConfig.Resolved, err)
				return
			}
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"Finished resolution for DRN %s: value %s\n", drnConfig.Original, drnConfig.Resolved)
		}
	}
	return
}

// Recursive expansion and resolution of DRNs
func (replacer *Replacer) resolve(ctx context.Context,
	origin originKey,
	hostInfo glbConfig.EndpointInfo,
	current *drn.DRC, parent *drn.DRC,
) (resolvedValue str.DRNVal, err error) {
	// limit recursion
	if current.Depth == drn.MaxNesting {
		err = fmt.Errorf("hit maximum recursion (nested DRN strings) at depth %d for DRN %s", drn.MaxNesting, current.Resolved)
		return
	}

	// attach lineage
	current.Parent = parent

	// Check and expand macros
	expandedDRC, err := ExpandMacros(*current, replacer.repoRootDir, hostInfo, origin.file)
	if err != nil {
		err = fmt.Errorf("macro in '%s': %w", current.Resolved, err)
		return
	}
	*current = expandedDRC

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
		"Expanded macros: DRN %s -> %s\n", current.Original, current.Expanded)

	// Prevent self references
	if current.HasCycle() {
		err = fmt.Errorf("cyclic DRN reference: %s", current.Expanded)
		return
	}

	// Cache check
	replacer.cacheMutex.RLock()
	existingValue, seen := replacer.cache[current.Expanded]
	replacer.cacheMutex.RUnlock()
	if seen {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"Expanded DRN %s: using cached resolved value '%s'\n",
			current.Expanded, existingValue)
		current.Resolved = existingValue
		resolvedValue = existingValue
		return
	}

	if current.IsInternalDRN() {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"Resolving internal DRN %s with context: host=%s file=%s\n",
			current.Expanded, hostInfo.EndpointName, origin.file)

		var macroValue str.DRNVal
		macroValue, err = resolveInternal(*current, replacer.repoRootDir, hostInfo, origin.file)
		if err != nil {
			err = fmt.Errorf("internal '%s': %w", current.Expanded, err)
			return
		}

		// Cache the value
		replacer.cacheMutex.Lock()
		replacer.cache[current.Expanded] = macroValue
		replacer.cacheMutex.Unlock()

		// Internal DRNs cannot be nested - return value here
		current.Resolved = macroValue
		resolvedValue = macroValue
		return
	}

	// External DRN resolution
	value, err := replacer.resolveExternal(ctx, *current)
	if err != nil {
		err = fmt.Errorf("external '%s': %w", current.Expanded, err)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
		"DRN %s: Direct external resolution returned value %s\n",
		current.Expanded, value)

	// Check if resolved value contains embedded DRNs and resolve them
	value, err = replacer.resolveEmbedded(ctx, origin, hostInfo, value, current, parent)
	if err != nil {
		return
	}

	// Cache concrete values for resolved DRN
	replacer.cacheMutex.Lock()
	replacer.cache[current.Expanded] = value
	replacer.cacheMutex.Unlock()

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
		"DRN %s: Embedded external resolution returned value %s\n",
		current.Expanded, value)

	// Got a real value, return back
	resolvedValue = value
	current.Resolved = value
	if parent != nil {
		parent.Resolved = value
	}
	return
}

// Recursively find and resolve all DRN substrings within a value string.
// Re-scans after each round of replacements to handle chains.
func (replacer *Replacer) resolveEmbedded(ctx context.Context,
	origin originKey,
	hostInfo glbConfig.EndpointInfo,
	value str.DRNVal,
	current *drn.DRC,
	parent *drn.DRC,
) (resolvedValue str.DRNVal, err error) {
	// First check: entire value is a single DRN (existing behavior)
	if drn.LikelyDRN(string(value)) {
		child, validationErr := drn.Validate(string(value))
		if validationErr == nil {
			logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
				"External DRN %s value %s looks like a DRN, resolving\n", current.Expanded, value)

			child.Depth += current.Depth + 1
			value, err = replacer.resolve(ctx, origin, hostInfo, &child, current)
			if err != nil {
				return
			}
			// After resolving, re-scan in case result still contains DRNs
			return replacer.resolveEmbedded(ctx, origin, hostInfo, value, current, parent)
		}
		// If validation failed, fall through to attempting DRN extraction
	}

	// Second check: value contains embedded DRN substrings
	embeddedDRNs := ExtractStringDRN(string(value))
	if len(embeddedDRNs) == 0 {
		resolvedValue = value
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
		"External DRN %s value %s: extracted individual DRNs %v\n", current.Expanded, value, embeddedDRNs)

	// Resolve each embedded DRN and replace in string
	for _, embeddedDRN := range embeddedDRNs {
		child, validationErr := drn.Validate(embeddedDRN)
		if validationErr != nil {
			err = fmt.Errorf("embedded DRN validation: %w", validationErr)
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
			"External DRN %s value %s: running resolution on DRN %s'\n", current.Expanded, value, embeddedDRN)

		child.Depth += current.Depth + 1
		resolvedValue, err = replacer.resolve(ctx, origin, hostInfo, &child, current)
		if err != nil {
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog,
			"External DRN %s value %s: Replacing '%s' with '%s'\n", current.Expanded, value, embeddedDRN, resolvedValue)

		value = str.DRNVal(strings.Replace(string(value), embeddedDRN, string(resolvedValue), 1))
	}

	// Re-scan in case resolved values introduced new DRNs
	return replacer.resolveEmbedded(ctx, origin, hostInfo, value, current, parent)
}

// Retrieves the config value for the given DRN
func (replacer *Replacer) resolveExternal(ctx context.Context, drnConfig drn.DRC) (value str.DRNVal, err error) {
	// Use cached config if available
	replacer.extConfigMutex.RLock()
	cfg, isCached := replacer.extConfigs[drnConfig.SerializeNamespace()]
	replacer.extConfigMutex.RUnlock()

	if !isCached {
		logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog,
			"External DRN %s has no cached config, loading file\n", drnConfig.Expanded)

		cfg, err = drnconfig.LoadConfig(replacer.repoRootDir, replacer.fileReader, drnConfig.Namespace)
		if err != nil {
			return
		}

		// Cache read-in config
		replacer.extConfigMutex.Lock()
		replacer.extConfigs[drnConfig.SerializeNamespace()] = cfg
		replacer.extConfigMutex.Unlock()
	} else {
		// Need a read lock for the rest of the function since we will be reading from the map at least once
		replacer.extConfigMutex.RLock()
		defer replacer.extConfigMutex.RUnlock()
	}

	value, err = cfg.LookupValue(drnConfig.Fields)
	return
}

// Retrieves the value for an internal DRN (superset of a macro - cannot recurse)
func resolveInternal(drnConfig drn.DRC, repositoryPath string, hostCfg glbConfig.EndpointInfo, filePath str.LocalRepoPath) (value str.DRNVal, err error) {
	if len(drnConfig.Namespace) == 0 {
		err = fmt.Errorf("invalid DRN state: missing namespace")
		return
	}

	if !drnConfig.IsInternalDRN() {
		err = fmt.Errorf("invalid DRN: internal DRN namespace must start with %s", drn.InternalNamespacePrefix)
		return
	}

	macroName, valid := drn.InternalDRNToMacroName(str.DRN(drnConfig.Expanded))
	if !valid {
		err = fmt.Errorf("invalid DRN: internal DRN not recognized: '%s'", drnConfig.Expanded)
		return
	}

	fileReplacer, hostReplacer, err := drn.NewMacroReplacer(repositoryPath, hostCfg, filePath)
	if err != nil {
		return
	}

	if drn.IsFileMacro(macroName) {
		value = str.DRNVal(fileReplacer.Replace(macroName))
	} else if drn.IsHostMacro(macroName) {
		value = str.DRNVal(hostReplacer.Replace(macroName))
	} else {
		err = fmt.Errorf("unknown macro category: neither host nor file")
		return
	}
	return
}
