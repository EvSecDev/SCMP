package resolve

import (
	"fmt"
	"scmp/core/drn"
	"scmp/core/drn/drnconfig"
	"scmp/internal/fsops"
	"scmp/internal/str"
)

// Single replacer used across all DRNs and input data/files
func NewReplacer(repositoryRootPath string, fileReader fsops.FileReader) (replacer *Replacer) {
	if fileReader == nil {
		fileReader = fsops.NewFileSystemReader(repositoryRootPath)
	}
	replacer = &Replacer{
		repoRootDir:     repositoryRootPath,
		unvalidatedDRNs: make(map[originKey][]string),
		originOfDRN:     make(map[originKey][]*drn.DRC),
		cache:           make(map[str.DRN]str.DRNVal),
		extConfigs:      make(map[string]*drnconfig.CfgNode),
		fileReader:      fileReader,
	}
	return
}

// Retrieves the number of extracted (un-validated) DRNs (Not unique, total DRNs across all files/hosts)
func (replacer *Replacer) ExtractedCount() (length int) {
	replacer.extractionMutex.RLock()
	defer replacer.extractionMutex.RUnlock()
	for _, drns := range replacer.unvalidatedDRNs {
		length += len(drns)
	}
	return
}

// Use to build the root(initial) version of all found DRN configs.
// Walks every already extracted DRN from raw data/header and validates each. Halts at first invalid DRN
func (replacer *Replacer) initDRConfigs() (err error) {
	replacer.extractionMutex.Lock()
	pendingDRNs := replacer.unvalidatedDRNs
	replacer.unvalidatedDRNs = make(map[originKey][]string) // Reset
	replacer.extractionMutex.Unlock()

	if len(pendingDRNs) == 0 {
		// No-op
		return
	}

	replacer.originMutex.Lock()
	defer replacer.originMutex.Unlock()

	for originKey, candidates := range pendingDRNs {
		var validDRNs []*drn.DRC
		for _, candidate := range candidates {
			var drnCfg drn.DRC
			drnCfg, err = drn.Validate(candidate)
			if err != nil {
				if originKey.headerField != "" {
					err = fmt.Errorf("%s file '%s' header %s: %w",
						originKey.globalID, originKey.file, originKey.headerField, err)
				} else {
					err = fmt.Errorf("%s file '%s': %w",
						originKey.globalID, originKey.file, err)
				}
				return
			}
			validDRNs = append(validDRNs, &drnCfg)
		}

		replacer.originOfDRN[originKey] = append(replacer.originOfDRN[originKey], validDRNs...)
	}

	return
}
