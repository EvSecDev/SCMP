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

// Removes all references in host file for the given path
func (files *HostFiles) PurgePath(path str.LocalRepoPath) (err error) {
	files.mutex.Lock()
	defer files.mutex.Unlock()

	metadata, validPath := files.metadata[path]
	if !validPath {
		err = fmt.Errorf("path '%s' not tracked in metadata map", path)
		return
	}

	fileID := metadata.Hash

	// Metadata is path specific, delete completely
	delete(files.metadata, path)

	// Remove from grouped lists
	for _, group := range files.Groups {
		group.PurgePath(path)
	}

	// Remove data if not referenced anymore
	var doNotDeleteFileID bool
	for _, metadata := range files.metadata {
		if metadata.Hash == fileID {
			// Another file has the same content
			doNotDeleteFileID = true
		}
	}
	if !doNotDeleteFileID {
		// No other file shares the same content, remove from map
		delete(files.data, fileID)
	}
	return
}

func (files *HostFiles) InitPostInstallCmdSet() {
	files.mutex.RLock()
	defer files.mutex.RUnlock()

	// Post installation commands are grouped per independent deploy group and reload set
	for _, group := range files.Groups {
		for _, reloadID := range group.GetReloadIDs() {
			fileList := group.GetReloadIDFiles(reloadID)
			for _, file := range fileList {
				info, ok := files.metadata[file]
				if !ok {
					continue
				}

				group.AddPostInstallCommands(reloadID, file, info.PostInstall)
			}
		}
	}
}
