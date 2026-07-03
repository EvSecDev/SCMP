// Package for deployment orchestration per host and file
package host

import (
	"scmp/core/deployment"
	"scmp/core/deployment/metrics"
	"scmp/internal/config"
	"scmp/internal/sshinternal"
	"scmp/internal/str"
	"sync"
)

// Per-host deployer state
type Deployer struct {
	allHostWG   *sync.WaitGroup
	connLimiter chan struct{}
	host        config.EndpointInfo
	proxy       config.EndpointInfo

	metrics *metrics.Metrics

	state sshinternal.HostMeta

	deployWG             *sync.WaitGroup
	deployLimiter        chan struct{}
	maxConcurrentDeploys int
}

// Per-file-group deployer state
type fileGroup struct {
	deployWG      *sync.WaitGroup
	deployLimiter chan struct{}
	hostState     sshinternal.HostMeta
	metrics       *metrics.Metrics
}

type reloadTracker struct {
	fileGroup                *deployment.FileGroup
	hostFiles                *deployment.HostFiles
	hostEndpointName         str.RepoRootDir
	totalDeployedReloadFiles map[str.ReloadID]int                             // Count of successfully deployed files by their reloadID
	reloadIDreadyToReload    map[str.ReloadID]bool                            // Signal when a reload group is cleared to reload
	remoteFileMetadatas      map[str.LocalRepoPath]sshinternal.RemoteFileInfo // Track remote file metadata (mainly for reload failure restoration)
}
