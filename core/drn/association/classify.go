package association

import (
	"fmt"
	"scmp/core/drn"
	"scmp/core/drn/resolve"
	"scmp/internal/str"
)

// Determines whether a file's DRN references match any of the target roots or their transitive dependents.
// It uses a two-phase approach: first check for concrete (non-macro) matches (these are fast and unambiguous) then fall through to macro expansion only if needed.
// Macro expansion is expensive (it must be tried per-host), so we avoid it when a concrete match already qualifies the file.
func (ref *ReferenceFinder) classifyFileReferences(
	content []byte,
	path str.LocalRepoPath,
	rootSet map[string]struct{},
	depSet map[string]bool,
	depToRoot map[string]string,
) (fileHosts map[str.RepoRootDir]struct{}, err error) {
	foundDRNs := resolve.ExtractRawDRNs(content)

	for _, foundDRN := range foundDRNs {
		_, ok := rootSet[foundDRN]
		if ok {
			fileHosts = ref.collectApplicableHosts(path)
			return
		}
		if depSet[foundDRN] {
			fileHosts = ref.collectApplicableHosts(path)
			return
		}
	}

	fileHosts = make(map[str.RepoRootDir]struct{})
	for _, foundDRN := range foundDRNs {
		if drn.ContainsMacro(str.DRN(foundDRN)) {
			var macroHosts map[str.RepoRootDir]struct{}
			macroHosts, err = ref.matchMacroDRN(foundDRN, path, rootSet, depToRoot)
			if err != nil {
				err = fmt.Errorf("macro match for '%s': %w", foundDRN, err)
				return
			}
			for macroHost := range macroHosts {
				fileHosts[macroHost] = struct{}{}
			}
		}
	}
	if len(fileHosts) == 0 {
		return
	}
	return
}

// Expands a macro-containing DRN once per host and checks if the expanded form matches a root or its transitive dependent.
// Early-continue on fileAppliesToHost filters hosts the file doesn't belong to.
func (ref *ReferenceFinder) matchMacroDRN(
	foundDRN string,
	path str.LocalRepoPath,
	rootSet map[string]struct{},
	depToRoot map[string]string,
) (matchedHosts map[str.RepoRootDir]struct{}, err error) {
	drc, err := drn.Validate(foundDRN)
	if err != nil {
		err = fmt.Errorf("validate '%s': %w", foundDRN, err)
		return
	}

	matchedHosts = make(map[str.RepoRootDir]struct{})
	for hostAlias, hostInfo := range ref.hostInfo {
		if !fileAppliesToHost(path, hostAlias, hostInfo, ref.primaryUniversalDirectory) {
			continue
		}
		var expanded drn.DRC
		expanded, err = resolve.ExpandMacros(drc, ref.repositoryPath, hostInfo, path)
		if err != nil {
			err = fmt.Errorf("expand macros for host '%s': %w", hostAlias, err)
			return
		}
		_, ok := rootSet[string(expanded.Expanded)]
		if ok {
			matchedHosts[hostAlias] = struct{}{}
		} else if depToRoot[string(expanded.Expanded)] != "" {
			matchedHosts[hostAlias] = struct{}{}
		}
	}
	if len(matchedHosts) == 0 {
		return
	}
	return
}
