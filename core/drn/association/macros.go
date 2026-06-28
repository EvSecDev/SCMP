package association

import (
	"fmt"
	"scmp/core/drn"
	"slices"
	"strings"
)

// Populates the reverse macro map from host-specific macros.
// Each host defines these differently, so we need a replacer per endpoint.
func (ref *ReferenceFinder) collectHostMacroValues() (err error) {
	for _, hostInfo := range ref.hostInfo {
		err = ref.collectMacroValues(
			func() (*strings.Replacer, error) { return drn.NewHostMacroReplacer(hostInfo) },
			drn.GetAllHostMacros(),
			fmt.Sprintf("host replacer '%s'", hostInfo.EndpointName),
		)
		if err != nil {
			return
		}
	}
	return
}

// Populates the reverse macro map from file-scoped macros.
// These depend on the file path, so we need one replacer per file. This is the more expensive half of ensureMacroValues.
func (ref *ReferenceFinder) collectFileMacroValues() (err error) {
	allFiles, err := ref.getAllFiles()
	if err != nil {
		return
	}
	for _, file := range allFiles {
		err = ref.collectMacroValues(
			func() (*strings.Replacer, error) { return drn.NewFileMacroReplacer(ref.repositoryPath, file) },
			drn.GetAllFileMacros(),
			fmt.Sprintf("file replacer for '%s'", file),
		)
		if err != nil {
			return
		}
	}
	return
}

// The shared worker for building the value→macro reverse map.
// It applies each macro name through a replacer and records the result.
// Multiple macros can produce the same value, hence the []string slice.
func (ref *ReferenceFinder) collectMacroValues(
	newReplacer func() (*strings.Replacer, error),
	macros []string,
	errPrefix string,
) (err error) {
	replacer, err := newReplacer()
	if err != nil {
		err = fmt.Errorf("%s: %w", errPrefix, err)
		return
	}
	for _, macro := range macros {
		value := replacer.Replace(macro)
		if value != "" && !slices.Contains(ref.macroValues[value], macro) {
			ref.macroValues[value] = append(ref.macroValues[value], macro)
		}
	}
	return
}

// Builds, once, a map from each macro value to the set of macro names that produce it.
// This is the reverse map used by reverseExpandToMacroVariants to "un-expand" concrete values back into macro forms.
// Host macros and file macros are collected separately because they come from different sources.
func (ref *ReferenceFinder) ensureMacroValues() (err error) {
	if ref.macroValuesOK {
		return
	}
	ref.macroValues = make(map[string][]string)

	err = ref.collectHostMacroValues()
	if err != nil {
		return
	}
	err = ref.collectFileMacroValues()
	if err != nil {
		return
	}

	ref.sortedMacroValues = sortedByLengthDesc(ref.macroValues)
	ref.macroValuesOK = true
	return
}

// Sorts macro values longest-first so that when we reverse-expand, a longer value like "myhost1" is tried before "myhost".
// Otherwise we could potentially greedily match a short version producing incorrect variants
func sortedByLengthDesc(input map[string][]string) (keys []string) {
	keys = make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		return len(b) - len(a)
	})
	return
}
