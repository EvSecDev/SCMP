package resolve

import (
	"fmt"
	"scmp/core/drn"
	"scmp/internal/config"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"strings"
)

// Returns fully macro-expanded and macro-validated DRN config using host and file context
func ExpandMacros(drnConfig drn.DRC, repositoryPath string, hostCfg config.EndpointInfo, filePath str.LocalRepoPath) (expandedDRC drn.DRC, err error) {
	hasFile := drn.HasFileMacros(drnConfig.Namespace) || drn.HasFileMacros(drnConfig.Fields)
	hasHost := drn.HasHostMacros(drnConfig.Namespace) || drn.HasHostMacros(drnConfig.Fields)

	var fileMacroReplacer, hostMacroReplacer *strings.Replacer
	if hasFile {
		fileMacroReplacer, err = drn.NewFileMacroReplacer(repositoryPath, filePath)
		if err != nil {
			return
		}
	}
	if hasHost {
		hostMacroReplacer, err = drn.NewHostMacroReplacer(hostCfg)
		if err != nil {
			return
		}
	}

	if fileMacroReplacer != nil {
		drnConfig.Namespace = parsing.BulkSliceReplacer(drnConfig.Namespace, fileMacroReplacer)
		drnConfig.Fields = parsing.BulkSliceReplacer(drnConfig.Fields, fileMacroReplacer)
	}
	if hostMacroReplacer != nil {
		drnConfig.Namespace = parsing.BulkSliceReplacer(drnConfig.Namespace, hostMacroReplacer)
		drnConfig.Fields = parsing.BulkSliceReplacer(drnConfig.Fields, hostMacroReplacer)
	}

	// Reject macros that are not known
	err = validateNoUnknownMacros(drnConfig.Namespace)
	if err != nil {
		err = fmt.Errorf("namespace component %w", err)
		return
	}
	err = validateNoUnknownMacros(drnConfig.Fields)
	if err != nil {
		err = fmt.Errorf("field %w", err)
		return
	}

	// Write expanded string to drc
	err = drnConfig.SerializeExpanded()
	if err != nil {
		err = fmt.Errorf("expanded DRN validate: %w", err)
		return
	}

	expandedDRC = drnConfig
	return
}

// Reports any unknown macros and their position in component slice
func validateNoUnknownMacros(components []string) (err error) {
	for index, value := range components {
		if strings.Contains(value, drn.InternalMacroOpen) || strings.Contains(value, drn.InternalMacroClose) {
			// Get unk macro name for better error
			unkMacros := drn.ExtractMacros(value)
			err = fmt.Errorf("%d contains unknown macro(s): %v", index+1, unkMacros)
			return
		}
	}
	return
}
