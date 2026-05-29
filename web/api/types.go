// Package for all backend HTTP server API logic
package api

import (
	"context"
	"scmp/core/deployment/metrics"
	"scmp/internal/str"
)

// ====================== GENERICS ======================
type PaginationReq struct {
	Limit     int    `json:"limit"`               // limit response to this number of items
	Offset    int    `json:"offset"`              // number of items to offset from start
	SortBy    string `json:"sortBy,omitempty"`    // field name in response to sort by
	SortOrder string `json:"sortOrder,omitempty"` // "asc" or "desc"
}
type NilSuccess struct {
	Status string `json:"status"` // Generic string to place in result when no actual data is to be sent back
}

// ====================== AUTHENTICATION ======================
type UserLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type AuthToken struct {
	Username     string `json:"username"`
	Token        string `json:"id_token"`
	ValidTime    int    `json:"validTime"`
	RedirectPage string `json:"redirectTo,omitempty"`
}
type UserLogout struct {
	RedirectPage string `json:"redirectTo,omitempty"`
}

// ====================== SETTINGS ======================

type RepoList struct {
	Repositories []string `json:"repositories"`
}

type HostListReq struct {
	WithDetails bool `json:"withDetails"`
}
type HostList struct {
	Hosts   []str.RepoRootDir                `json:"hosts"`
	Details map[str.RepoRootDir]HostSettings `json:"hostDetails,omitempty"`
}

// See global for more info
type HostSettings struct {
	DeploymentState string            `json:"state"`
	IgnoreUniversal bool              `json:"ignoresUniversal"`
	RequiresVault   bool              `json:"requiresVault"`
	UniversalGroups []str.RepoRootDir `json:"groups"`
	Proxy           string            `json:"proxy,omitempty"`
	Endpoint        string            `json:"address"`
	EndpointUser    string            `json:"loginUser"`
	IdentityFile    string            `json:"identityFile,omitempty"`
	ConnectTimeout  int               `json:"connectTimeout,omitempty"`
}

// ====================== FILESYSTEM ======================
type PathRequest struct {
	Path string `json:"path"`
}

type DownloadLink struct {
	Location string `json:"downloadLocation"`
}

type ProcessUploadReq struct {
	Path   string `json:"path"`
	DataID string `json:"dataID"`
}

type FileMetadata struct {
	Path                    string              `json:"path"`
	Type                    string              `json:"type"`
	Size                    int                 `json:"size"`
	OwnerName               string              `json:"ownerName"`
	GroupName               string              `json:"groupName"`
	Permissions             string              `json:"permissions"`
	LastModified            string              `json:"lastModified,omitempty"`
	ExternalContentLocation string              `json:"externalContentLocation,omitempty"`
	SymbolicLinkTarget      string              `json:"symbolicLinkTarget,omitempty"`
	Dependencies            []str.LocalRepoPath `json:"dependencies,omitempty"`
	PreDeployCommands       []string            `json:"preDeployCommands,omitempty"`
	InstallCommands         []string            `json:"installCommands,omitempty"`
	CheckCommands           []string            `json:"checkCommands,omitempty"`
	ReloadCommands          []string            `json:"reloadCommands,omitempty"`
	ReloadGroup             str.ReloadID        `json:"reloadGroup,omitempty"`
}

type FileOp struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	Recursive bool   `json:"recursive"`
}
type FileMove struct {
	SourcePath           string `json:"sourcePath"`           // Full path
	DestinationPath      string `json:"destinationPath"`      // Full path
	DeleteSource         bool   `json:"deleteSource"`         // move instead of copy
	OverwriteDestination bool   `json:"overwriteDestination"` // ignore present dest
}

type FilePathSearchReq struct {
	Path      string `json:"path"`       // base directory to search from
	Query     string `json:"query"`      // search text
	QueryType string `json:"searchType"` // type of match (contains, prefix, suffix)
	FileType  string `json:"fileType"`   // type of file (all, directory, file)
	Depth     int    `json:"depth"`      // max directory recursion (default is current dir only, 0)
}
type FilePathSearchResults struct {
	OriginalReq FilePathSearchReq `json:"orig"`       // Original query
	MatchCount  int               `json:"matchCount"` // number of results
	Matches     []FileMetadata    `json:"matches"`    // list of files/dirs by their meta
}

type PathList struct {
	Paths []string `json:"paths"`
}

// ====================== REPOSITORY ======================
type RepoStatus struct {
	Staged   []RepoFilestatus `json:"staged"`
	Unstaged []RepoFilestatus `json:"unstaged"`
}

type RepoFilestatus struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type RepoCommit struct {
	Message string `json:"message"`
}
type RepoCommitHistory struct {
	Commits []RepoCommitInfo `json:"commits"`
}
type RepoCommitInfo struct {
	ShortHash       string           `json:"shortHash"`
	FullHash        string           `json:"fullHash"`
	Date            string           `json:"date"`
	AuthorName      string           `json:"authorName"`
	AuthorEmail     string           `json:"authorEmail"`
	NumberOfChanges int              `json:"numberOfChanges"`
	FilesChanged    []RepoFilestatus `json:"filesChanged"`
	Message         string           `json:"message"`
	GPGSignature    string           `json:"gpgSignature,omitempty"`
	Branches        []string         `json:"branches,omitempty"`
	Tags            []string         `json:"tags,omitempty"`
}

type RepoFileDiffReq struct {
	Path           string `json:"path"`
	TgtCommitHash  string `json:"targetCommit"`
	BaseCommitHash string `json:"baseCommit"`
}
type RepoFileDiffResp struct {
	Files []DiffFile `json:"files"`
}
type DiffFile struct {
	OldPath    string     `json:"old_path,omitempty"` // Original path (before rename/move)
	NewPath    string     `json:"new_path,omitempty"` // New path (after rename/move)
	ChangeType string     `json:"change_type"`        // added, modified, deleted, renamed, etc.
	IsBinary   bool       `json:"is_binary"`          // True if file is binary
	Hunks      []DiffHunk `json:"hunks,omitempty"`    // List of changed hunks (if not binary)
}
type DiffHunk struct {
	OldStartLine int          `json:"old_start_line"` // Start line in the old file
	OldLineCount int          `json:"old_line_count"` // Number of lines affected in old file
	NewStartLine int          `json:"new_start_line"` // Start line in the new file
	NewLineCount int          `json:"new_line_count"` // Number of lines affected in new file
	Changes      []LineChange `json:"changes"`        // Line-level changes
}
type LineChange struct {
	Type    string `json:"type"`    // "add", "del", "context"
	Content string `json:"content"` // Line content
}

// ====================== DEPLOYMENTS ======================
type DeployStatusReq struct {
	ID string `json:"deploymentID"`
}
type DeployStatus struct {
	ID            string `json:"deploymentID"`
	Status        string `json:"status"`
	Pending       bool   `json:"pending,omitempty"`
	PendingAction any    `json:"pendingAction,omitempty"`
}
type DeployStart struct {
	Mode string `json:"mode"`
	Type string `json:"type"`
	Opts struct {
		AllowDeletions     bool   `json:"allowDeletions"`
		RunInstallCmds     bool   `json:"runInstall"`
		DisableReloads     bool   `json:"disableReloads"`
		DisableSudo        bool   `json:"disableSudo"`
		IgnoreHostState    bool   `json:"ignoreHostState"`
		Force              bool   `json:"force"`
		AutoCommitRollback bool   `json:"autoCommitRollbackEnabled"`
		CommitID           string `json:"commitID"`
		HostOverride       string `json:"hostOverride"`
		FileOverride       string `json:"fileOverride"`
		RunAsUser          string `json:"runAsUser"`
		MaxSSHConn         int    `json:"maxSSHConnections"`
		MaxSSHChannels     int    `json:"maxSSHChannels"`
		CommandTimeout     int    `json:"maxCommandRuntime"`
		Verbosity          int    `json:"verbosity"`
	} `json:"options"`
}
type DeployAbort struct {
	ID            string `json:"deploymentID"`
	StopRequested bool   `json:"stopRequested"`
}
type DeployOutput struct {
	ID        string          `json:"deploymentID"`
	Status    string          `json:"status"`
	Details   metrics.Summary `json:"summary,omitempty"`
	RawOutput string          `json:"rawOutput,omitempty"`
}

// Internal use
type deploymentTracker struct {
	associatedDataID string // For prompts
	abort            context.CancelFunc
	output           []string
	status           string
	errorStr         string
}

// ====================== PROMPT ======================
type PromptAnswer struct {
	AssociatedDataID string `json:"associatedDataID"` // uuid for datastore location
	PromptID         string `json:"promptID"`         // uuid for prompt within prompt queue
	Data             string `json:"encodedData"`      // base64
}

// ====================== SEED ======================
type SeedRequest struct {
	Hosts       []string `json:"hosts"`
	Interactive bool     `json:"interactive"`
	StartPath   string   `json:"startPath"`
	Files       []string `json:"remoteFiles,omitempty"`
}
type SeedResp struct {
	SeedID      string   `json:"sessionID"`
	CurrentPath string   `json:"path"`
	FileList    []string `json:"fileList"`
}
type SeedSelections struct {
	SeedID     string          `json:"sessionID"`
	Selections []SeedSelection `json:"selections"`
	Finished   bool            `json:"finished"`
}
type SeedSelection struct {
	Path              string `json:"path"`
	UseDefaultReloads bool   `json:"useDefaultReloads"`
	Recursive         bool   `json:"recursive"`
}
type SeedSelectSuccess struct {
	SeedID       string   `json:"sessionID"`
	SuccessFiles []string `json:"successFiles,omitempty"`
	FailedFiles  []string `json:"failedFiles,omitempty"`
}
