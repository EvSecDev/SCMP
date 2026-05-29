package deployment

import (
	"cmp"
	"scmp/internal/str"
	"slices"
	"sync"
)

func NewFileGroup(existingFileList []str.LocalRepoPath) (group *FileGroup) {
	listCopy := make([]str.LocalRepoPath, len(existingFileList))
	copy(listCopy, existingFileList)

	group = &FileGroup{
		list:              listCopy,
		reloadIDtoFile:    make(map[str.ReloadID][]str.LocalRepoPath),
		fileToReloadID:    make(map[str.LocalRepoPath]str.ReloadID),
		reloadIDfileCount: make(map[str.ReloadID]int),
		reloadIDcommands:  make(map[str.ReloadID][]string),
		mutex:             sync.RWMutex{},
	}
	return
}

// =================== MUTATIONS ===================

func (group *FileGroup) AppendFileToReloadID(reloadID str.ReloadID, paths ...str.LocalRepoPath) {
	pathsCopy := make([]str.LocalRepoPath, len(paths))
	copy(pathsCopy, paths)

	group.mutex.Lock()
	group.reloadIDtoFile[reloadID] = append(group.reloadIDtoFile[reloadID], pathsCopy...)
	group.mutex.Unlock()
}

func (group *FileGroup) AppendCmdToReloadID(reloadID str.ReloadID, cmds ...string) {
	cmdsCopy := make([]string, len(cmds))
	copy(cmdsCopy, cmds)

	group.mutex.Lock()
	group.reloadIDcommands[reloadID] = append(group.reloadIDcommands[reloadID], cmdsCopy...)
	group.mutex.Unlock()
}

// Reorders the slice per reload ID to match the main ordered list slice
func (group *FileGroup) OrderReloadIDFiles() {
	group.mutex.Lock()
	defer group.mutex.Unlock()

	// Build canonical deployment order lookup
	fileOrder := make(map[str.LocalRepoPath]int, len(group.list))
	for idx, file := range group.list {
		fileOrder[file] = idx
	}

	// Order every reload group according to deployment order
	for reloadID, files := range group.reloadIDtoFile {
		slices.SortStableFunc(files, func(a, b str.LocalRepoPath) int {
			return cmp.Compare(fileOrder[a], fileOrder[b])
		})

		group.reloadIDtoFile[reloadID] = files
	}
}

func (group *FileGroup) InitFiletoReloadID() {
	group.mutex.Lock()
	// Create file to reload id mapping

	for reloadID, reloadFiles := range group.reloadIDtoFile {
		for _, reloadFile := range reloadFiles {
			group.fileToReloadID[reloadFile] = reloadID
		}
	}
	group.mutex.Unlock()
}

func (group *FileGroup) RecordReloadIDFileCount() {
	group.mutex.Lock()
	for reloadID, groupFiles := range group.reloadIDtoFile {
		group.reloadIDfileCount[reloadID] += len(groupFiles)
	}
	group.mutex.Unlock()
}

// =================== READ ONLY ===================

func (group *FileGroup) GetOrderedList() (paths []str.LocalRepoPath) {
	group.mutex.RLock()
	paths = make([]str.LocalRepoPath, len(group.list))
	copy(paths, group.list)
	group.mutex.RUnlock()
	return
}

func (group *FileGroup) GetReloadIDFiles(reloadID str.ReloadID) (paths []str.LocalRepoPath) {
	group.mutex.RLock()
	paths = make([]str.LocalRepoPath, len(group.reloadIDtoFile[reloadID]))
	copy(paths, group.reloadIDtoFile[reloadID])
	group.mutex.RUnlock()
	return
}

func (group *FileGroup) GetReloadIDCommands(reloadID str.ReloadID) (cmds []string) {
	group.mutex.RLock()
	cmds = make([]string, len(group.reloadIDcommands[reloadID]))
	copy(cmds, group.reloadIDcommands[reloadID])
	group.mutex.RUnlock()
	return
}

func (group *FileGroup) GetReloadIDs() (reloadIDs []str.ReloadID) {
	group.mutex.RLock()
	for reloadID := range group.reloadIDtoFile {
		reloadIDs = append(reloadIDs, reloadID)
	}
	slices.Sort(reloadIDs)
	group.mutex.RUnlock()
	return
}

func (group *FileGroup) GetFileReloadID(path str.LocalRepoPath) (reloadID str.ReloadID, hasReload bool) {
	group.mutex.RLock()
	reloadID, hasReload = group.fileToReloadID[path]
	group.mutex.RUnlock()
	return
}

func (group *FileGroup) GetReloadIDFileCount(reloadID str.ReloadID) (fileCount int) {
	group.mutex.RLock()
	fileCount = group.reloadIDfileCount[reloadID]
	group.mutex.RUnlock()
	return
}

// Retrieves list of files for a reloadID but reversed from deployment order
func (group *FileGroup) GetReloadIDFilesReverse(reloadID str.ReloadID) (paths []str.LocalRepoPath) {
	group.mutex.RLock()
	defer group.mutex.RUnlock()

	files := group.reloadIDtoFile[reloadID]

	paths = make([]str.LocalRepoPath, len(files))

	for i := range files {
		paths[i] = files[len(files)-1-i]
	}

	return
}
