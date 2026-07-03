package resolve

import (
	"scmp/core/deployment"
	"scmp/core/drn"
	"scmp/internal/str"
	"strings"
)

// Extracts all DRN strings from relevant sections of a metadata header
func (replacer *Replacer) ExtractHeaderDRNs(hostAlias str.RepoRootDir, file str.LocalRepoPath, header deployment.FileInfo) {
	baseKey := originKey{
		globalID: hostAlias,
		file:     file,
	}

	batches := map[string][]string{
		headerPreDeploy:   header.Predeploy,
		headerInstall:     header.Install,
		headerPostInstall: header.PostInstall,
		headerPreapply:    header.Preapply,
		headerPostapply:   header.Postapply,
		headerReload:      header.Reload,
	}

	for field, cmds := range batches {
		if len(cmds) == 0 {
			continue
		}

		var drns []string
		for _, cmd := range cmds {
			drns = append(drns, ExtractStringDRN(cmd)...)
		}

		if len(drns) == 0 {
			// No DRNs in any commands
			continue
		}

		key := baseKey
		key.headerField = field

		replacer.extractionMutex.Lock()
		replacer.unvalidatedDRNs[key] = append(replacer.unvalidatedDRNs[key], drns...)
		replacer.extractionMutex.Unlock()
	}
}

// Simple string extraction from input matching a drn
func ExtractStringDRN(input string) (drns []string) {
	if len(input) < drn.MinTotalLength {
		return
	}
	for len(input) >= drn.MinTotalLength {
		idx := strings.Index(input, drn.OpenDelimiter+drn.Prefix)
		if idx == -1 {
			break
		}

		// Find the first close delimiter after the prefix start
		closeIndex := strings.Index(input[idx:], drn.CloseDelimiter)
		if closeIndex != -1 {
			drns = append(drns, input[idx:idx+closeIndex+1])
			// Advance past the delimiter to continue searching
			input = input[idx+closeIndex+1:]
		}
	}
	return
}

// Extracts a list of DRN strings from raw bytes
func (replacer *Replacer) ExtractDRNs(hostAlias str.RepoRootDir, file str.LocalRepoPath, data []byte) {
	drnCandidates := ExtractRawDRNs(data)
	if len(drnCandidates) == 0 {
		// No DRNs found anywhere in data
		return
	}

	key := originKey{
		globalID: hostAlias,
		file:     file,
	}

	replacer.extractionMutex.Lock()
	replacer.unvalidatedDRNs[key] = append(replacer.unvalidatedDRNs[key], drnCandidates...)
	replacer.extractionMutex.Unlock()
}

// Greedily extracts <scmp://...> strings from raw bytes.
// Does NOT validate DRNs. (Intentional to catch potential user typos with the validator later.)
func ExtractRawDRNs(data []byte) (drnCandidates []string) {
	if len(data) == 0 {
		return
	}

	p0 := drn.OpenDelimiter[0]
	plen := len(drn.OpenDelimiter) + len(drn.Prefix)

	prefixStr := drn.OpenDelimiter + drn.Prefix

	drnCandidates = make([]string, 0, 16)
	for i := 0; i < len(data); i++ {
		// Exclude non-matching first characters immediately
		if data[i] != p0 {
			continue
		}

		// Immediately skip data that could not contain DRN length
		if i+plen > len(data) {
			continue
		}

		// Manual prefix check - scan through data to match the whole prefix
		prefixFound := true
		for j := 1; j < plen; j++ {
			if data[i+j] != prefixStr[j] {
				// Bail early on first non-matching character
				prefixFound = false
				break
			}
		}
		if !prefixFound {
			continue
		}

		prefixStartIndex := i
		i += plen

		// Find the index of the first space following the prefix
		drnEndIndex := i
		for drnEndIndex < len(data) && data[drnEndIndex] != []byte(drn.CloseDelimiter)[0] {
			drnEndIndex++
		}

		// No closing delimiter found - malformed DRN, skip gracefully
		if drnEndIndex == len(data) {
			continue
		}

		// DRN-possible slug
		possibleDRN := data[prefixStartIndex : drnEndIndex+1]

		// Record the entire DRN string
		drnCandidates = append(drnCandidates, string(possibleDRN))

		// Start next scan at end of this DRN
		i = drnEndIndex
	}
	return
}
