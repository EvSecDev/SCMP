package deployment

import (
	"fmt"
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
func (files *HostFiles) CopyGlobalFiles(allFiles *AllFiles) (err error) {
	if len(files.Groups) == 0 {
		err = fmt.Errorf("host file groups were not initialized")
		return
	}
	for _, group := range files.Groups {
		if group == nil {
			continue
		}

		for _, file := range group.list {
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

func (files *HostFiles) GetFileInfo(path str.LocalRepoPath) (info FileInfo) {
	files.mutex.RLock()
	info, validPath := files.metadata[path]
	if !validPath {
		return
	}
	files.mutex.RUnlock()
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
