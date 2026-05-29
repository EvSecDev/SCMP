// Package for all custom filesystem operations
package filesystem

import "scmp/internal/str"

// Struct for metadata json in config files
type MetaHeader struct {
	TargetFileOwnerGroup    string              `json:"FileOwnerGroup"`
	TargetFilePermissions   int                 `json:"FilePermissions"`
	ExternalContentLocation string              `json:"ExternalContentLocation,omitempty"`
	SymbolicLinkTarget      str.RemotePath      `json:"SymbolicLinkTarget,omitempty"`
	Dependencies            []str.LocalRepoPath `json:"Dependencies,omitempty"`
	PreDeployCommands       []string            `json:"PreDeploy,omitempty"`
	InstallCommands         []string            `json:"Install,omitempty"`
	CheckCommands           []string            `json:"Checks,omitempty"`
	ReloadCommands          []string            `json:"Reload,omitempty"`
	ReloadGroup             str.ReloadID        `json:"ReloadGroup,omitempty"`
}
