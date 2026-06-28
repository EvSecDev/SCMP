package association

import (
	"fmt"
	"os"
	"scmp/core/deployment"
	"scmp/core/drn"
	"scmp/internal/str"
	"strings"
)

// Constructs the byte sequences we search file contents for.
// It includes both the concrete DRNs and their macro variants, because a file might reference a DRN using any of its valid macro forms.
// The search is a superset, false positives are filtered out later by classifyFileReferences.
func (ref *ReferenceFinder) buildSearchTerms(roots []str.DRN) (searchTerms [][]byte, err error) {
	err = ref.ensureReverseDeps()
	if err != nil {
		err = fmt.Errorf("reverse dependents: %w", err)
		return
	}
	err = ref.ensureMacroValues()
	if err != nil {
		err = fmt.Errorf("macro collection: %w", err)
		return
	}
	dependencies := ref.traverseReverseDeps(roots)
	for dependency := range dependencies {
		searchTerms = append(searchTerms, []byte(dependency))

		var variants []string
		variants, err = ref.reverseExpandToMacroVariants(dependency)
		if err != nil {
			return
		}
		for _, mv := range variants {
			searchTerms = append(searchTerms, []byte(mv))
		}
	}
	return
}

// Finds every macro-variant form of a concrete DRN.
// This is the inverse of normal macro expansion: we start from the concrete form and un-expand known macro values back into macro names.
// These variants are added to search terms because a file might use macros where we only know the concrete value at query time.
func (ref *ReferenceFinder) reverseExpandToMacroVariants(concreteDRN string) (variants []string, err error) {
	sortedMacroValues := ref.sortedMacroValues
	seen := map[string]struct{}{concreteDRN: {}}
	queue := []string{concreteDRN}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		var curDRC drn.DRC
		curDRC, err = drn.Validate(current)
		if err != nil {
			err = fmt.Errorf("validate '%s': %w", current, err)
			return
		}

		var expanded []string
		expanded, err = ref.expandOneMacroVariant(curDRC, sortedMacroValues)
		if err != nil {
			err = fmt.Errorf("macro variant: %w", err)
			return
		}
		for _, expandedVal := range expanded {
			_, ok := seen[expandedVal]
			if !ok {
				seen[expandedVal] = struct{}{}
				variants = append(variants, expandedVal)
				queue = append(queue, expandedVal)
			}
		}
	}
	return
}

// Builds the reverse-dependency index once and caches it.
// The index maps every concrete DRN value (the result of macro expansion) to the set of DRN keys that reference it.
// This lets us answer "what DRNs transitively depend on this one?" by walking the index BFS-style.
// Building it requires expanding all macro-valued DRNs, which is why its lazy.
func (ref *ReferenceFinder) ensureReverseDeps() (err error) {
	if ref.reverseDepsOK {
		return
	}
	ref.reverseDeps = make(map[string][]str.DRN)

	allFiles, err := ref.getAllFiles()
	if err != nil {
		return
	}

	for _, drnMap := range ref.allDRNs {
		for drnStr, drnVal := range drnMap {
			var concretes []string
			concretes, err = ref.expandValue(string(drnVal), allFiles)
			if err != nil {
				err = fmt.Errorf("expand '%s': %w", drnVal, err)
				return
			}

			for _, expandedDRN := range concretes {
				ref.reverseDeps[expandedDRN] = append(ref.reverseDeps[expandedDRN], str.DRN(drnStr))
			}
		}
	}
	ref.reverseDepsOK = true
	return
}

// Returns every file in the repository, cached after the first walk.
func (ref *ReferenceFinder) getAllFiles() (files []str.LocalRepoPath, err error) {
	if ref.allFilesOK {
		files = ref.allFiles
		return
	}
	gotFiles, err := ref.pathWalker()
	if err != nil {
		err = fmt.Errorf("failed repository walk: %w", err)
		return
	}
	for _, gotFile := range gotFiles {
		// Always ignore files in root of repository
		if !strings.ContainsRune(string(gotFile), os.PathSeparator) {
			continue
		}
		// Always ignore (files under) directories with underscore prefix
		if str.HasPrefix(gotFile, deployment.IgnoreDirectoryPrefix) {
			continue
		}
		files = append(files, gotFile)
	}
	ref.allFiles = files
	ref.allFilesOK = true
	return
}
