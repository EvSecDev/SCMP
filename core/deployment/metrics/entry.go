package metrics

import (
	"scmp/internal/str"
	"time"
)

func New() (new *Metrics) {
	new = &Metrics{
		hostFiles:    make(map[str.RepoRootDir][]str.LocalRepoPath),
		hostBytes:    make(map[str.RepoRootDir]int),
		hostsFileErr: make(map[str.RepoRootDir]map[str.LocalRepoPath]error),
		hostErr:      make(map[str.RepoRootDir]error),
		fileAction:   make(map[str.LocalRepoPath]str.DeployAction),
		startTime:    time.Now(),
	}
	return
}

func (metric *Metrics) Stop() {
	metric.endTime = time.Now()
}

func (metric *Metrics) AnyErrorsPresent() (errorsPresent bool) {
	if len(metric.hostsFileErr) > 0 {
		errorsPresent = true
	}
	return
}
