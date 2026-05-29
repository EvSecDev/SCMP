package host

import (
	"scmp/core/deployment/metrics"
	"scmp/internal/config"
	"sync"
)

func New(wg *sync.WaitGroup, connLimiter chan struct{}, endpointInfo config.EndpointInfo, proxyInfo config.EndpointInfo, metrics *metrics.Metrics, maxDeployConcurrency int) (deployer *Deployer) {
	deployer = &Deployer{
		allHostWG:   wg,
		connLimiter: connLimiter,
		host:        endpointInfo,
		proxy:       proxyInfo,

		metrics: metrics,

		deployWG:             &sync.WaitGroup{},
		deployLimiter:        make(chan struct{}, maxDeployConcurrency),
		maxConcurrentDeploys: maxDeployConcurrency,
	}
	return
}

func newGroupDeployer(hostDeployer *Deployer) (group *fileGroup) {
	group = &fileGroup{
		deployWG:      hostDeployer.deployWG,
		deployLimiter: hostDeployer.deployLimiter,
		hostState:     hostDeployer.state,
		metrics:       hostDeployer.metrics,
	}
	return
}
