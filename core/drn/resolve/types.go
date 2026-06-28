// Handles resolving DRNs and macro-containing-DRNs to concrete values
package resolve

import (
	"scmp/core/drn"
	"scmp/core/drn/drnconfig"
	"scmp/internal/fsops"
	"scmp/internal/str"
	"sync"
)

// State-keeper for replacements across multiple hosts and files
type Replacer struct {
	// Config
	repoRootDir string

	// Extraction (separated for concurrency throughput)
	extractionMutex sync.RWMutex           // Protects unvalidated DRN map
	unvalidatedDRNs map[originKey][]string // Temporary holding of unverified DRNs

	// Resolution
	originMutex sync.RWMutex             // Protects origin list
	originOfDRN map[originKey][]*drn.DRC // Identifying where DRNs where found

	// Caching expanded but not yet resolved DRNs (expanded should always be one-to-one with a value - context cannot affect it)
	cacheMutex sync.RWMutex
	cache      map[str.DRN]str.DRNVal

	// Caching external config contents since multiple DRNs might live in the same file
	extConfigMutex sync.RWMutex
	extConfigs     map[string]*drnconfig.CfgNode // Keys on DRN namespace

	// File system abstractions
	fileReader fsops.FileReader
}

type originKey struct {
	globalID    str.RepoRootDir   // Top level DRN context, either universal dir, host dir, or host alias (all treated the same as global unique IDs)
	file        str.LocalRepoPath // Which specific file DRN came from
	headerField string            // Optional, which specific header field DRN came from
}
