// Package for deployment orchestration per host and file
package host

import (
	"scmp/core/deployment/metrics"
	"scmp/internal/config"
	"scmp/internal/sshinternal"
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
