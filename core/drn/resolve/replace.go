package resolve

import (
	"bytes"
	"fmt"
	"scmp/core/deployment"
	"scmp/core/drn"
	"scmp/internal/str"
	"strings"
)

// Replaces any found DRN strings in header fields with resolved value
func (replacer *Replacer) ReplaceHeaderDRNs(hostAlias str.RepoRootDir, file str.LocalRepoPath, header deployment.FileInfo) (newHeader deployment.FileInfo, replaceMade bool, err error) {
	batches := map[string][]string{
		headerPreDeploy:   header.Predeploy,
		headerInstall:     header.Install,
		headerPostInstall: header.PostInstall,
		headerPreapply:    header.Preapply,
		headerPostapply:   header.Postapply,
		headerReload:      header.Reload,
	}

	for headerField, inputs := range batches {
		if len(inputs) == 0 {
			continue
		}

		key := originKey{
			globalID:    hostAlias,
			file:        file,
			headerField: headerField,
		}

		replacer.originMutex.Lock()
		drnsToReplace, validHost := replacer.originOfDRN[key]
		if !validHost {
			replacer.originMutex.Unlock()
			continue // No match
		}
		replacer.originMutex.Unlock()

		for _, drc := range drnsToReplace {
			if drc == nil {
				continue
			}
			if drc.Original == "" || drc.Resolved == "" {
				continue
			}

			for index, input := range inputs {
				batches[headerField][index] = strings.ReplaceAll(input, string(drc.Original), string(drc.Resolved))
				replaceMade = true
			}
		}
	}

	newHeader = header
	return
}

// Replaces any found DRN strings in file data with resolved value
func (replacer *Replacer) ReplaceDRNs(hostAlias str.RepoRootDir, file str.LocalRepoPath, data []byte) (newData []byte, replaceMade bool, err error) {
	if len(data) == 0 {
		return
	}

	newData = make([]byte, len(data))
	copy(newData, data)

	key := originKey{
		globalID: hostAlias,
		file:     file,
	}

	replacer.originMutex.Lock()
	drnsToReplace, validHost := replacer.originOfDRN[key]
	if !validHost {
		replacer.originMutex.Unlock()
		return // No match
	}
	replacer.originMutex.Unlock()

	if len(drnsToReplace) == 0 {
		return
	}

	for _, drc := range drnsToReplace {
		if drc.Original == "" {
			continue
		}

		if drc.Resolved == "" {
			err = fmt.Errorf("unresolved DRN: %s", drc.Original)
			return
		}

		find := []byte(drc.Original)
		replacement := []byte(drc.Resolved)

		newData = bytes.ReplaceAll(newData, find, replacement)
		replaceMade = true
	}
	if bytes.Contains(newData, []byte(drn.Prefix)) {
		var offendingLines []string
		lines := bytes.SplitSeq(newData, []byte("\n"))
		for line := range lines {
			if bytes.Contains(line, []byte(drn.Prefix)) {
				offendingLines = append(offendingLines, string(line))
			}
		}
		err = fmt.Errorf("file '%s': replaced text still contains DRN in line(s): %v", file, offendingLines)
		return
	}

	return
}
