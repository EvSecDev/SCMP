package association

import (
	"os"
	"scmp/internal/config"
	"scmp/internal/str"
	"strings"
)

// Checks whether a file belongs to a given deployment host.
func fileAppliesToHost(file str.LocalRepoPath, host str.RepoRootDir, hostInfo config.EndpointInfo, universalDirectory str.RepoRootDir) (applicable bool) {
	segments := strings.SplitN(string(file), string(os.PathSeparator), 2)
	fileHost := str.RepoRootDir(segments[0])

	if fileHost == host || fileHost == universalDirectory {
		applicable = true
		return
	}
	_, hostInGroup := hostInfo.UniversalGroups[fileHost]
	applicable = hostInGroup
	return
}

// Returns every host that a file is valid for.
// A file in a host-specific directory applies only to that host; a file in the universal directory applies to any host that includes it.
// This mirrors deployment scoping rules at the file level rather than the DRN level.
func (ref *ReferenceFinder) collectApplicableHosts(path str.LocalRepoPath) (fileHosts map[str.RepoRootDir]struct{}) {
	fileHosts = make(map[str.RepoRootDir]struct{})
	for hostAlias, hostInfo := range ref.hostInfo {
		if fileAppliesToHost(path, hostAlias, hostInfo, ref.primaryUniversalDirectory) {
			fileHosts[hostAlias] = struct{}{}
		}
	}
	return
}
