// Package for recording various metrics and final state of a deployment
package metrics

import (
	"scmp/internal/str"
	"sync"
	"time"
)

// Used for metrics - counting post deployment
type Metrics struct {
	startTime         time.Time
	hostFiles         map[str.RepoRootDir][]str.LocalRepoPath // Key on hostname, list of files deployed to host
	hostFilesMutex    sync.Mutex
	hostErr           map[str.RepoRootDir]error // Error for host (agnostic of files)
	hostErrMutex      sync.Mutex
	hostsFileErr      map[str.RepoRootDir]map[str.LocalRepoPath]error // Key on hostname, key on repo file path, value of error (ensures file errors are always scoped to host)
	hostsFileErrMutex sync.RWMutex
	fileAction        map[str.LocalRepoPath]str.DeployAction
	fileActionMutex   sync.Mutex
	hostBytes         map[str.RepoRootDir]int
	hostBytesMutex    sync.Mutex
	endTime           time.Time
}

// Summary of actions done and collected metrics
// Status could be UpToDate,Deployed,Partial,Failed
type Summary struct {
	Status          string `json:"Status"`
	StartTime       string `json:"Start-Time"`
	EndTime         string `json:"End-Time"`
	ElapsedTime     string `json:"Elapsed-Time"`     // Human readable
	TransferredData string `json:"Transferred-Size"` // Human readable
	Counters        struct {
		Hosts          int `json:"Hosts" `
		Items          int `json:"Items"`
		CompletedHosts int `json:"Hosts-Completed"`
		CompletedItems int `json:"Items-Completed"`
		FailedHosts    int `json:"Hosts-Failed"`
		FailedItems    int `json:"Items-Failed"`
	} `json:"Counters"`
	CommitID string        `json:"Deployment-Commit-Hash"`
	Hosts    []HostSummary `json:"Hosts,omitempty"`
}

type HostSummary struct {
	Name            str.RepoRootDir `json:"Name"`
	Status          string          `json:"Status,omitempty"`
	ErrorMsg        string          `json:"Error-Message,omitempty"`
	TotalItems      int             `json:"Total-Items,omitempty"`
	TransferredData string          `json:"Transferred-Size,omitempty"`
	Items           []ItemSummary   `json:"Items,omitempty"`
}

type ItemSummary struct {
	Name     str.LocalRepoPath `json:"Name"`
	Action   str.DeployAction  `json:"Deployment-Action"`
	Status   string            `json:"Status,omitempty"`
	ErrorMsg string            `json:"Error-Message,omitempty"`
}
