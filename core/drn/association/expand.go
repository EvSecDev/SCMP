package association

import (
	"fmt"
	"scmp/core/drn"
	"scmp/core/drn/resolve"
	"scmp/internal/str"
	"slices"
	"strings"
)

// Applies a single macro replacement at one position in a DRC, producing one candidate variant.
// We iterate over macro values sorted longest-first (sortedByLengthDesc) so that a value like "v2" doesn't prematurely match when "v2.0" is the actual macro target.
// The clone-then-mutate pattern on namespace/fields avoids corrupting the original DRC for subsequent replacement attempts.
func (ref *ReferenceFinder) expandOneMacroVariant(curDRC drn.DRC, sortedMacroValues []string) (variants []string, err error) {
	for nsOrFields, segment := range [][]string{curDRC.Namespace, curDRC.Fields} {
		for segmentIndex, text := range segment {
			for _, value := range sortedMacroValues {
				if !strings.Contains(text, value) {
					continue
				}

				for _, macroName := range ref.macroValues[value] {
					replaced := strings.Replace(text, value, macroName, 1)
					if replaced == text {
						continue
					}

					namespace := slices.Clone(curDRC.Namespace)
					fields := slices.Clone(curDRC.Fields)
					if nsOrFields == 0 {
						namespace[segmentIndex] = replaced
					} else {
						fields[segmentIndex] = replaced
					}

					vdrc := drn.DRC{Namespace: namespace, Fields: fields}
					err = vdrc.SerializeExpanded()
					if err != nil {
						err = fmt.Errorf("serialize '%v%s%v': %w", namespace, drn.PrimarySeparator, fields, err)
						return
					}

					variants = append(variants, string(vdrc.Expanded))
				}
			}
		}
	}
	return
}

// Resolves a DRN value (which may contain macros) into its concrete form(s).
// A DRN without macros produces a single concrete value.
// A DRN with only host macros needs no file context, so we use a single dummy path to drive the expansion.
// A DRN with file macros must be expanded once per file.
func (ref *ReferenceFinder) expandValue(drnValue string, allFiles []str.LocalRepoPath) (concretes []string, err error) {
	foundDRNs := resolve.ExtractStringDRN(drnValue)
	if len(foundDRNs) == 0 {
		// No drn embedded in the value
		return
	}

	for _, foundDRN := range foundDRNs {
		var drc drn.DRC
		drc, err = drn.Validate(foundDRN)
		if err != nil {
			err = fmt.Errorf("validate '%s': %w", foundDRN, err)
			return
		}
		if !drn.ContainsMacro(str.DRN(foundDRN)) {
			concretes = append(concretes, string(drc.Original))
			return
		}

		seen := make(map[string]struct{})

		files := allFiles
		if !drn.HasFileMacros(drc.Namespace) && !drn.HasFileMacros(drc.Fields) {
			// Host-only macros: ExpandMacros skips file replacer, so empty path is safe
			files = []str.LocalRepoPath{""}
		}

		for _, filePath := range files {
			for _, hostInfo := range ref.hostInfo {
				var expanded drn.DRC
				expanded, err = resolve.ExpandMacros(drc, ref.repositoryPath, hostInfo, filePath)
				if err != nil {
					err = fmt.Errorf("expand '%s': %w", drc.Original, err)
					return
				}
				key := string(expanded.Expanded)
				_, ok := seen[key]
				if !ok {
					seen[key] = struct{}{}
					concretes = append(concretes, key)
				}
			}
		}
	}
	return
}
