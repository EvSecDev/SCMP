// Package for wrapping standard string types and functions with custom types for strict text handling
package str

type StringLike interface {
	~string
}

type LocalRepoPath string // Relative path in the repository starting with the host/universal name then remote path
type RemotePath string    // Absolute path for a remote file/dir
type RepoRootDir string   // Top level directory in repository for identifying host OR universal group
type FileID string        // Unique identifier for file contents (i.e. hash)
type DeployAction string  // File action to be done during deployment (on remote) (i.e. create)
type ReloadID string      // Unique identifier for reload commands (and/or reload group)
