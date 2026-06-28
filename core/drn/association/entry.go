package association

import (
	"context"
	"fmt"
	"scmp/internal/str"
	"slices"
)

// Given a list of concrete DRNs, find every local repository file that reference that DRN and the hosts associated with those files (in case of Universal)
func (ref ReferenceFinder) FilesReferencingExternals(ctx context.Context, drnList []str.DRN) (relatedFiles []str.LocalRepoPath, relatedHosts []str.RepoRootDir, err error) {
	searchTerms, err := ref.buildSearchTerms(drnList)
	if err != nil {
		err = fmt.Errorf("search term builder: %w", err)
		return
	}
	result, err := ref.fileSeacher(ctx, searchTerms)
	if err != nil {
		err = fmt.Errorf("repository search: %w", err)
		return
	}

	rootSet := make(map[string]struct{}, len(drnList))
	for _, r := range drnList {
		rootSet[string(r)] = struct{}{}
	}

	depSet, depToRoot, err := ref.buildDependentSet(drnList)
	if err != nil {
		err = fmt.Errorf("dependent set: %w", err)
		return
	}

	relatedFiles = make([]str.LocalRepoPath, 0, len(result))
	hostSet := make(map[str.RepoRootDir]struct{})

	for filePath := range result {
		path := str.LocalRepoPath(filePath)

		var content []byte
		content, err = ref.fileReader(path)
		if err != nil {
			err = fmt.Errorf("reading '%s': %w", path, err)
			return
		}

		var fileHosts map[str.RepoRootDir]struct{}
		fileHosts, err = ref.classifyFileReferences(content, path, rootSet, depSet, depToRoot)
		if err != nil {
			err = fmt.Errorf("reference classification: %w", err)
			return
		}

		if len(fileHosts) > 0 {
			relatedFiles = append(relatedFiles, path)
			for h := range fileHosts {
				hostSet[h] = struct{}{}
			}
		}
	}

	relatedHosts = make([]str.RepoRootDir, 0, len(hostSet))
	for host := range hostSet {
		relatedHosts = append(relatedHosts, host)
	}

	slices.Sort(relatedFiles)
	slices.Sort(relatedHosts)
	return
}
