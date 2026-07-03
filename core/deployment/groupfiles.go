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
		reloadIDcommands:  make(map[str.ReloadID]map[str.LocalRepoPath][]string),
		reloadIDpostinst:  make(map[str.ReloadID]map[str.LocalRepoPath][]string),
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

func (group *FileGroup) AppendCmdToReloadID(reloadID str.ReloadID, file str.LocalRepoPath, cmds ...string) {
	cmdsCopy := make([]string, len(cmds))
	copy(cmdsCopy, cmds)

	group.mutex.Lock()
	fileCmds, ok := group.reloadIDcommands[reloadID]
	if !ok {
		fileCmds = make(map[str.LocalRepoPath][]string)
	}
	fileCmds[file] = append(fileCmds[file], cmds...)
	group.reloadIDcommands[reloadID] = fileCmds
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

func (group *FileGroup) AddPostInstallCommands(reloadID str.ReloadID, file str.LocalRepoPath, cmdSet []string) {
	group.mutex.Lock()
	defer group.mutex.Unlock()
	fileCmds, ok := group.reloadIDpostinst[reloadID]
	if !ok {
		fileCmds = make(map[str.LocalRepoPath][]string)
	}
	fileCmds[file] = append(fileCmds[file], cmdSet...)
	group.reloadIDpostinst[reloadID] = fileCmds
}

func (group *FileGroup) RecordReloadIDFileCount() {
	group.mutex.Lock()
	for reloadID, groupFiles := range group.reloadIDtoFile {
		group.reloadIDfileCount[reloadID] += len(groupFiles)
	}
	group.mutex.Unlock()
}

func (group *FileGroup) PurgePath(path str.LocalRepoPath) {
	group.mutex.Lock()
	defer group.mutex.Unlock()

	// Remove from deployment list itself
	indexToDelete := slices.Index(group.list, path)
	if indexToDelete == -1 {
		// Path not present in this group, no-op return
		return
	}
	group.list = append(group.list[:indexToDelete], group.list[indexToDelete+1:]...)

	// Remove from reload id mapping
	delete(group.fileToReloadID, path)

	// Find reloadIDs to modify
	var reloadIDsToModify []str.ReloadID
	for reloadID, paths := range group.reloadIDtoFile {
		indexToDelete := slices.Index(paths, path)
		if indexToDelete == -1 {
			// Path to delete not part of this reloadID
			continue
		}
		newPaths := append(paths[:indexToDelete], paths[indexToDelete+1:]...)
		group.reloadIDtoFile[reloadID] = newPaths

		if len(newPaths) == 0 {
			// Path to delete was only path for this reload ID
			delete(group.reloadIDtoFile, reloadID)
		}

		reloadIDsToModify = append(reloadIDsToModify, reloadID)
	}

	// Decrement or delete reloadIDs that were just for the path being deleted
	for _, reloadID := range reloadIDsToModify {
		count, exists := group.reloadIDfileCount[reloadID]
		if !exists {
			continue
		}
		newCount := count - 1

		delete(group.reloadIDcommands[reloadID], path)
		delete(group.reloadIDpostinst[reloadID], path)

		if newCount < 1 {
			delete(group.reloadIDfileCount, reloadID)
			delete(group.reloadIDcommands, reloadID)
			delete(group.reloadIDpostinst, reloadID)
		} else {
			group.reloadIDfileCount[reloadID] = newCount
		}
	}
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
	defer group.mutex.RUnlock()

	// Duplicate commands should not appear in raw ordered set
	seen := make(map[string]bool)

	reloadCmdMap := group.reloadIDcommands[reloadID]
	for _, file := range group.reloadIDtoFile[reloadID] {
		cmds := reloadCmdMap[file]
		for _, cmd := range cmds {
			if seen[cmd] {
				continue
			}
			seen[cmd] = true
			cmds = append(cmds, cmd)
		}
	}
	return
}

func (group *FileGroup) GetReloadIDPostInstCommands(reloadID str.ReloadID) (cmds []string) {
	group.mutex.RLock()
	defer group.mutex.RUnlock()

	// Duplicate commands should not appear in raw ordered set
	seen := make(map[string]bool)

	postInstCmdMap := group.reloadIDpostinst[reloadID]
	for _, file := range group.reloadIDtoFile[reloadID] {
		cmds := postInstCmdMap[file]
		for _, cmd := range cmds {
			if seen[cmd] {
				continue
			}
			seen[cmd] = true
			cmds = append(cmds, cmd)
		}
	}
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
