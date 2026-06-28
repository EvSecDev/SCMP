package drn

import (
	"os"
	"path/filepath"
	"scmp/internal/str"
	"slices"
	"strings"
)

// Helper to extract a DRN namespace from a path.
// Returns empty slice if path does not contain a DRN config or the known DRN parent directory name
func PathToNamespace(path string) (namespace []string) {
	segments := strings.Split(path, string(os.PathSeparator))
	if !slices.Contains(segments, ExternalVariableDirectory) {
		// Path did not have _global
		return
	}
	nsStartIndex := slices.Index(segments, ExternalVariableDirectory)
	if nsStartIndex == len(segments)-1 {
		// Path just ended in _global
		return
	}
	namespace = segments[nsStartIndex+1:]
	return
}

// Helper to convert from namespace to absolute path
func NamespaceToPath(repositoryRoot string, namespace []string) (absolutePath string) {
	pathElems := []string{repositoryRoot, ExternalVariableDirectory}
	pathElems = append(pathElems, namespace...)
	absolutePath = filepath.Join(pathElems...)
	return
}

// Finds changed DRN strings between two maps, key=DRN, value=DRN val
func DiffSet(before, after map[str.DRN]str.DRNVal) (result []str.DRN) {
	changed := make(map[str.DRN]struct{})

	for drn, beforeValue := range before {
		afterValue, exists := after[drn]

		if !exists || beforeValue != afterValue {
			changed[drn] = struct{}{}
		}
	}

	for drn := range after {
		_, exists := before[drn]
		if !exists {
			changed[drn] = struct{}{}
		}
	}

	result = make([]str.DRN, 0, len(changed))
	for drn := range changed {
		result = append(result, drn)
	}
	return
}

// Peaks prefix of input and determines if the input looks like a DRN
func LikelyDRN(input string) (maybeDRN bool) {
	if strings.HasPrefix(input, OpenDelimiter+Prefix) {
		maybeDRN = true
	}
	return
}
