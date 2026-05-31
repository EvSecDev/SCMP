package metrics

import (
	"scmp/core/deployment"
	"scmp/internal/str"
)

func (metric *Metrics) AddAllDeployFiles(host str.RepoRootDir, files *deployment.HostFiles) {
	metric.hostFilesMutex.Lock()
	for _, fileGroup := range files.Groups {
		metric.hostFiles[host] = append(metric.hostFiles[host], fileGroup.GetOrderedList()...)
	}
	metric.hostFilesMutex.Unlock()

	metric.fileActionMutex.Lock()
	for _, fileGroup := range files.Groups {
		for _, file := range fileGroup.GetOrderedList() {
			info := files.GetFileInfo(file)
			metric.fileAction[file] = info.Action
		}
	}
	metric.fileActionMutex.Unlock()
}

func (metric *Metrics) AddFile(host str.RepoRootDir, deployFiles *deployment.HostFiles, files ...str.LocalRepoPath) {
	metric.hostFilesMutex.Lock()
	metric.hostFiles[host] = append(metric.hostFiles[host], files...)
	metric.hostFilesMutex.Unlock()

	metric.fileActionMutex.Lock()
	for _, file := range files {
		info := deployFiles.GetFileInfo(file)
		metric.fileAction[file] = info.Action
	}
	metric.fileActionMutex.Unlock()
}

func (metric *Metrics) AddFileFailure(hostname str.RepoRootDir, file str.LocalRepoPath, err error) {
	if err == nil {
		return
	}

	metric.hostsFileErrMutex.Lock()
	hostFileErr := metric.hostsFileErr[hostname]
	if hostFileErr == nil {
		hostFileErr = make(map[str.LocalRepoPath]error)
	}
	hostFileErr[file] = err
	metric.hostsFileErr[hostname] = hostFileErr
	metric.hostsFileErrMutex.Unlock()
}

// Checks if the repository file path for a given host has had an error recorded
func (metric *Metrics) HostFileHasError(host str.RepoRootDir, repoFilePath str.LocalRepoPath) (err error) {
	metric.hostsFileErrMutex.RLock()
	defer metric.hostsFileErrMutex.RUnlock()

	hostFileErr, ok := metric.hostsFileErr[host]
	if !ok {
		return
	}

	err = hostFileErr[repoFilePath]
	return
}
