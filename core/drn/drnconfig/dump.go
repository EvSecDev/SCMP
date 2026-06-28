package drnconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"scmp/core/drn"
	"scmp/internal/fsops"
	"scmp/internal/str"
	"slices"
	"strings"
)

// Dumps all known internal and external DRNs in readable table format (does not resolve)
func ShowAll(repositoryRoot string) (table string, err error) {
	var tableBuilder strings.Builder

	tableBuilder.WriteString("Internal DRNs:\n")

	// Run through internal DRNs
	maxSeenLen := 0
	var intDRNS []str.DRN
	for _, drnString := range drn.GetAllInternalDRNs() {
		intDRNS = append(intDRNS, drnString)
		if len(drnString) > maxSeenLen {
			maxSeenLen = len(drnString)
		}
	}

	// Source is map, sort slice for stable output
	slices.Sort(intDRNS)

	// Add space after longest DRN string
	maxSeenLen += 4

	// Write nicely indented line with macro for each internal DRN
	for _, intDRN := range intDRNS {
		tableBuilder.WriteString("  ")
		tableBuilder.WriteString(string(intDRN))
		tableBuilder.WriteString(strings.Repeat(" ", maxSeenLen-len(intDRN)))
		tableBuilder.WriteString("Macro Name - ")
		macroName, _ := drn.InternalDRNToMacroName(intDRN)
		tableBuilder.WriteString(macroName)
		tableBuilder.WriteString("\n")
	}

	// Walk external config directory to compile complete list of nodes
	collectedDRNs, err := GetAllDRNs(repositoryRoot, nil, nil)
	if err != nil {
		return
	}

	tableBuilder.WriteString("\nExternal DRNs (located in ")
	tableBuilder.WriteString(drn.ExternalVariableDirectory)
	tableBuilder.WriteString("):\n")

	// Write nicely indented line with macro for each external DRN
	maxSeenLen = 0
	for _, drns := range collectedDRNs {
		for drn := range drns {
			if len(drn) > maxSeenLen {
				maxSeenLen = len(drn)
			}
		}
	}

	// Add space after longest DRN string
	maxSeenLen += 4

	for _, drns := range collectedDRNs {
		for drn := range drns {
			tableBuilder.WriteString("  ")
			tableBuilder.WriteString(string(drn))
			tableBuilder.WriteString(strings.Repeat(" ", maxSeenLen-len(drn)))
			tableBuilder.WriteString("\n")
		}
	}

	table = tableBuilder.String()
	return
}

// Recursively walks external DRN configuration directory and gathers every DRN and its direct value
func GetAllDRNs(repositoryRoot string, pathWalker fsops.PathWalker, reader fsops.FileReader) (collectedDRNs map[str.LocalRepoPath]map[str.DRN]str.DRNVal, err error) {
	repoConfigDir := filepath.Join(repositoryRoot, drn.ExternalVariableDirectory)

	if pathWalker == nil {
		pathWalker = fsops.NewFileSystemWalker(repoConfigDir)
	}
	if reader == nil {
		reader = fsops.NewFileSystemReader(repoConfigDir)
	}

	paths, err := pathWalker()
	if err != nil {
		err = fmt.Errorf("failed walking configuration directory: %w", err)
		return
	}

	collectedDRNs = make(map[str.LocalRepoPath]map[str.DRN]str.DRNVal)
	for _, path := range paths {
		var cfg []byte
		cfg, err = reader(path)
		if err != nil {
			return
		}

		repoPath := str.LocalRepoPath(filepath.Join(drn.ExternalVariableDirectory, string(path)))

		var node CfgNode
		err = json.Unmarshal(cfg, &node)
		if err != nil {
			err = fmt.Errorf("parse '%s': %w", path, err)
			return
		}

		var foundDRNs map[str.DRN]str.DRNVal
		foundDRNs, err = node.FormatAll(string(repoPath))
		if err != nil {
			err = fmt.Errorf("config '%s': %w", path, err)
			return
		}

		collectedDRNs[repoPath] = foundDRNs
	}
	return
}
