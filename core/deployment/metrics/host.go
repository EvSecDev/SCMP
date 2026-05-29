package metrics

import (
	"scmp/internal/str"
)

func (metric *Metrics) HostHasError(host str.RepoRootDir) (errorPresent bool) {
	if len(metric.hostsFileErr[host]) > 0 {
		errorPresent = true
	}
	return
}

func (metric *Metrics) AddHostBytes(host str.RepoRootDir, deployedBytes int) {
	// Lock and write to metric var - increment total transferred bytes
	if deployedBytes > 0 {
		metric.hostBytesMutex.Lock()
		metric.hostBytes[host] += deployedBytes
		metric.hostBytesMutex.Unlock()
	}
}

func (metric *Metrics) AddHostFailure(host str.RepoRootDir, err error) {
	if err == nil {
		return
	}
	metric.hostErrMutex.Lock()
	metric.hostErr[host] = err
	metric.hostErrMutex.Unlock()
}
