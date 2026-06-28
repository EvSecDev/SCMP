package deployment

import (
	"scmp/internal/str"
)

func NewHostFiles() (files *HostFiles, err error) {
	files = &HostFiles{
		metadata: make(map[str.LocalRepoPath]FileInfo),
		data:     make(map[str.FileID][]byte),
	}
	return
}

// Makes a copy of all relevant global file metadata and data after sorting groups are created
func (files *HostFiles) CopyGlobalFiles(sourceList []str.LocalRepoPath, allFiles *AllFiles) {
	for _, file := range sourceList {
		info := allFiles.GetFileInfo(file)
		data := allFiles.GetFileData(info.Hash)

		dataCopy := make([]byte, len(data))
		copy(dataCopy, data)

		files.mutex.Lock()

		_, alreadyLoaded := files.data[info.Hash]
		if !alreadyLoaded {
			files.data[info.Hash] = dataCopy
		}

		files.metadata[file] = info

		files.mutex.Unlock()
	}
}

// Retrieves unordered list of all files associated with the host
func (files *HostFiles) GetUnorderedList() (list []str.LocalRepoPath) {
	files.mutex.RLock()
	for path := range files.metadata {
		list = append(list, path)
	}
	files.mutex.RUnlock()
	return
}

func (files *HostFiles) StoreDataOnce(identifier str.FileID, content []byte) {
	files.mutex.Lock()
	_, alreadyLoaded := files.data[identifier]
	if !alreadyLoaded {
		files.data[identifier] = content
	}
	files.mutex.Unlock()
}

func (files *HostFiles) SetFileMetadata(path str.LocalRepoPath, metadata FileInfo) {
	files.mutex.Lock()
	_, alreadyLoaded := files.metadata[path]
	if !alreadyLoaded {
		files.metadata[path] = metadata
	}
	files.mutex.Unlock()
}

func (files *HostFiles) GetFileInfo(path str.LocalRepoPath) (info FileInfo) {
	files.mutex.RLock()
	defer files.mutex.RUnlock()
	info = files.metadata[path]
	return
}

func (files *HostFiles) GetFileData(identifier str.FileID) (data []byte) {
	files.mutex.RLock()
	data = files.data[identifier]
	files.mutex.RUnlock()
	return
}

func (files *HostFiles) ChangeFileDataPointer(path str.LocalRepoPath, newIdentifier str.FileID) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	info, validPath := files.metadata[path]
	if !validPath {
		return
	}
	info.Hash = newIdentifier
	files.metadata[path] = info
}
