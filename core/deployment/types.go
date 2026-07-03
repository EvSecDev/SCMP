// Package for containing all deployment-related code
package deployment

import (
	"scmp/internal/str"
	"sync"
)

// Represents all files across all hosts for deployment
type AllFiles struct {
	metadata map[str.LocalRepoPath]FileInfo
	data     map[str.FileID][]byte
	mutex    sync.RWMutex
}

// Wrapper for host lists and global list
type HostFiles struct {
	Groups   []*FileGroup
	metadata map[str.LocalRepoPath]FileInfo
	data     map[str.FileID][]byte
	mutex    sync.RWMutex
}

// Represents files to be deployed in serial for a given host
type FileGroup struct {
	list              []str.LocalRepoPath                  // Ordered list of files to deploy together
	reloadIDtoFile    map[str.ReloadID][]str.LocalRepoPath // Lookup of file list by reload ID - File slice ordered the same as above list
	fileToReloadID    map[str.LocalRepoPath]str.ReloadID   // Lookup of a files reload ID
	reloadIDfileCount map[str.ReloadID]int                 // Total files in reload group
	reloadIDcommands  map[str.ReloadID][]string            // Ordered list of reload commands - STABLE ORDER OF COMMANDS - REQUIRED TO ENSURE PROPER RELOAD SEQUENCE
	mutex             sync.RWMutex
}

// Struct for deployment file metadata
type FileInfo struct {
	Hash              str.FileID        // Pointer (key) to file data map (for deduplication)
	RepoFilePath      str.LocalRepoPath // Source path relative to repository
	TargetFilePath    str.RemotePath    // Expected remote file path
	Action            str.DeployAction
	OwnerGroup        string
	Permissions       int
	FileSize          int
	LinkTarget        str.RemotePath
	Dependencies      []str.LocalRepoPath // List of files required by this file
	PredeployRequired bool
	Predeploy         []string // Command list
	InstallOptional   bool
	Install           []string // Command list
	ChecksRequired    bool
	Checks            []string // Command list
	ReloadRequired    bool
	Reload            []string     // Command list
	ReloadGroup       str.ReloadID // Named string defined by user to manually group files together
}
