// Package for finding files and hosts that reference a set of concrete DRNs
package association

import (
	"scmp/internal/config"
	"scmp/internal/fsops"
	"scmp/internal/str"
)

// Locates repository files that reference a set of DRNs and determines which deployment hosts those files apply to.
// The finder caches aggressively since it is called repeatedly during sync operations with different root DRN sets but the same underlying repository.
type ReferenceFinder struct {
	// Configuration
	repositoryPath            string
	hostInfo                  map[str.RepoRootDir]config.EndpointInfo
	primaryUniversalDirectory str.RepoRootDir

	// Authoritative DRN source (dump of every configured DRN)
	allDRNs map[str.LocalRepoPath]map[str.DRN]str.DRNVal

	// State/Caching
	reverseDeps       map[string][]str.DRN
	reverseDepsOK     bool
	macroValues       map[string][]string
	macroValuesOK     bool
	sortedMacroValues []string
	allFiles          []str.LocalRepoPath
	allFilesOK        bool

	// File system abstractions
	pathWalker  fsops.PathWalker
	fileSeacher fsops.FileSearcher
	fileReader  fsops.FileReader
}

// Accepts optional filesystem abstractions so tests can inject fake walkers/searchers/readers without touching the filesystem.
// When nil, defaults are created from cfg.RepositoryPath use the live filesystem.
func NewReferenceFinder(cfg *config.Config, allDRNs map[str.LocalRepoPath]map[str.DRN]str.DRNVal,
	walker fsops.PathWalker, searcher fsops.FileSearcher, reader fsops.FileReader,
) (referenceFinder *ReferenceFinder, err error) {
	if walker == nil {
		walker = fsops.NewFileSystemWalker(cfg.RepositoryPath)
	}
	if searcher == nil {
		searcher = fsops.NewFileSystemSearcher(cfg.RepositoryPath)
	}
	if reader == nil {
		reader = fsops.NewFileSystemReader(cfg.RepositoryPath)
	}

	referenceFinder = &ReferenceFinder{
		repositoryPath:            cfg.RepositoryPath,
		hostInfo:                  cfg.HostInfo,
		primaryUniversalDirectory: cfg.UniversalDirectory,

		allDRNs: allDRNs,

		reverseDeps: make(map[string][]str.DRN),
		macroValues: make(map[string][]string),

		pathWalker:  walker,
		fileSeacher: searcher,
		fileReader:  reader,
	}
	return
}
