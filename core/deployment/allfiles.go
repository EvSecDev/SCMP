package deployment

import (
	"scmp/internal/str"
)

func NewAllFiles() (files *AllFiles) {
	files = &AllFiles{
		metadata: make(map[str.LocalRepoPath]FileInfo),
		data:     make(map[str.FileID][]byte),
	}
	return
}

func (files *AllFiles) AddMetadata(path str.LocalRepoPath, metadata FileInfo) {
	files.mutex.Lock()
	files.metadata[path] = metadata
	files.mutex.Unlock()
}

func (files *AllFiles) StoreDataOnce(identifier str.FileID, content []byte) {
	files.mutex.Lock()
	_, alreadyLoaded := files.data[identifier]
	if !alreadyLoaded {
		files.data[identifier] = content
	}
	files.mutex.Unlock()
}

func (files *AllFiles) AlreadyLoaded(identifier str.FileID) (loaded bool) {
	files.mutex.RLock()
	_, alreadyLoaded := files.data[identifier]
	if alreadyLoaded {
		loaded = true
	}
	files.mutex.RUnlock()
	return
}

func (files *AllFiles) IsEmpty() (noFiles bool) {
	files.mutex.RLock()
	if len(files.metadata) == 0 {
		noFiles = true
	}
	files.mutex.RUnlock()
	return
}

func (files *AllFiles) Count() (cnt int) {
	files.mutex.RLock()
	cnt = len(files.metadata)
	files.mutex.RUnlock()
	return
}

func (files *AllFiles) GetFileInfo(path str.LocalRepoPath) (info FileInfo) {
	files.mutex.RLock()
	info, validPath := files.metadata[path]
	if !validPath {
		return
	}
	files.mutex.RUnlock()
	return
}

func (files *AllFiles) GetFileData(identifier str.FileID) (data []byte) {
	files.mutex.RLock()
	data = files.data[identifier]
	files.mutex.RUnlock()
	return
}

func (files *AllFiles) ChangeFileDataPointer(path str.LocalRepoPath, newIdentifier str.FileID) {
	files.mutex.Lock()
	defer files.mutex.Unlock()
	info, validPath := files.metadata[path]
	if !validPath {
		return
	}
	info.Hash = newIdentifier
	files.metadata[path] = info
}
